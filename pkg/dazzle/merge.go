package dazzle

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/cli/cli/config/configfile"
	docker "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/docker/pkg/term"
	"github.com/mholt/archiver"
	"github.com/mitchellh/go-homedir"
	log "github.com/sirupsen/logrus"
)

// NewEnvironment creates a new default environment
func NewEnvironment() (*Environment, error) {
	ctx := context.Background()
	client, err := docker.NewEnvClient()
	if err != nil {
		return nil, err
	}
	client.NegotiateAPIVersion(ctx)

	home, err := homedir.Dir()
	if err != nil {
		return nil, err
	}
	home, err = homedir.Expand(home)
	if err != nil {
		return nil, err
	}
	dockerCfgFN := filepath.Join(home, ".docker", "config.json")

	dockerCfg := configfile.New(dockerCfgFN)
	if dockerCfgF, err := os.OpenFile(dockerCfgFN, os.O_RDONLY, 0600); err == nil {
		err := dockerCfg.LoadFromReader(dockerCfgF)
		dockerCfgF.Close()

		if err != nil {
			return nil, err
		}
		log.WithField("filename", dockerCfgFN).Info("using Docker config")
	}

	wd := os.Getenv("DAZZLE_WORKDIR")
	if wd == "" {
		wd, err = ioutil.TempDir("", "")
		if err != nil {
			return nil, err
		}
	}
	log.WithField("workdir", wd).Info("working here")

	return &Environment{
		Out:       os.Stdout,
		Client:    client,
		DockerCfg: dockerCfg,
		Context:   ctx,
		Workdir:   wd,
	}, nil
}

// Environment describes the environment in which an image merge is to happen
type Environment struct {
	Out       io.Writer
	Client    *docker.Client
	DockerCfg *configfile.ConfigFile

	Context context.Context
	Workdir string
}

// MergeImages merges a set of Docker images while keeping the layer hashes
func MergeImages(env *Environment, dest, base string, addons ...string) error {
	wd := env.Workdir
	os.RemoveAll(wd)
	os.Mkdir(wd, 0755)

	// download images
	fmt.Fprintln(env.Out, "🌟\tsummoning interdimensional portal")
	allimgNames := append(addons, base)
	img, err := env.Client.ImageSave(env.Context, allimgNames)
	if err != nil {
		return err
	}

	allimgFn := filepath.Join(wd, "allimgs.tar")
	f, err := os.OpenFile(allimgFn, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, img)
	f.Close()
	if err != nil {
		return err
	}

	// extract the saved tar
	fmt.Fprintln(env.Out, "🥡\topening pandoras box")
	repoFn := filepath.Join(wd, "repo")
	err = os.Mkdir(repoFn, 0755)
	if err != nil {
		return err
	}
	err = archiver.Unarchive(allimgFn, repoFn)
	if err != nil {
		return err
	}

	// read manifest
	fmt.Fprintln(env.Out, "📖\treading forbidden manifests")
	manifestFn := filepath.Join(repoFn, "manifest.json")
	manifest, err := loadTarExportManifest(manifestFn)
	if err != nil {
		return err
	}

	// find images
	baseImage := manifest.GetByRepoTag(base)
	if baseImage == nil {
		return fmt.Errorf("base image %s was not downloaded", base)
	}
	var addonImages []tarExportManifestEntry
	for _, n := range addons {
		img := manifest.GetByRepoTag(n)
		if img == nil {
			return fmt.Errorf("addon image %s was not downloaded", n)
		}

		addonImages = append(addonImages, *img)
	}

	// build the new ~world~ layer order
	fmt.Fprintln(env.Out, "🌍\tbuilding new world order")
	var (
		layers    []string
		diffIDs   []string
		histories []map[string]interface{}
	)
	layers = append(layers, baseImage.Layers...)
	diffIDs = append(diffIDs, baseImage.LoadedConfig.RootFS.DiffIDs...)
	histories = append(histories, baseImage.LoadedConfig.History...)
	for _, addonImg := range addonImages {
		for i, l := range addonImg.Layers {
			if i < len(baseImage.Layers) {
				continue
			}

			layers = append(layers, l)
			diffIDs = append(diffIDs, addonImg.LoadedConfig.RootFS.DiffIDs[i])
			histories = append(histories, addonImg.LoadedConfig.History[i])
		}
	}

	// create new image config from base layer config
	fmt.Fprintln(env.Out, "🔥\tremaking to the world to my liking")
	fc, err := ioutil.ReadFile(filepath.Join(repoFn, baseImage.Config))
	if err != nil {
		return err
	}
	var baselayerConfig map[string]interface{}
	err = json.Unmarshal(fc, &baselayerConfig)
	if err != nil {
		return err
	}
	baselayerConfig["rootfs"].(map[string]interface{})["diff_ids"] = diffIDs
	baselayerConfig["history"] = histories
	fc, err = json.Marshal(baselayerConfig)
	if err != nil {
		return err
	}
	baselayerConfigHash := fmt.Sprintf("%x", sha256.Sum256(fc))
	newConfigFn := baselayerConfigHash + ".json"
	err = ioutil.WriteFile(filepath.Join(repoFn, newConfigFn), fc, 0611)
	if err != nil {
		return err
	}

	// create new manifest
	fmt.Fprintln(env.Out, "🙈\trewriting history")
	var newManifest tarExportManifest
	newManifest = append(newManifest, tarExportManifestEntry{
		Config:   newConfigFn,
		RepoTags: []string{dest},
		Layers:   layers,
	})
	fc, err = json.Marshal(newManifest)
	if err != nil {
		return err
	}
	err = os.Rename(manifestFn, filepath.Join(repoFn, "manifest_original.json"))
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(manifestFn, fc, 0611)
	if err != nil {
		return err
	}

	// update the layer json files
	for i, l := range layers[1:] {
		cfgFn := filepath.Join(repoFn, filepath.Dir(l), "json")
		fc, err := ioutil.ReadFile(cfgFn)
		if err != nil {
			return err
		}

		var cfg map[string]interface{}
		err = json.Unmarshal(fc, &cfg)
		if err != nil {
			return err
		}

		cfg["parent"] = layerName(layers[i])

		fc, err = json.Marshal(cfg)
		if err != nil {
			return err
		}
		err = os.Rename(cfgFn, cfgFn+"_original")
		if err != nil {
			return err
		}
		err = ioutil.WriteFile(cfgFn, fc, 0611)
		if err != nil {
			return err
		}
	}

	// replace the repositories file
	dstsegs := strings.Split(dest, ":")
	dstrepo := dstsegs[0]
	dsttag := "latest"
	if len(dstsegs) > 1 {
		dsttag = dstsegs[1]
	}
	repositories := map[string]map[string]string{
		dstrepo: map[string]string{
			dsttag: layerName(layers[len(layers)-1]),
		},
	}
	fc, err = json.Marshal(repositories)
	if err != nil {
		return err
	}
	err = os.Rename(filepath.Join(repoFn, "repositories"), filepath.Join(repoFn, "repositories_original"))
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(filepath.Join(repoFn, "repositories"), fc, 0611)
	if err != nil {
		return err
	}

	// pack it up
	fmt.Fprintln(env.Out, "⚰️\tpacking it all up")
	var pkgcnt []string
	pkgcnt = append(pkgcnt,
		filepath.Join(repoFn, "manifest.json"),
		filepath.Join(repoFn, "repositories"),
		filepath.Join(repoFn, newManifest[0].Config),
	)
	for _, l := range layers {
		base := filepath.Join(repoFn, filepath.Dir(l))
		pkgcnt = append(pkgcnt, base)
	}
	pkgfn := filepath.Join(wd, "pkg.tar")

	err = archiver.Archive(pkgcnt, pkgfn)
	if err != nil {
		return err
	}

	// load it back into the daemon
	fmt.Fprintln(env.Out, "👹\toffering world to daemons")
	pkg, err := os.OpenFile(pkgfn, os.O_RDONLY, 0644)
	if err != nil {
		return err
	}
	defer pkg.Close()
	resp, err := env.Client.ImageLoad(env.Context, pkg, false)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	termFd, isTerm := term.GetFdInfo(env.Out)
	err = jsonmessage.DisplayJSONMessagesStream(resp.Body, env.Out, termFd, isTerm, nil)
	if err != nil {
		return err
	}

	return nil
}

func loadTarExportManifest(fn string) (*tarExportManifest, error) {
	var manifest tarExportManifest
	mffc, err := ioutil.ReadFile(fn)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(mffc, &manifest)
	if err != nil {
		return nil, err
	}

	for li, layer := range manifest {
		var cfg layerConfig
		fc, err := ioutil.ReadFile(filepath.Join(filepath.Dir(fn), layer.Config))
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(fc, &cfg)
		if err != nil {
			return nil, err
		}

		var newHistory []map[string]interface{}
		for _, h := range cfg.History {
			if h["empty_layer"] == true {
				continue
			}

			newHistory = append(newHistory, h)
		}
		cfg.History = newHistory
		layer.LoadedConfig = &cfg
		manifest[li] = layer
	}

	return &manifest, nil
}

type tarExportManifest []tarExportManifestEntry

type tarExportManifestEntry struct {
	Config   string
	RepoTags []string
	Layers   []string

	LoadedConfig *layerConfig
}

type layerConfig struct {
	History []map[string]interface{} `json:"history"`
	RootFS  struct {
		Type    string   `json:"type"`
		DiffIDs []string `json:"diff_ids"`
	} `json:"rootfs"`
}

func (m tarExportManifest) GetByRepoTag(tag string) *tarExportManifestEntry {
	for _, e := range m {
		for _, et := range e.RepoTags {
			if et == tag {
				return &e
			}
		}
	}
	return nil
}

func layerName(path string) string {
	return filepath.Base(filepath.Dir(path))
}

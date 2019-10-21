package dazzle

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	golog "log"
	"os"
	"path/filepath"

	"github.com/32leaves/dazzle/pkg/fancylog"
	"github.com/docker/cli/cli/config/configfile"
	docker "github.com/docker/docker/client"
	"github.com/mholt/archiver"
	"github.com/mitchellh/go-homedir"
	"github.com/segmentio/textio"
	log "github.com/sirupsen/logrus"

	"github.com/buildpack/imgutil/remote"
	"github.com/google/go-containerregistry/pkg/authn"
)

// NewEnvironment creates a new default environment
func NewEnvironment() (*Environment, error) {
	ctx := context.Background()
	client, err := docker.NewEnvClient()
	if err != nil {
		return nil, err
	}
	client.NegotiateAPIVersion(ctx)
	_, err = client.ServerVersion(ctx)
	if err != nil {
		return nil, err
	}

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
		log.WithField("filename", dockerCfgFN).Debug("using Docker config")
	}

	wd := os.Getenv("DAZZLE_WORKDIR")
	if wd == "" {
		wd, err = ioutil.TempDir("", "")
		if err != nil {
			return nil, err
		}
	}
	log.WithField("workdir", wd).Debug("working here")

	return &Environment{
		BaseOut:   os.Stdout,
		Client:    client,
		DockerCfg: dockerCfg,
		Formatter: &fancylog.Formatter{},
		Context:   ctx,
		Workdir:   wd,
	}, nil
}

// Environment describes the environment in which an image merge is to happen
type Environment struct {
	BaseOut   io.Writer
	Client    *docker.Client
	DockerCfg *configfile.ConfigFile

	PrettyLayerNames map[string]string

	Formatter *fancylog.Formatter
	Context   context.Context
	Workdir   string
}

// Out produces the output channel for log output
func (env *Environment) Out() io.WriteCloser {
	padding := fancylog.DefaultIndent
	for i := 0; i < env.Formatter.Level; i++ {
		padding += "  "
	}
	return &closablePrefixWriter{textio.NewPrefixWriter(env.BaseOut, padding)}
}

type closablePrefixWriter struct {
	*textio.PrefixWriter
}

func (w *closablePrefixWriter) Close() error {
	return w.Flush()
}

// MergeImages merges a set of Docker images while keeping the layer hashes
func MergeImages(env *Environment, dest, base string, addons ...string) error {
	wd := env.Workdir
	os.RemoveAll(wd)
	os.Mkdir(wd, 0755)

	// download images
	log.WithField("step", 1).WithField("emoji", "🌟").Info("downloading images")
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
	log.WithField("step", 2).WithField("emoji", "🥡").Info("extracting images")
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
	log.WithField("step", 3).WithField("emoji", "📖").Info("reading exported manifests")
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

	// create dest image
	log.WithField("step", 4).WithField("emoji", "🔥").Info("assembling layers")
	dst, err := remote.NewImage(dest, authn.DefaultKeychain, remote.FromBaseImage(base))
	if err != nil {
		return err
	}

	for i, ai := range addonImages {
		for _, l := range ai.Layers[len(baseImage.Layers):] {
			sourceName := addons[i]
			if env.PrettyLayerNames != nil {
				betterName, ok := env.PrettyLayerNames[sourceName]
				if ok {
					sourceName = betterName
				}
			}
			log.WithField("layer", l).WithField("from", sourceName).Debug("adding layer")
			err = dst.AddLayer(filepath.Join(repoFn, l))
			if err != nil {
				return err
			}
		}
	}

	log.WithField("step", 5).WithField("emoji", "🙈").Info("pushing merged image")
	golog.SetOutput(env.Out())
	err = dst.Save()
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

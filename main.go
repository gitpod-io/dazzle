package main

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

	// "github.com/docker/docker/api/types"
	docker "github.com/docker/docker/client"
	"github.com/mholt/archiver"
	// imgspec_v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

const (
	col     = "\x1b[38;2;255;100;0m"
	colterm = "\x1b[0m\n"
)

func main() {
	args := os.Args
	if len(args) < 3 {
		fmt.Fprintf(os.Stderr, "usage: %s target-ref base-ref layers...\n", args[0])
		return
	}

	ctx := context.Background()
	cli, err := docker.NewEnvClient()
	if err != nil {
		panic(err)
	}

	wd := os.Getenv("DAZZLE_WORKDIR")
	if wd == "" {
		wd, err = ioutil.TempDir("", "")
		if err != nil {
			panic(err)
		}
		fmt.Printf("wd: %s\n", wd)
	} else {
		os.RemoveAll(wd)
		os.Mkdir(wd, 0755)
	}

	// download images
	fmt.Printf("🌟\t%sopening interdimensional portal to make graven images%s", col, colterm)
	dstimgName := args[1]
	allimgNames := args[2:]
	baseimgName := args[2]
	addonimgNames := args[3:]
	img, err := cli.ImageSave(ctx, allimgNames)
	if err != nil {
		panic(err)
	}

	allimgFn := filepath.Join(wd, "allimgs.tar")
	f, err := os.OpenFile(allimgFn, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}
	_, err = io.Copy(f, img)
	f.Close()
	if err != nil {
		panic(err)
	}

	// extract the saved tar
	fmt.Printf("🥡\t%sopening pandoras box%s", col, colterm)
	repoFn := filepath.Join(wd, "repo")
	err = os.Mkdir(repoFn, 0755)
	if err != nil {
		panic(err)
	}
	err = archiver.Unarchive(allimgFn, repoFn)
	if err != nil {
		panic(err)
	}

	// read manifest
	fmt.Printf("📖\t%sreading forbidden manifests%s", col, colterm)
	manifestFn := filepath.Join(repoFn, "manifest.json")
	manifest, err := loadTarExportManifest(manifestFn)
	if err != nil {
		panic(err)
	}

	// find images
	baseImage := manifest.GetByRepoTag(baseimgName)
	if baseImage == nil {
		panic("base image " + baseimgName + " was not downloaded")
	}
	var addonImages []tarExportManifestEntry
	for _, n := range addonimgNames {
		img := manifest.GetByRepoTag(n)
		if img == nil {
			panic("addon image " + baseimgName + " was not downloaded")
		}

		addonImages = append(addonImages, *img)
	}

	// build the new ~world~ layer order
	fmt.Printf("🌍\t%sbuilding new world order%s", col, colterm)
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
	fmt.Printf("🔥\t%sremaking to the world to my liking%s", col, colterm)
	fc, err := ioutil.ReadFile(filepath.Join(repoFn, baseImage.Config))
	if err != nil {
		panic(err)
	}
	var baselayerConfig map[string]interface{}
	err = json.Unmarshal(fc, &baselayerConfig)
	if err != nil {
		panic(err)
	}
	baselayerConfig["rootfs"].(map[string]interface{})["diff_ids"] = diffIDs
	baselayerConfig["history"] = histories
	fc, err = json.Marshal(baselayerConfig)
	if err != nil {
		panic(err)
	}
	baselayerConfigHash := fmt.Sprintf("%x", sha256.Sum256(fc))
	newConfigFn := baselayerConfigHash + ".json"
	err = ioutil.WriteFile(filepath.Join(repoFn, newConfigFn), fc, 0611)
	if err != nil {
		panic(err)
	}

	// create new manifest
	fmt.Printf("🙈\t%srewriting history%s", col, colterm)
	var newManifest tarExportManifest
	newManifest = append(newManifest, tarExportManifestEntry{
		Config:   newConfigFn,
		RepoTags: []string{dstimgName},
		Layers:   layers,
	})
	fc, err = json.Marshal(newManifest)
	if err != nil {
		panic(err)
	}
	err = os.Rename(manifestFn, filepath.Join(repoFn, "manifest_original.json"))
	if err != nil {
		panic(err)
	}
	err = ioutil.WriteFile(manifestFn, fc, 0611)
	if err != nil {
		panic(err)
	}

	// update the layer json files
	for i, l := range layers[1:] {
		cfgFn := filepath.Join(repoFn, filepath.Dir(l), "json")
		fc, err := ioutil.ReadFile(cfgFn)
		if err != nil {
			panic(err)
		}

		var cfg map[string]interface{}
		err = json.Unmarshal(fc, &cfg)
		if err != nil {
			panic(err)
		}

		cfg["parent"] = layerName(layers[i])

		fc, err = json.Marshal(cfg)
		if err != nil {
			panic(err)
		}
		err = os.Rename(cfgFn, cfgFn+"_original")
		if err != nil {
			panic(err)
		}
		err = ioutil.WriteFile(cfgFn, fc, 0611)
		if err != nil {
			panic(err)
		}
	}

	// replace the repositories file
	dstsegs := strings.Split(dstimgName, ":")
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
		panic(err)
	}
	err = os.Rename(filepath.Join(repoFn, "repositories"), filepath.Join(repoFn, "repositories_original"))
	if err != nil {
		panic(err)
	}
	err = ioutil.WriteFile(filepath.Join(repoFn, "repositories"), fc, 0611)
	if err != nil {
		panic(err)
	}

	// pack it up
	fmt.Printf("⚰️\t%spacking it all up%s", col, colterm)
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
		panic(err)
	}

	// load it back into the daemon
	fmt.Printf("👹\t%soffering world to the daemon%s", col, colterm)
	pkg, err := os.OpenFile(pkgfn, os.O_RDONLY, 0644)
	if err != nil {
		panic(err)
	}
	defer pkg.Close()
	resp, err := cli.ImageLoad(ctx, pkg, false)
	if err != nil {
		panic(err)
	}
	_, err = io.Copy(os.Stdout, resp.Body)
	if err != nil {
		panic(err)
	}
	resp.Body.Close()
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

	for i, layer := range manifest {
		var cfg layerConfig
		fc, err := ioutil.ReadFile(filepath.Join(filepath.Dir(fn), layer.Config))
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(fc, &cfg)
		if err != nil {
			return nil, err
		}
		for i, h := range cfg.History {
			if h["empty_layer"] == true {
				cfg.History = append(cfg.History[:i], cfg.History[i+1:]...)
			}
		}
		layer.LoadedConfig = &cfg
		manifest[i] = layer
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

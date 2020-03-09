package dazzle

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/32leaves/dazzle/pkg/test"
	containertest "github.com/32leaves/dazzle/pkg/test/container"
	"github.com/docker/distribution/reference"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/docker/pkg/term"
	"github.com/mholt/archiver"
	"github.com/moby/buildkit/frontend/dockerfile/command"
	"github.com/moby/buildkit/frontend/dockerfile/parser"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

// BuildConfig configures a dazzle build
type BuildConfig struct {
	Env *Environment

	// BuildImageRepo is the name/repo of the individual build layers. When UseRegistry is true
	// this repo should be something that can be pushed to a registry.
	BuildImageRepo string

	SourceLoc string
	Chunks    []string
}

const (
	labelLayer = "dazzle/layer"
	labelTest  = "dazzle/test"

	layerNamePrologue = "dazzle-prologue"
)

// BuildResult describes the information produced during a build
type BuildResult struct {
	BaseImage    LayerBuildResult
	Layers       []LayerBuildResult
	PrologueTest []*test.Results
}

// LayerBuildResult is the result of an individual layer build
type LayerBuildResult struct {
	Ref        string
	Pulled     bool
	LayerName  string
	HasTest    bool
	TestResult *test.Results
	Size       int64
}

// Build builds a Dockerfile with independent layers
func Build(cfg BuildConfig) (res *BuildResult, err error) {
	chunks, err := collectChunks(cfg)
	if err != nil {
		return
	}

	baseRes, err := buildBaseImage(cfg)
	if err != nil {
		return
	}

	res = &BuildResult{
		BaseImage: baseRes,
	}
	var (
		rchan = make(chan LayerBuildResult)
		echan = make(chan error)
	)
	for _, chunk := range chunks {
		go buildChunk(chunk, rchan, echan)
		select {
		case r := <-rchan:
			res.Layers = append(res.Layers, r)
		case err := <-echan:
			return nil, err
		}
	}

	return
}

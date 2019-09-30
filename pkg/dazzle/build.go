package dazzle

import (
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

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/docker/pkg/term"
	"github.com/mholt/archiver"
	"github.com/moby/buildkit/frontend/dockerfile/command"
	"github.com/moby/buildkit/frontend/dockerfile/parser"
	log "github.com/sirupsen/logrus"
)

// BuildConfig configures a dazzle build
type BuildConfig struct {
	Env *Environment

	// if true we'll attempt to pull the partial images prior to building them
	UseRegistry bool

	// BuildImageRepo is the name/repo of the individual build layers. When UseRegistry is true
	// this repo should be something that can be pushed to a registry.
	BuildImageRepo string
}

const (
	layerLabel = "dazzle/layer"
)

// Build builds a Dockerfile with independent layers
func Build(cfg BuildConfig, loc, dockerfile, dst string) error {
	fullDFN := filepath.Join(loc, dockerfile)

	parsedDF, err := ParseDockerfile(fullDFN)
	if err != nil {
		return err
	}

	if err = parsedDF.Validate(); err != nil {
		return err
	}

	// compute splitpoints
	sps, err := parsedDF.SplitPoints()
	if err != nil {
		return err
	}

	// split off base dockerfile
	baseSP := sps[0]
	baseDFN := "dazzle___base.Dockerfile"
	err = parsedDF.ExtractFrom(filepath.Join(loc, baseDFN), baseSP, "")
	if err != nil {
		return err
	}

	// split off the addon dockerfiles (do this prior to creating the context)
	baseHash, err := fileChecksum(filepath.Join(loc, baseDFN))
	if err != nil {
		return err
	}
	baseImgName := fmt.Sprintf("%s:base-%s", cfg.BuildImageRepo, baseHash)

	// split of addon Dockerfiles
	addons := sps[1:]
	var builds []string
	for _, sp := range addons {
		fn := fmt.Sprintf("dazzle__%s.Dockerfile", sp.Name)
		err = parsedDF.ExtractFrom(filepath.Join(loc, fn), sp, baseImgName)

		if err != nil {
			return err
		}
		builds = append(builds, fn)
	}

	// create the prologue Dockerfile
	fullDfHash, err := fileChecksum(fullDFN)
	if err != nil {
		return err
	}
	mergedImgName := fmt.Sprintf("%s:merged-%s", cfg.BuildImageRepo, fullDfHash)
	prologueDFN := "dazzle___prologue.Dockerfile"
	err = parsedDF.ExtractEnvs(filepath.Join(loc, prologueDFN), mergedImgName)
	if err != nil {
		return err
	}

	// create build context
	fns, err := ioutil.ReadDir(loc)
	if err != nil {
		return err
	}
	var buildctxCtnt []string
	for _, bfi := range fns {
		buildctxCtnt = append(buildctxCtnt, filepath.Join(loc, bfi.Name()))
	}
	buildctxFn := filepath.Join(cfg.Env.Workdir, "build-context.tar.gz")
	os.Remove(buildctxFn)
	err = archiver.Archive(buildctxCtnt, buildctxFn)
	if err != nil {
		return err
	}
	fmt.Printf("created build context in %s\n", buildctxFn)

	// build base image
	err = pullOrBuildImage(cfg, buildctxFn, baseImgName, types.ImageBuildOptions{
		PullParent: true,
		Dockerfile: baseDFN,
	})
	if err != nil {
		return err
	}

	// build addons
	var buildNames []string
	for _, bd := range builds {
		dfhash, err := fileChecksum(filepath.Join(loc, bd))
		if err != nil {
			return err
		}
		buildName := fmt.Sprintf("%s:build-%s", cfg.BuildImageRepo, dfhash)
		err = pullOrBuildImage(cfg, buildctxFn, buildName, types.ImageBuildOptions{
			PullParent: false,
			Dockerfile: bd,
		})
		if err != nil {
			return err
		}

		buildNames = append(buildNames, buildName)
	}

	// merge the whole thing
	mergeEnv := *cfg.Env
	mergeEnv.Workdir = filepath.Join(mergeEnv.Workdir, "merge")
	err = MergeImages(&mergeEnv, mergedImgName, baseImgName, buildNames...)
	if err != nil {
		return err
	}

	// build image with prologue
	finalCfg := cfg
	finalCfg.UseRegistry = false
	err = pullOrBuildImage(finalCfg, buildctxFn, dst, types.ImageBuildOptions{
		PullParent: false,
		Dockerfile: prologueDFN,
	})
	if err != nil {
		return err
	}

	return nil
}

func getDockerAuthForTag(cfg BuildConfig, tag string) (string, error) {
	reg := strings.Split(strings.Split(tag, ":")[0], "/")[0]
	auth, err := cfg.Env.DockerCfg.GetAuthConfig(reg)
	if err != nil {
		return "", err
	}
	encodedJSON, err := json.Marshal(auth)
	if err != nil {
		return "", err
	}
	authStr := base64.URLEncoding.EncodeToString(encodedJSON)
	if auth.Username != "" {
		log.WithField("registry", reg).WithField("username", auth.Username).Debug("authenticating during operation")
	}

	return authStr, nil
}

func pullOrBuildImage(cfg BuildConfig, buildctxFn, tag string, opts types.ImageBuildOptions) error {
	log.WithField("dockerfile", opts.Dockerfile).WithField("tag", tag).Info("building image")

	env := cfg.Env

	termFd, isTerm := term.GetFdInfo(env.Out)
	if cfg.UseRegistry {
		auth, err := getDockerAuthForTag(cfg, tag)
		if err != nil {
			return err
		}

		presp, err := env.Client.ImagePull(env.Context, tag, types.ImagePullOptions{
			RegistryAuth: auth,
		})
		if err == nil {
			err = jsonmessage.DisplayJSONMessagesStream(presp, env.Out, termFd, isTerm, nil)
			if err != nil {
				return err
			}

			return nil
		}
		fmt.Println(err)
	}

	buildctx, err := os.OpenFile(buildctxFn, os.O_RDONLY, 0644)
	if err != nil {
		return err
	}

	opts.Tags = append(opts.Tags, tag)
	bresp, err := env.Client.ImageBuild(env.Context, buildctx, opts)
	if err != nil {
		return err
	}
	err = jsonmessage.DisplayJSONMessagesStream(bresp.Body, env.Out, termFd, isTerm, nil)
	if err != nil {
		return err
	}
	bresp.Body.Close()
	buildctx.Close()

	if cfg.UseRegistry {
		auth, err := getDockerAuthForTag(cfg, tag)
		if err != nil {
			return err
		}

		presp, err := env.Client.ImagePush(env.Context, tag, types.ImagePushOptions{
			RegistryAuth: auth,
		})
		if err != nil {
			return err
		}

		err = jsonmessage.DisplayJSONMessagesStream(presp, env.Out, termFd, isTerm, nil)
		if err != nil {
			return err
		}
	}

	return nil
}

func fileChecksum(fn string) (string, error) {
	input, err := os.OpenFile(fn, os.O_RDONLY, 0644)
	if err != nil {
		return "", err
	}
	defer input.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, input); err != nil {
		log.Fatal(err)
	}
	sum := hash.Sum(nil)

	return fmt.Sprintf("%x", sum), nil
}

// SplitPoint is a location within a Dockerfile where that file gets split
type SplitPoint struct {
	StartLine int
	EndLine   int
	Name      string
}

// ParsedDockerfile contains the result of a Dockerfile parse
type ParsedDockerfile struct {
	AST   *parser.Node
	Lines []string
}

// ParseDockerfile parses a Dockerfile
func ParseDockerfile(fn string) (*ParsedDockerfile, error) {
	fc, err := ioutil.ReadFile(fn)
	if err != nil {
		return nil, err
	}

	res, err := parser.Parse(bytes.NewReader(fc))
	if err != nil {
		return nil, err
	}

	return &ParsedDockerfile{
		AST:   res.AST,
		Lines: strings.Split(string(fc), "\n"),
	}, nil
}

// Validate ensures that the Dockerfile is suitable for use with dazzle
func (df *ParsedDockerfile) Validate() error {
	var fromCount int
	for _, tkn := range df.AST.Children {
		if tkn.Value == command.From {
			fromCount++

			if fromCount > 1 {
				return fmt.Errorf("dazzle does not support multi-stage builds")
			}
		}
		if tkn.Value == command.Arg {
			return fmt.Errorf("dazzle does not support build args")
		}
	}
	return nil
}

// SplitPoints computes the splitpoints along the dazzle/layer labels
func (df *ParsedDockerfile) SplitPoints() ([]SplitPoint, error) {
	var (
		sps []SplitPoint
		cur SplitPoint
	)
	cur = SplitPoint{
		StartLine: 0,
		Name:      "_base",
	}
	for _, tkn := range df.AST.Children {
		if tkn.Value != command.Label {
			cur.EndLine = tkn.EndLine
			continue
		}
		next := tkn.Next
		if next == nil {
			cur.EndLine = tkn.EndLine
			continue
		}
		if next.Value != layerLabel {
			cur.EndLine = tkn.EndLine
			continue
		}

		name := next.Next.Value
		if len(name) == 0 {
			return nil, fmt.Errorf("invalid dazzle layer name in line %d", tkn.StartLine)
		}
		sps = append(sps, cur)
		cur = SplitPoint{
			Name:      name,
			StartLine: tkn.StartLine,
		}
	}
	sps = append(sps, cur)
	return sps, nil
}

// ExtractFrom extracts a new Dockerfile at the given splitpoint
func (df *ParsedDockerfile) ExtractFrom(fn string, sp SplitPoint, baseImage string) error {
	// fn = fmt.Sprintf("dazzle__%s.Dockerfile", sp.Name)

	var ctnt []string
	if baseImage != "" {
		ctnt = append(ctnt, fmt.Sprintf("FROM %s", baseImage))
	}
	ctnt = append(ctnt, df.Lines[sp.StartLine:sp.EndLine]...)

	err := ioutil.WriteFile(filepath.Join(fn), []byte(strings.Join(ctnt, "\n")), 0644)
	if err != nil {
		return err
	}

	return nil
}

// ExtractEnvs creates a new Dockerfile which only contains the ENV statements of the parent
func (df *ParsedDockerfile) ExtractEnvs(fn, baseImage string) error {
	var envlines []string
	for _, tkn := range df.AST.Children {
		if tkn.Value != command.Env {
			continue
		}

		envlines = append(envlines, df.Lines[tkn.StartLine-1:tkn.EndLine]...)
	}

	newDockerfile := strings.Join(append([]string{fmt.Sprintf("FROM %s", baseImage)}, envlines...), "\n")
	err := ioutil.WriteFile(fn, []byte(newDockerfile), 0644)
	if err != nil {
		return err
	}

	return nil
}

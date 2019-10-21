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

	"github.com/32leaves/dazzle/pkg/test"
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
}

const (
	labelLayer = "dazzle/layer"
	labelTest  = "dazzle/test"
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
func Build(cfg BuildConfig, loc, dockerfile, dst string) (*BuildResult, error) {
	var step int

	fullDFN := filepath.Join(loc, dockerfile)
	parsedDF, err := ParseDockerfile(fullDFN)
	if err != nil {
		return nil, err
	}

	if err = parsedDF.Validate(); err != nil {
		return nil, err
	}

	// compute splitpoints
	sps, err := parsedDF.SplitPoints()
	if err != nil {
		return nil, err
	}

	// split off base dockerfile
	baseSP := sps[0]
	baseDFN := "dazzle___base.Dockerfile"
	err = parsedDF.ExtractFrom(filepath.Join(loc, baseDFN), baseSP, "")
	if err != nil {
		return nil, err
	}

	baseHash, err := fileChecksum(filepath.Join(loc, baseDFN))
	if err != nil {
		return nil, err
	}
	baseImgName := fmt.Sprintf("%s:base-%s", cfg.BuildImageRepo, baseHash)

	var res BuildResult
	res.BaseImage = LayerBuildResult{
		Ref: baseImgName,
	}

	// split off the addon dockerfiles (do this prior to creating the context)
	addons := sps[1:]
	type buildSpec struct {
		Dockerfile string
		Layer      string
		Test       string
	}
	var builds []buildSpec
	for _, sp := range addons {
		fn := fmt.Sprintf("dazzle__%s.Dockerfile", sp.Name)
		err = parsedDF.ExtractFrom(filepath.Join(loc, fn), sp, baseImgName)

		if err != nil {
			return &res, err
		}
		builds = append(builds, buildSpec{fn, sp.Name, sp.Test})
	}

	// create the prologue Dockerfile
	fullDfHash, err := fileChecksum(fullDFN)
	if err != nil {
		return &res, err
	}
	mergedImgName := fmt.Sprintf("%s:merged-%s", cfg.BuildImageRepo, fullDfHash)
	prologueDFN := "dazzle___prologue.Dockerfile"
	err = parsedDF.ExtractEnvs(filepath.Join(loc, prologueDFN), mergedImgName)
	if err != nil {
		return &res, err
	}

	// create build context
	fns, err := ioutil.ReadDir(loc)
	if err != nil {
		return &res, err
	}
	var buildctxCtnt []string
	for _, bfi := range fns {
		buildctxCtnt = append(buildctxCtnt, filepath.Join(loc, bfi.Name()))
	}
	buildctxFn := filepath.Join(cfg.Env.Workdir, "build-context.tar.gz")
	os.Remove(buildctxFn)
	err = archiver.Archive(buildctxCtnt, buildctxFn)
	if err != nil {
		return &res, err
	}
	log.WithField("buildContext", buildctxFn).WithField("emoji", "🏠").WithField("step", step).Info("created build context")
	step++

	// build base image
	log.WithField("buildContext", buildctxFn).WithField("emoji", "👷").WithField("step", step).Info("building base image")
	step++
	pulledBaseImg, err := pullOrBuildImage(cfg, buildctxFn, baseImgName, types.ImageBuildOptions{
		PullParent: true,
		Dockerfile: baseDFN,
	})
	if err != nil {
		return &res, err
	}
	if !pulledBaseImg {
		err = pushImage(cfg, baseImgName)
		if err != nil {
			return &res, err
		}
	}
	baseImgInspct, _, _ := cfg.Env.Client.ImageInspectWithRaw(cfg.Env.Context, baseImgName)
	res.BaseImage.Size = baseImgInspct.Size
	res.BaseImage.Pulled = pulledBaseImg

	// build addons
	var buildNames []string
	var testSuites []string
	prettyLayerNames := make(map[string]string)
	for _, bd := range builds {
		log.WithField("name", bd.Layer).WithField("emoji", "👷").WithField("step", step).Info("building addon image")
		step++

		dfhash, err := fileChecksum(filepath.Join(loc, bd.Dockerfile))
		if err != nil {
			return &res, err
		}
		buildName := fmt.Sprintf("%s:build-%s", cfg.BuildImageRepo, dfhash)
		pulledImg, err := pullOrBuildImage(cfg, buildctxFn, buildName, types.ImageBuildOptions{
			Dockerfile: bd.Dockerfile,
		})
		if err != nil {
			return &res, err
		}
		buildNames = append(buildNames, buildName)
		prettyLayerNames[buildName] = bd.Layer

		layerres := LayerBuildResult{
			Ref:       buildName,
			Pulled:    pulledImg,
			LayerName: bd.Layer,
			HasTest:   bd.Test != "",
		}

		if bd.Test != "" {
			testfn := filepath.Join(loc, bd.Test)
			cfg.Env.Formatter.Push()
			layerres.TestResult, err = testImage(cfg, bd.Layer, buildName, testfn)
			cfg.Env.Formatter.Pop()
			if err != nil {
				// add this layer to the results so that the test result is part of the overall build result
				res.Layers = append(res.Layers, layerres)
				return &res, err
			}

			testSuites = append(testSuites, testfn)
		}

		if !pulledImg {
			err = pushImage(cfg, buildName)
			if err != nil {
				return &res, err
			}
		}

		imginspct, _, err := cfg.Env.Client.ImageInspectWithRaw(cfg.Env.Context, buildName)
		if err == nil {
			layerres.Size = imginspct.Size - res.BaseImage.Size
		}
		res.Layers = append(res.Layers, layerres)
	}

	// merge the whole thing
	log.WithField("emoji", "🤘").WithField("step", step).Info("merging images")
	step++

	mergeEnv := *cfg.Env
	mergeEnv.PrettyLayerNames = prettyLayerNames
	mergeEnv.Workdir = filepath.Join(mergeEnv.Workdir, "merge")
	cfg.Env.Formatter.Push()
	err = MergeImages(&mergeEnv, mergedImgName, baseImgName, buildNames...)
	cfg.Env.Formatter.Pop()
	if err != nil {
		return &res, err
	}

	// build image with prologue
	log.WithField("emoji", "👷").WithField("step", step).Info("building prologue image")
	step++
	allCliAuth, err := cfg.Env.DockerCfg.GetAllCredentials()
	if err != nil {
		return &res, err
	}
	allAuth := make(map[string]types.AuthConfig)
	for k, v := range allCliAuth {
		allAuth[k] = types.AuthConfig{
			Username:      v.Username,
			Password:      v.Password,
			Auth:          v.Auth,
			Email:         v.Email,
			ServerAddress: v.ServerAddress,
			IdentityToken: v.IdentityToken,
			RegistryToken: v.RegistryToken,
		}
	}
	err = buildImage(cfg, buildctxFn, dst, types.ImageBuildOptions{
		PullParent:  true,
		AuthConfigs: allAuth,
		Dockerfile:  prologueDFN,
	})
	if err != nil {
		return &res, err
	}

	// if there are tests, run them against the final image
	var ptr []*test.Results
	for _, testfn := range testSuites {
		cfg.Env.Formatter.Push()
		tr, err := testImage(cfg, testfn, dst, testfn)
		cfg.Env.Formatter.Pop()
		if err != nil {
			return &res, err
		}

		ptr = append(ptr, tr)
	}
	res.PrologueTest = ptr

	// finally push the whole thing
	err = pushImage(cfg, dst)
	if err != nil {
		return &res, err
	}

	return &res, nil
}

func testImage(cfg BuildConfig, layerName, image, testfn string) (res *test.Results, err error) {
	fc, err := ioutil.ReadFile(testfn)
	if err != nil {
		return nil, err
	}

	var tests []*test.Spec
	err = yaml.Unmarshal(fc, &tests)
	if err != nil {
		return nil, err
	}

	rawres, success := test.RunTests(cfg.Env.Context, cfg.Env.Client, image, tests)
	if !success {
		return nil, fmt.Errorf("tests failed")
	}

	return &rawres, nil
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

func pullOrBuildImage(cfg BuildConfig, buildctxFn, tag string, opts types.ImageBuildOptions) (pulledImage bool, err error) {
	log.WithField("dockerfile", opts.Dockerfile).WithField("tag", tag).Debug("building image")

	env := cfg.Env
	termFd, isTerm := term.GetFdInfo(env.Out)

	auth, err := getDockerAuthForTag(cfg, tag)
	if err != nil {
		return false, err
	}

	presp, err := env.Client.ImagePull(env.Context, tag, types.ImagePullOptions{
		RegistryAuth: auth,
	})
	if err == nil {
		err = jsonmessage.DisplayJSONMessagesStream(presp, env.Out(), termFd, isTerm, nil)
		if err != nil {
			return false, err
		}

		return true, nil
	}

	if strings.Contains(err.Error(), "not found") {
		log.WithError(err).Debug("image not built yet")
	} else {
		log.WithError(err).Warn("cannot pull image")
	}

	err = buildImage(cfg, buildctxFn, tag, opts)
	if err != nil {
		return false, err
	}

	return false, nil
}

func buildImage(cfg BuildConfig, buildctxFn, tag string, opts types.ImageBuildOptions) error {
	env := cfg.Env
	termFd, isTerm := term.GetFdInfo(env.Out)
	buildctx, err := os.OpenFile(buildctxFn, os.O_RDONLY, 0644)
	if err != nil {
		return err
	}

	opts.Tags = append(opts.Tags, tag)
	bresp, err := env.Client.ImageBuild(env.Context, buildctx, opts)
	if err != nil {
		return err
	}
	err = jsonmessage.DisplayJSONMessagesStream(bresp.Body, env.Out(), termFd, isTerm, nil)
	if err != nil {
		return err
	}
	bresp.Body.Close()
	buildctx.Close()

	return nil
}

func pushImage(cfg BuildConfig, tag string) error {
	env := cfg.Env
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

	termFd, isTerm := term.GetFdInfo(env.Out)
	err = jsonmessage.DisplayJSONMessagesStream(presp, env.Out(), termFd, isTerm, nil)
	if err != nil {
		return err
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
	Test      string
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

		if next.Value == labelTest {
			name := next.Next.Value
			if len(name) == 0 {
				return nil, fmt.Errorf("invalid dazzle test name in line %d", tkn.StartLine)
			}

			cur.Test = name
		}

		if next.Value != labelLayer {
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

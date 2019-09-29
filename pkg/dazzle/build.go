package dazzle

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/mholt/archiver"
	"github.com/moby/buildkit/frontend/dockerfile/command"
	"github.com/moby/buildkit/frontend/dockerfile/parser"
)

// Build builds a Dockerfile with independent layers
func Build(env *Environment, loc, dockerfile, dst string) error {
	fullDFN := filepath.Join(loc, dockerfile)

	// compute splitpoints
	sps, err := findSplitPoints(fullDFN)
	if err != nil {
		return err
	}

	// split off base dockerfile
	var builds []string
	baseSPS := sps[0]
	bd, err := splitDockerfile(fullDFN, baseSPS, "")
	if err != nil {
		return err
	}
	builds = append(builds, bd)

	// create build context
	fns, err := ioutil.ReadDir(loc)
	if err != nil {
		return err
	}
	var buildctxCtnt []string
	for _, bfi := range fns {
		buildctxCtnt = append(buildctxCtnt, filepath.Join(loc, bfi.Name()))
	}
	buildctxFn := filepath.Join(env.Workdir, "build-context.tar.gz")
	os.Remove(buildctxFn)
	err = archiver.Archive(buildctxCtnt, buildctxFn)
	if err != nil {
		return err
	}
	fmt.Printf("created build context in %s\n", buildctxFn)

	// build base image
	buildctx, err := os.OpenFile(buildctxFn, os.O_RDONLY, 0644)
	if err != nil {
		return err
	}
	baseImgName := fmt.Sprintf("dazzle-base:%x", sha256.Sum256([]byte(dst)))
	bresp, err := env.Client.ImageBuild(env.Context, buildctx, types.ImageBuildOptions{
		Tags:       []string{baseImgName},
		PullParent: true,
		Dockerfile: bd,
	})
	if err != nil {
		return err
	}
	err = jsonmessage.DisplayJSONMessagesStream(bresp.Body, env.Out, 0, false, nil)
	if err != nil {
		return err
	}
	bresp.Body.Close()
	buildctx.Close()

	// build addons
	addons := sps[1:]
	for _, sp := range addons {
		bd, err := splitDockerfile(fullDFN, sp, baseImgName)

		if err != nil {
			return err
		}
		builds = append(builds, bd)
	}

	// build addons
	var buildNames []string
	for _, bd := range builds {
		buildctx, err := os.OpenFile(buildctxFn, os.O_RDONLY, 0644)
		if err != nil {
			return err
		}
		buildName := fmt.Sprintf("dazzle-build:%x", sha256.Sum256([]byte(bd)))
		bresp, err := env.Client.ImageBuild(env.Context, buildctx, types.ImageBuildOptions{
			Tags:       []string{buildName},
			PullParent: false,
			Dockerfile: bd,
		})
		if err != nil {
			return err
		}
		err = jsonmessage.DisplayJSONMessagesStream(bresp.Body, env.Out, 0, false, nil)
		if err != nil {
			return err
		}
		bresp.Body.Close()
		buildctx.Close()

		buildNames = append(buildNames, buildName)
	}

	// merge the whole thing
	return MergeImages(env, dst, baseImgName, buildNames...)
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

func splitDockerfile(loc string, sp splitPoint, from string) (fn string, err error) {
	fn = fmt.Sprintf("dazzle__%s.Dockerfile", sp.Name)

	wd := filepath.Dir(loc)
	fc, err := ioutil.ReadFile(loc)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(fc), "\n")

	var ctnt []string
	if from != "" {
		ctnt = append(ctnt, fmt.Sprintf("FROM %s", from))
	}
	ctnt = append(ctnt, lines[sp.StartLine:sp.EndLine]...)
	err = ioutil.WriteFile(filepath.Join(wd, fn), []byte(strings.Join(ctnt, "\n")), 0644)
	if err != nil {
		return "", err
	}

	return
}

func findSplitPoints(dockerfileLoc string) (splitpoints []splitPoint, err error) {
	df, err := os.OpenFile(dockerfileLoc, os.O_RDONLY, 0644)
	if err != nil {
		return nil, err
	}
	defer df.Close()
	res, err := parser.Parse(df)
	if err != nil {
		return nil, err
	}

	var (
		sps []splitPoint
		cur splitPoint
	)
	cur = splitPoint{
		StartLine: 0,
		Name:      "_base",
	}
	for _, tkn := range res.AST.Children {
		if tkn.Value != command.Label {
			cur.EndLine = tkn.EndLine
			continue
		}
		next := tkn.Next
		if next == nil {
			cur.EndLine = tkn.EndLine
			continue
		}
		if next.Value != "dazzle/layer" {
			cur.EndLine = tkn.EndLine
			continue
		}

		name := next.Next.Value
		if len(name) == 0 {
			return nil, fmt.Errorf("invalid dazzle layer name in line %d", tkn.StartLine)
		}
		sps = append(sps, cur)
		cur = splitPoint{
			Name:      name,
			StartLine: tkn.StartLine,
		}
	}
	sps = append(sps, cur)
	return sps, nil
}

type splitPoint struct {
	StartLine int
	EndLine   int
	Name      string
}

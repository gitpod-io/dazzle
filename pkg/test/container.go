package test

import (
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/auth/authprovider"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// NewBuildkitExecutor creates a new buildkit-backed executor
func NewBuildkitExecutor(cl *client.Client, ref string) *BuildkitExecutor {
	return &BuildkitExecutor{
		cl:  cl,
		ref: ref,
	}
}

// BuildkitExecutor runs tests in containers using buildkit
type BuildkitExecutor struct {
	cl  *client.Client
	ref string
}

// Run executes the test
func (b *BuildkitExecutor) Run(ctx context.Context, spec *Spec) (rr *RunResult, err error) {
	var opts []llb.RunOption
	for _, e := range spec.Env {
		seg := strings.Split(e, "=")
		k, v := seg[0], seg[1]
		opts = append(opts, llb.AddEnv(k, v))
	}
	opts = append(opts, llb.Args(append([]string{"/bin/run.sh"}, spec.Command...)))
	opts = append(opts, llb.IgnoreCache)

	runnerScript := []byte(`#!/bin/sh
$* > /tmp/stdout 2> /tmp/stderr
c=$?

echo "DAZZLE_EXIT_CODE"
echo $c
echo "DAZZLE_STDOUT"
cat /tmp/stdout
echo "DAZZLE_STDERR"
cat /tmp/stderr
`)
	def, err := llb.Image(b.ref).
		File(llb.Mkfile("/bin/run.sh", 0777, runnerScript)).
		Run(opts...).
		Root().
		Marshal()
	if err != nil {
		return
	}

	var (
		cctx, cancel = context.WithCancel(ctx)
		ch           = make(chan *client.SolveStatus)
		eg, bctx     = errgroup.WithContext(cctx)
		rchan        = make(chan []byte, 1)
	)
	defer cancel()
	eg.Go(func() error {
		_, err := b.cl.Solve(bctx, def, client.SolveOpt{
			Session: []session.Attachable{
				authprovider.NewDockerAuthProvider(os.Stderr),
			},
		}, ch)
		if err != nil {
			return err
		}
		return nil
	})
	eg.Go(func() error {
		var b []byte
		defer func() {
			rchan <- b
		}()

		for {
			select {
			case cs, ok := <-ch:
				if !ok {
					return nil
				}

				for _, l := range cs.Logs {
					b = append(b, l.Data...)
				}
			case <-ctx.Done():
				return nil
			}
		}
	})
	err = eg.Wait()
	if err != nil {
		return
	}

	buf := <-rchan
	log.WithField("buf", string(buf)).Debug("received test run output")
	return extractRunResult(buf)
}

func extractRunResult(buf []byte) (res *RunResult, err error) {
	r := &RunResult{}
	mode := ""
	for _, l := range strings.Split(string(buf), "\n") {
		switch strings.TrimSpace(l) {
		case "DAZZLE_EXIT_CODE":
			mode = "ec"
			continue
		case "DAZZLE_STDOUT":
			mode = "stdout"
			continue
		case "DAZZLE_STDERR":
			mode = "stderr"
			continue
		}

		switch mode {
		case "ec":
			r.StatusCode, err = strconv.ParseInt(strings.TrimSpace(l), 10, 64)
			if err != nil {
				return
			}
		case "stdout":
			r.Stdout = append(r.Stdout, []byte(l)...)
		case "stderr":
			r.Stderr = append(r.Stderr, []byte(l)...)
		}
	}
	return r, nil
}

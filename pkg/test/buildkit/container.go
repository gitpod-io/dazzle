package buildkit

import (
	"context"
	"os"
	"strings"

	"github.com/gitpod-io/dazzle/pkg/test"
	"github.com/gitpod-io/dazzle/pkg/test/runner"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/auth/authprovider"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// NewExecutor creates a new buildkit-backed executor
func NewExecutor(cl *client.Client, ref string, cfg *ociv1.Image) *Executor {
	return &Executor{
		cl:  cl,
		ref: ref,
		cfg: cfg,
	}
}

// Executor runs tests in containers using buildkit
type Executor struct {
	cl  *client.Client
	ref string
	cfg *ociv1.Image
}

// Run executes the test
func (b *Executor) Run(ctx context.Context, spec *test.Spec) (rr *test.RunResult, err error) {
	rb, err := runner.GetRunner("linux_amd64")
	if err != nil {
		return
	}
	espec, err := runner.Args(spec)
	if err != nil {
		return
	}

	state := llb.Image(b.ref)
	if user := b.cfg.Config.User; user != "" {
		state = state.User(user)
		log.WithField("user", user).Debug("running test as user")
	}
	for _, e := range b.cfg.Config.Env {
		segs := strings.Split(e, "=")
		state = state.AddEnv(segs[0], segs[1])
	}
	def, err := state.
		File(llb.Mkdir("/dazzle", 0755)).
		File(llb.Mkfile("/dazzle/runner", 0777, rb)).
		Run(llb.Args(append([]string{"/dazzle/runner"}, espec...)), llb.IgnoreCache).
		Root().
		Marshal(ctx)
	if err != nil {
		return
	}

	log.WithField("args", espec).Debug("running test using buildkit")
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
		log.WithField("solvestatuserr", err).Debug("solve status")
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
		log.WithError(err).Info("ignored error group error")
	}

	buf := <-rchan
	log.WithField("buf", string(buf)).Debug("received test run output")
	res, err := runner.UnmarshalRunResult(buf)
	if err != nil {
		return
	}
	return res, nil
}

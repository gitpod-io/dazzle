// Copyright © 2020 Christian Weichel

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package dazzle

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/csweichel/dazzle/pkg/test"
	"github.com/csweichel/dazzle/pkg/test/buildkit"
	"github.com/docker/distribution/reference"
	"github.com/moby/buildkit/client"
	"github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	log "github.com/sirupsen/logrus"
)

type combinerOpts struct {
	BuildkitClient *client.Client
	RunTests       bool
	TempBuild      bool
}

// CombinerOpt configrues the combiner
type CombinerOpt func(*combinerOpts) error

// WithTests enable tests after image combination
func WithTests(cl *client.Client) CombinerOpt {
	return func(o *combinerOpts) error {
		o.BuildkitClient = cl
		o.RunTests = true
		return nil
	}
}

func asTempBuild(o *combinerOpts) error {
	o.TempBuild = true
	return nil
}

// Combine combines a set of previously built chunks into a single image while maintaining
// the layer identity.
func (p *Project) Combine(ctx context.Context, chunks []string, dest reference.Named, sess *BuildSession, opts ...CombinerOpt) (err error) {
	var options combinerOpts
	for _, o := range opts {
		err = o(&options)
		if err != nil {
			return
		}
	}

	if options.RunTests && !options.TempBuild {
		// We have to push the combination result. To avoid overwriting the target but have the tests fail
		// we combine and test with a temp name first, then do the real thing.
		tmpdest, err := reference.WithTag(dest, fmt.Sprintf("temp%d", time.Now().Unix()))
		if err != nil {
			return err
		}
		err = p.Combine(ctx, chunks, tmpdest, sess, append(opts, asTempBuild)...)
		if err != nil {
			return err
		}

		options.RunTests = false
	}

	cs := make([]ProjectChunk, len(chunks))
	for i, cn := range chunks {
		var found bool
		for _, c := range p.Chunks {
			if c.Name == cn {
				cs[i] = c
				found = true
				break
			}
		}

		if !found {
			return fmt.Errorf("chunk %s not found", cn)
		}
	}

	var (
		mfs  = make([]*ociv1.Manifest, 0, len(chunks)+1)
		cfgs = make([]*ociv1.Image, 0, len(chunks)+1)
	)

	basemf, basecfg := sess.baseMF, sess.baseCfg
	if basemf == nil || basecfg == nil {
		return fmt.Errorf("base image not resolved")
	}

	mfs = append(mfs, basemf)
	cfgs = append(cfgs, basecfg)

	for _, c := range cs {
		cref, err := c.ImageName(ImageTypeChunked, sess)
		if err != nil {
			return err
		}
		log.WithField("ref", cref.String()).Info("pulling chunk metadata")
		_, mf, cfg, err := getImageMetadata(ctx, cref, sess.opts.Resolver)
		if err != nil {
			return err
		}
		mfs = append(mfs, mf)
		cfgs = append(cfgs, cfg)
	}

	var (
		allLayer []ociv1.Descriptor
		allDiffs []digest.Digest
		allHist  []ociv1.History
	)
	for i, m := range mfs {
		allLayer = append(allLayer, m.Layers...)
		allDiffs = append(allDiffs, cfgs[i].RootFS.DiffIDs...)
		allHist = append(allHist, cfgs[i].History...)
	}

	env, err := mergeEnv(basecfg, cfgs)
	if err != nil {
		return
	}

	now := time.Now()
	ccfg := ociv1.Image{
		Created:      &now,
		Architecture: basecfg.Architecture,
		History:      allHist,
		OS:           basecfg.OS,
		Config: ociv1.ImageConfig{
			StopSignal:   basecfg.Config.StopSignal,
			Cmd:          basecfg.Config.Cmd,
			Entrypoint:   basecfg.Config.Entrypoint,
			ExposedPorts: mergeExposedPorts(basecfg, cfgs),
			Env:          env,
			// Labels:       mergeLabels(basecfg, cfgs),
			User: basecfg.Config.User,
			// Volumes:      mergeVolumes(basecfg, cfgs),
			WorkingDir: basecfg.Config.WorkingDir,
		},
		RootFS: ociv1.RootFS{
			Type:    basecfg.RootFS.Type,
			DiffIDs: allDiffs,
		},
	}
	serializedCcfg, err := json.Marshal(ccfg)
	if err != nil {
		return
	}
	ccfgdesc := ociv1.Descriptor{
		MediaType: ociv1.MediaTypeImageConfig,
		Digest:    digest.FromBytes(serializedCcfg),
		Size:      int64(len(serializedCcfg)),
	}
	log.WithField("content", string(serializedCcfg)).Debug("produced config")

	cmf := ociv1.Manifest{
		Versioned:   basemf.Versioned,
		Annotations: mergeAnnotations(basemf, mfs),
		Config:      ccfgdesc,
		Layers:      allLayer,
	}
	serializedMf, err := json.Marshal(cmf)
	if err != nil {
		return
	}
	cmfdesc := ociv1.Descriptor{
		MediaType: ociv1.MediaTypeImageManifest,
		Digest:    digest.FromBytes(serializedMf),
		Size:      int64(len(serializedMf)),
		Platform:  basemf.Config.Platform,
	}
	log.WithField("content", string(serializedMf)).Debug("produced manifest")

	log.WithField("dest", dest.String()).Info("pushing combined image")
	pusher, err := sess.opts.Resolver.Pusher(ctx, dest.String())
	if err != nil {
		return
	}
	ccfgw, err := pusher.Push(ctx, ccfgdesc)
	if err != nil {
		return
	}
	ccfgw.Write(serializedCcfg)
	err = ccfgw.Commit(ctx, cmf.Config.Size, cmf.Config.Digest)
	if err != nil {
		return
	}
	mfw, err := pusher.Push(ctx, cmfdesc)
	mfw.Write(serializedMf)
	err = mfw.Commit(ctx, int64(len(serializedMf)), cmfdesc.Digest)
	if err != nil {
		return err
	}

	if options.RunTests {
		for _, chk := range cs {
			if len(chk.Tests) == 0 {
				continue
			}

			executor := buildkit.NewExecutor(options.BuildkitClient, dest.String(), &ccfg)
			_, ok := test.RunTests(ctx, executor, chk.Tests)
			if !ok {
				return fmt.Errorf("tests failed")
			}
		}

	}

	return
}

func mergeAnnotations(base *ociv1.Manifest, others []*ociv1.Manifest) map[string]string {
	res := make(map[string]string)
	for k, v := range base.Annotations {
		res[k] = v
	}
	for _, m := range others {
		for k, v := range m.Annotations {
			if _, ok := res[k]; ok {
				continue
			}
			res[k] = v
		}
	}
	return res
}

func mergeExposedPorts(base *ociv1.Image, others []*ociv1.Image) map[string]struct{} {
	res := make(map[string]struct{})
	for k, v := range base.Config.ExposedPorts {
		res[k] = v
	}
	for _, m := range others {
		for k, v := range m.Config.ExposedPorts {
			if _, ok := res[k]; ok {
				continue
			}
			res[k] = v
		}
	}
	return res
}

func mergeEnv(base *ociv1.Image, others []*ociv1.Image) ([]string, error) {
	envs := make(map[string]string)
	for _, e := range base.Config.Env {
		segs := strings.Split(e, "=")
		if len(segs) != 2 {
			return nil, fmt.Errorf("env var %s in invalid", e)
		}
		envs[segs[0]] = segs[1]
	}

	for _, m := range others {
		for _, e := range m.Config.Env {
			segs := strings.Split(e, "=")
			if len(segs) != 2 {
				return nil, fmt.Errorf("env var %s in invalid", e)
			}

			k, v := segs[0], segs[1]
			if ov, ok := envs[k]; ok {
				ov += ";" + v
				envs[k] = ov
				continue
			}
			envs[k] = v
		}
	}

	var (
		res = make([]string, len(envs))
		i   = 0
	)
	for k, v := range envs {
		res[i] = fmt.Sprintf("%s=%s", k, v)
		i++
	}
	return res, nil
}

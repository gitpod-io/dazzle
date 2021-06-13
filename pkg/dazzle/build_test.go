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
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	_ "github.com/bshuster-repo/logrus-logstash-hook"
	"github.com/containerd/containerd/remotes"
	"github.com/containerd/containerd/remotes/docker"
	"github.com/csweichel/dazzle/pkg/solve"
	_ "github.com/distribution/distribution/registry/storage/driver/inmemory"
	"github.com/docker/distribution/configuration"
	"github.com/docker/distribution/reference"
	"github.com/docker/distribution/registry"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

func setupRegistry(ctx context.Context, addr string) (*registry.Registry, error) {
	config := &configuration.Configuration{}
	// TODO: this needs to change to something ephemeral as the test will fail if there is any server
	// already listening on the specified port
	config.HTTP.Secret = "not_a_secret"
	config.HTTP.Addr = addr
	config.HTTP.DrainTimeout = time.Duration(10) * time.Second
	config.Storage = map[string]configuration.Parameters{"inmemory": map[string]interface{}{}}
	return registry.NewRegistry(ctx, config)
}

func TestProjectChunk_test(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	registry, err := setupRegistry(ctx, "127.0.0.1:5111")
	if err != nil {
		t.Fatal(err)
	}
	// run registry server
	var errchan chan error
	go func() {
		errchan <- registry.ListenAndServe()
	}()
	select {
	case err = <-errchan:
		t.Fatalf("Error listening: %v", err)
	default:
	}

	sess, err := NewSession(nil, "127.0.0.1:5111/test_projectchunk")
	if err != nil {
		t.Errorf("could not create session:%v", err)
	}
	resolver := docker.NewResolver(docker.ResolverOptions{})
	reg := NewResolverRegistry(resolver)
	sess.opts.Resolver = resolver

	type fields struct {
		Name     string
		FS       map[string]*fstest.MapFile
		Base     string
		Chunk    string
		BaseRef  string
		Registry Registry
	}
	type args struct {
		ctx  context.Context
		sess *BuildSession
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantOk  bool
		wantErr bool
	}{
		// 		{
		// 			name: "passes with no tests",
		// 			fields: fields{
		// 				Name:  "no test chunk",
		// 				Base:  "chunks",
		// 				Chunk: "notest",
		// 				FS: map[string]*fstest.MapFile{
		// 					"chunks/notest/Dockerfile": {
		// 						Data: []byte("FROM alpine"),
		// 					},
		// 				},
		// 			},
		// 			args: args{
		// 				ctx:  ctx,
		// 				sess: sess,
		// 			},
		// 			wantOk:  true,
		// 			wantErr: false,
		// 		},
		// 		{
		// 			name: "fails when no base reference set",
		// 			fields: fields{
		// 				Name:  "no base ref chunk",
		// 				Base:  "chunks",
		// 				Chunk: "nobaseref",
		// 				FS: map[string]*fstest.MapFile{
		// 					"chunks/nobaseref/Dockerfile": {
		// 						Data: []byte("FROM alpine"),
		// 					},
		// 					"tests/nobaseref.yaml": {
		// 						Data: []byte(`---
		// - desc: "it should run ls"
		//   command: ["ls"]
		//   assert:
		//   - "status == 0"
		// `),
		// 					},
		// 				},
		// 			},
		// 			args: args{
		// 				ctx:  ctx,
		// 				sess: sess,
		// 			},
		// 			wantOk:  false,
		// 			wantErr: true,
		// 		},
		// 		{
		// 			name: "does not build if tests have passed",
		// 			fields: fields{
		// 				Name:  "a chunk",
		// 				Base:  "chunks",
		// 				Chunk: "foobar",
		// 				FS: map[string]*fstest.MapFile{
		// 					"chunks/foobar/Dockerfile": {
		// 						Data: []byte("FROM alpine"),
		// 					},
		// 					"tests/foobar.yaml": {
		// 						Data: []byte(`---
		// - desc: "it should run ls"
		//   command: ["ls"]
		//   assert:
		//   - "status == 0"
		// `),
		// 					},
		// 				},
		// 				Registry: testRegistry{
		// 					testResult: &StoredTestResult{
		// 						Passed: true,
		// 					},
		// 				},
		// 				BaseRef: "localhost:5111/test@sha256:b25ab047a146b43a7a1bdd2b3346a05fd27dd2730af8ab06a9b8acca0f15b378",
		// 			},
		// 			args: args{
		// 				ctx:  ctx,
		// 				sess: sess,
		// 			},
		// 			wantOk:  true,
		// 			wantErr: false,
		// 		},
		{
			name: "builds if not present",
			fields: fields{
				Name:     "a chunk",
				Base:     "chunks",
				Chunk:    "basic",
				Registry: reg,
				BaseRef:  "127.0.0.1:5111/test_projectchunk@sha256:b25ab047a146b43a7a1bdd2b3346a05fd27dd2730af8ab06a9b8acca0f15b378",
			},
			args: args{
				ctx:  ctx,
				sess: sess,
			},
			wantOk:  true,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var filesys fs.FS
			if tt.fields.FS != nil {
				filesys = fstest.MapFS(tt.fields.FS)
			} else {
				wd, err := os.Getwd()
				if err != nil {
					panic(err)
				}
				filesys = os.DirFS(wd + "/testdata")
			}
			chks, err := loadChunks(filesys, "testdata", tt.fields.Base, tt.fields.Chunk)
			if err != nil {
				t.Errorf("could not load chunks:%v", err)
				return
			}
			if len(chks) != 1 {
				t.Error("can only support 1 chunk")
				return
			}
			if tt.fields.BaseRef != "" {
				baseRef, err := reference.Parse(tt.fields.BaseRef)
				if err != nil {
					t.Errorf("could not parse baseRef:%s", tt.fields.BaseRef)
					return
				}
				digested, ok := baseRef.(reference.Digested)
				if !ok {
					t.Errorf("not a digest baseRef:%s", tt.fields.BaseRef)
				}
				tt.args.sess.baseRef = digested
			}
			if tt.fields.Registry != nil {
				sess.opts.Registry = tt.fields.Registry
			}
			gotOk, err := chks[0].test(tt.args.ctx, tt.args.sess)
			if (err != nil) != tt.wantErr {
				t.Errorf("ProjectChunk.test() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotOk != tt.wantOk {
				t.Errorf("ProjectChunk.test() = %v, want %v", gotOk, tt.wantOk)
			}
		})
	}
}

func TestProjectChunk_test_builds_if_not_present(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	registry, err := setupRegistry(ctx, "127.0.0.1:5111")
	if err != nil {
		t.Fatal(err)
	}
	// run registry server
	var errchan chan error
	go func() {
		errchan <- registry.ListenAndServe()
	}()
	select {
	case err = <-errchan:
		t.Fatalf("Error listening: %v", err)
	default:
	}

	sess, err := NewSession(NewFakeSolver(
		&client.SolveResponse{
			ExporterResponse: map[string]string{
				"containerimage.config.digest" : "sha256:455236b3a96eb95d7b7ccaa1c5073b7efb676b8146d7fcbba5013554d814efd4",
				"containerimage.digest": "sha256:0eb1357cb23f1577a56fac66942a7f785a27ceb9574a39d5079e4e07a6a8d70f",
				"image.name" : "localhost:5111/dazzle:base--f38f08be1b469c1b5e083e5e64104462344fe8843ab103a4ce5d2bfd7c09619e",
			},
		},
	), "127.0.0.1:5111/test_projectchunk")
	if err != nil {
		t.Errorf("could not create session:%v", err)
	}
	resolver := docker.NewResolver(docker.ResolverOptions{})
	reg := NewResolverRegistry(resolver)
	sess.opts.Resolver = resolver
	sess.opts.Registry = reg
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	filesys := os.DirFS(wd + "/testdata")
	chks, err := loadChunks(filesys, "testdata", "chunks", "basic")
	if err != nil {
		t.Errorf("could not load chunks:%v", err)
		return
	}
	if len(chks) != 1 {
		t.Error("can only support 1 chunk")
		return
	}
	// Should fail if no base ref set
	gotOk, err := chks[0].test(ctx, sess)
	if gotOk || (err != nil && !strings.Contains(err.Error(), "base ref not")) {
		t.Errorf("TestProjectChunk_test_builds_if_not_present() unexpected result: %v or error = %v", gotOk, err)
		return
	}

	// TODO(rl): validate the base image
	// // Should handle if invalid base ref set
	// invalidBaseRef := "127.0.0.1:5111/test_projectchunk@sha256:b25ab047a146b43a7a1bdd2b3346a05fd27dd2730af8ab06a9b8acca0f15b378"
	// baseRef, err := reference.Parse(invalidBaseRef)
	// if err != nil {
	// 	t.Errorf("could not parse baseRef:%s", invalidBaseRef)
	// 	return
	// }
	// digested, ok := baseRef.(reference.Digested)
	// if !ok {
	// 	t.Errorf("not a digest baseRef:%s", invalidBaseRef)
	// }
	// sess.baseRef = digested

	// gotOk, err = chks[0].test(ctx, sess)
	// if gotOk || (err != nil && !strings.Contains(err.Error(), "base ref not")) {
	// 	t.Errorf("TestProjectChunk_test_builds_if_not_present() unexpected result: %v or error = %v", gotOk, err)
	// 	return
	// }

	prj, err := LoadFromDir("testdata", LoadFromDirOpts{})
	if err != nil {
		t.Errorf("TestProjectChunk_test_builds_if_not_present() could not load project: %v", err)
		return
	}
	err = prj.BuildBase(ctx, sess)
	if err != nil {
		t.Errorf("TestProjectChunk_test_builds_if_not_present() could not build base: %v", err)
		return
	}
	gotOk, err = chks[0].test(ctx, sess)
	if !gotOk || err != nil {
		t.Errorf("TestProjectChunk_test_builds_if_not_present() unexpected result: %v or error = %v", gotOk, err)
		return
	}
}

type testRegistry struct {
	testResult *StoredTestResult
}

func (t testRegistry) Push(ctx context.Context, ref reference.Named, opts storeInRegistryOptions) (absref reference.Digested, err error) {
	return nil, nil
}

func (t testRegistry) Pull(ctx context.Context, ref reference.Reference, cfg interface{}) (manifest *ociv1.Manifest, absref reference.Digested, err error) {
	if t.testResult != nil {
		r := cfg.(*StoredTestResult)
		r.Passed = t.testResult.Passed
	}
	return nil, nil, nil
}

type testResolver struct{}

func (t testResolver) Resolve(ctx context.Context, ref string) (name string, desc ocispec.Descriptor, err error) {
	return "test", ocispec.Descriptor{}, nil
}

func (t testResolver) Fetcher(ctx context.Context, ref string) (remotes.Fetcher, error) {
	return nil, nil
}

func (t testResolver) Pusher(ctx context.Context, ref string) (remotes.Pusher, error) {
	return nil, nil
}

type fakeSolver struct {
	resp *client.SolveResponse
}

func NewFakeSolver(resp * client.SolveResponse) solve.Solver {
	return fakeSolver{
		resp,
	}
}

func (c fakeSolver) Solve(ctx context.Context, def *llb.Definition, opt client.SolveOpt, statusChan chan *client.SolveStatus) (*client.SolveResponse, error) {
	return c.resp, nil
}
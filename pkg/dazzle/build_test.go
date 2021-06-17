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
	"testing"
	"testing/fstest"

	"github.com/containerd/containerd/remotes"
	"github.com/docker/distribution/reference"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

func TestProjectChunk_test(t *testing.T) {
	ctx := context.Background()
	sess, err := NewSession(nil, "localhost:9999/test")
	if err != nil {
		t.Errorf("could not create session:%v", err)
	}
	sess.opts.Resolver = testResolver{}

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
		{
			name: "passes with no tests",
			fields: fields{
				Name:  "no test chunk",
				Base:  "chunks",
				Chunk: "notest",
				FS: map[string]*fstest.MapFile{
					"chunks/notest/Dockerfile": {
						Data: []byte("FROM alpine"),
					},
				},
			},
			args: args{
				ctx:  ctx,
				sess: sess,
			},
			wantOk:  true,
			wantErr: false,
		},
		{
			name: "fails when no base reference set",
			fields: fields{
				Name:  "no base ref chunk",
				Base:  "chunks",
				Chunk: "nobaseref",
				FS: map[string]*fstest.MapFile{
					"chunks/nobaseref/Dockerfile": {
						Data: []byte("FROM alpine"),
					},
					"tests/nobaseref.yaml": {
						Data: []byte(`---
- desc: "it should run ls"
  command: ["ls"]
  assert:
  - "status == 0"
`),
					},
				},
			},
			args: args{
				ctx:  ctx,
				sess: sess,
			},
			wantOk:  false,
			wantErr: true,
		},
		{
			name: "does not build if tests have passed",
			fields: fields{
				Name:  "a chunk",
				Base:  "chunks",
				Chunk: "foobar",
				FS: map[string]*fstest.MapFile{
					"chunks/foobar/Dockerfile": {
						Data: []byte("FROM alpine"),
					},
					"tests/foobar.yaml": {
						Data: []byte(`---
- desc: "it should run ls"
  command: ["ls"]
  assert:
  - "status == 0"
`),
					},
				},
				Registry: testRegistry{
					testResult: &StoredTestResult{
						Passed: true,
					},
				},
				BaseRef: "localhost:9999/test@sha256:b25ab047a146b43a7a1bdd2b3346a05fd27dd2730af8ab06a9b8acca0f15b378",
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
			chks, err := loadChunks(fstest.MapFS(tt.fields.FS), "", tt.fields.Base, tt.fields.Chunk)
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

type testRegistry struct {
	testResult *StoredTestResult
}

func (t testRegistry) Push(ctx context.Context, ref reference.Named, opts storeInRegistryOptions) (absref reference.Canonical, err error) {
	return nil, nil
}

func (t testRegistry) Pull(ctx context.Context, ref reference.Reference, cfg interface{}) (manifest *ociv1.Manifest, absref reference.Canonical, err error) {
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

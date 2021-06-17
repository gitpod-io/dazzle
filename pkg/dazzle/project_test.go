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
	"fmt"
	"testing"
	"testing/fstest"

	"github.com/containerd/containerd/errdefs"
	"github.com/docker/distribution/reference"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"gopkg.in/yaml.v2"
)

func TestLoadChunk(t *testing.T) {
	type Expectation struct {
		Err    string
		Chunks []ProjectChunk
	}
	var tests = []struct {
		Name        string
		FS          map[string]*fstest.MapFile
		Base        string
		Chunk       string
		Expectation Expectation
	}{
		{
			Name:  "load base",
			Chunk: "base",
			FS: map[string]*fstest.MapFile{
				"base/Dockerfile": {
					Data: []byte("FROM alpine"),
				},
			},
			Expectation: Expectation{
				Chunks: []ProjectChunk{
					{
						Name:        "base",
						ContextPath: "base",
						Dockerfile:  []byte("FROM alpine"),
					},
				},
			},
		},
		{
			Name:  "load chunk",
			Base:  "chunks",
			Chunk: "foobar",
			FS: map[string]*fstest.MapFile{
				"chunks/foobar/Dockerfile": {
					Data: []byte("FROM alpine"),
				},
			},
			Expectation: Expectation{
				Chunks: []ProjectChunk{
					{
						Name:        "foobar",
						ContextPath: "chunks/foobar",
						Dockerfile:  []byte("FROM alpine"),
					},
				},
			},
		},
		{
			Name:  "load variant chunk",
			Base:  "chunks",
			Chunk: "foobar",
			FS: map[string]*fstest.MapFile{
				"chunks/foobar/Dockerfile": {
					Data: []byte("FROM foobar"),
				},
				"chunks/foobar/OtherDockerfile": {
					Data: []byte("FROM other"),
				},
				"chunks/foobar/chunk.yaml": {
					Data: []byte("variants:\n  - name: v1\n    args:\n      FOO: bar\n  - name: v2\n    args:\n      FOO: baz\n  - name: v3\n    args:\n      FOO: baz\n    dockerfile: OtherDockerfile"),
				},
			},
			Expectation: Expectation{
				Chunks: []ProjectChunk{
					{
						Name:        "foobar:v1",
						Dockerfile:  []byte("FROM foobar"),
						Args:        map[string]string{"FOO": "bar"},
						ContextPath: "chunks/foobar",
					},
					{
						Name:        "foobar:v2",
						Dockerfile:  []byte("FROM foobar"),
						Args:        map[string]string{"FOO": "baz"},
						ContextPath: "chunks/foobar",
					},
					{
						Name:        "foobar:v3",
						Dockerfile:  []byte("FROM other"),
						Args:        map[string]string{"FOO": "baz"},
						ContextPath: "chunks/foobar",
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			chk, err := loadChunks(fstest.MapFS(test.FS), "", test.Base, test.Chunk)
			var act Expectation
			if err != nil {
				act.Err = err.Error()
			} else {
				act.Chunks = chk
			}

			if diff := cmp.Diff(test.Expectation, act, cmp.AllowUnexported(ProjectChunk{})); diff != "" {
				t.Errorf("loadChunk() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestResolveCombinations(t *testing.T) {
	type Expectation struct {
		Err          string
		Combinations []ChunkCombination
	}
	var tests = []struct {
		Name       string
		Input      []ChunkCombination
		Expecation Expectation
	}{
		{
			Name:  "empty set",
			Input: nil,
			Expecation: Expectation{
				Combinations: []ChunkCombination{},
			},
		},
		{
			Name: "chunks only",
			Input: []ChunkCombination{
				{Name: "a", Chunks: []string{"a0", "a1"}},
			},
			Expecation: Expectation{
				Combinations: []ChunkCombination{
					{Name: "a", Chunks: []string{"a0", "a1"}},
				},
			},
		},
		{
			Name: "single combination ref",
			Input: []ChunkCombination{
				{Name: "a", Chunks: []string{"a0", "a1"}},
				{Name: "b", Chunks: []string{"b0"}, Ref: []string{"a"}},
			},
			Expecation: Expectation{
				Combinations: []ChunkCombination{
					{Name: "a", Chunks: []string{"a0", "a1"}},
					{Name: "b", Chunks: []string{"a0", "a1", "b0"}},
				},
			},
		},
		{
			Name: "transitive combination ref",
			Input: []ChunkCombination{
				{Name: "a", Chunks: []string{"a0", "a1"}},
				{Name: "b", Chunks: []string{"b0"}, Ref: []string{"a"}},
				{Name: "c", Chunks: []string{"c0"}, Ref: []string{"b"}},
			},
			Expecation: Expectation{
				Combinations: []ChunkCombination{
					{Name: "a", Chunks: []string{"a0", "a1"}},
					{Name: "b", Chunks: []string{"a0", "a1", "b0"}},
					{Name: "c", Chunks: []string{"a0", "a1", "b0", "c0"}},
				},
			},
		},
		{
			Name: "duplicate combination ref",
			Input: []ChunkCombination{
				{Name: "a", Chunks: []string{"a0", "a1"}},
				{Name: "b", Chunks: []string{"b0"}, Ref: []string{"a"}},
				{Name: "c", Chunks: []string{"c0"}, Ref: []string{"a"}},
			},
			Expecation: Expectation{
				Combinations: []ChunkCombination{
					{Name: "a", Chunks: []string{"a0", "a1"}},
					{Name: "b", Chunks: []string{"a0", "a1", "b0"}},
					{Name: "c", Chunks: []string{"a0", "a1", "c0"}},
				},
			},
		},
		{
			Name: "non-existent combination ref",
			Input: []ChunkCombination{
				{Name: "a", Chunks: []string{"a0"}, Ref: []string{"not-found"}},
			},
			Expecation: Expectation{
				Err: `unknown combination "not-found" referenced in "a"`,
			},
		},
		{
			Name: "cyclic combination ref",
			Input: []ChunkCombination{
				{Name: "a", Chunks: []string{"a0"}, Ref: []string{"b"}},
				{Name: "b", Chunks: []string{"b0"}, Ref: []string{"c"}},
				{Name: "c", Chunks: []string{"c0"}, Ref: []string{"a"}},
			},
			Expecation: Expectation{
				Combinations: []ChunkCombination{
					{Name: "a", Chunks: []string{"a0", "b0", "c0"}},
					{Name: "b", Chunks: []string{"a0", "b0", "c0"}},
					{Name: "c", Chunks: []string{"a0", "b0", "c0"}},
				},
			},
		},
		{
			Name: "cyclic self ref",
			Input: []ChunkCombination{
				{Name: "a", Chunks: []string{"a0"}, Ref: []string{"a"}},
			},
			Expecation: Expectation{
				Combinations: []ChunkCombination{{Name: "a", Chunks: []string{"a0"}}},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			res, err := resolveCombinations(test.Input)
			var act Expectation
			if err != nil {
				act.Err = err.Error()
			} else {
				act.Combinations = res
			}

			if diff := cmp.Diff(test.Expecation, act); diff != "" {
				t.Errorf("resolveCombinations() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestProjectChunk_hash(t *testing.T) {
	var tests = []struct {
		Name         string
		FS           map[string]*fstest.MapFile
		Base         string
		BaseRef      string
		Chunk        string
		IncludeTests bool
		Expectation  string
	}{
		{
			Name: "base only no tests",
			FS: map[string]*fstest.MapFile{
				"base/Dockerfile": {
					Data: []byte("FROM alpine"),
				},
			},
			Base:        "",
			BaseRef:     "",
			Chunk:       "base",
			Expectation: "550ccae3705ce9627190644ef89f404f94b8d6f9d13d8df537ca66080dd326b2",
		},
		{
			Name: "base with other tests should have same hash as no tests",
			FS: map[string]*fstest.MapFile{
				"base/Dockerfile": {
					Data: []byte("FROM alpine"),
				},
				"tests/notbase.yaml": {
					Data: []byte(`---
- desc: "it should run ls"
  command: ["ls"]
  assert:
  - "status == 0"
`),
				},
			},
			Base:        "",
			BaseRef:     "",
			Chunk:       "base",
			Expectation: "550ccae3705ce9627190644ef89f404f94b8d6f9d13d8df537ca66080dd326b2",
		},
		{
			Name: "base with tests should not have same hash as no tests if tests included",
			FS: map[string]*fstest.MapFile{
				"base/Dockerfile": {
					Data: []byte("FROM alpine"),
				},
				"tests/base.yaml": {
					Data: []byte(`---
- desc: "it should run ls"
  command: ["ls"]
  assert:
  - "status == 0"
`),
				},
			},
			Base:         "",
			BaseRef:      "",
			Chunk:        "base",
			Expectation:  "a557385d3e9d012dd179eaf7569850107c4af1adf8d99eb0fc402727827fab14",
			IncludeTests: true,
		},
		{
			Name: "base with changed test should have different hash if tests included",
			FS: map[string]*fstest.MapFile{
				"base/Dockerfile": {
					Data: []byte("FROM alpine"),
				},
				"tests/base.yaml": {
					Data: []byte(`---
- desc: "it should run pwd"
  command: ["pwd"]
  assert:
  - "status == 0"
`),
				},
			},
			Base:         "",
			BaseRef:      "",
			Chunk:        "base",
			Expectation:  "cf686202a95f644d3767667c6172b6b29c4d225db23bcc8d17aa4bdb42224b58",
			IncludeTests: true,
		},
		{
			Name: "base with tests should have same hash as no tests",
			FS: map[string]*fstest.MapFile{
				"base/Dockerfile": {
					Data: []byte("FROM alpine"),
				},
				"tests/base.yaml": {
					Data: []byte(`---
- desc: "it should run ls"
  command: ["ls"]
  assert:
  - "status == 0"
`),
				},
			},
			Base:        "",
			BaseRef:     "",
			Chunk:       "base",
			Expectation: "550ccae3705ce9627190644ef89f404f94b8d6f9d13d8df537ca66080dd326b2",
		},
		{
			Name:        "chunk only no tests",
			Base:        "chunks",
			BaseRef:     "",
			Chunk:       "foobar",
			Expectation: "fee0ceb7e0e5dd96ea24167ff3dc7fb31c88877cf165a37b1b35e6c7072e0993",
			FS: map[string]*fstest.MapFile{
				"chunks/foobar/Dockerfile": {
					Data: []byte("FROM ubuntu"),
				},
			},
		},
		{
			Name:        "chunk with tests should have same hash as no tests",
			Base:        "chunks",
			BaseRef:     "",
			Chunk:       "foobar",
			Expectation: "fee0ceb7e0e5dd96ea24167ff3dc7fb31c88877cf165a37b1b35e6c7072e0993",
			FS: map[string]*fstest.MapFile{
				"chunks/foobar/Dockerfile": {
					Data: []byte("FROM ubuntu"),
				},
				"tests/foobar.yal": {
					Data: []byte(`---
- desc: "it should run ls"
  command: ["ls"]
  assert:
  - "status == 0"
- desc: "it should run pwd"
  command: ["pwd"]
  assert:
  - "status == 0"
`),
				},
			},
		},
		{
			Name:        "chunk with tests should not have same hash as no tests if tests included",
			Base:        "chunks",
			BaseRef:     "",
			Chunk:       "foobar",
			Expectation: "f9e18ae354d33f5a9c317c89d7251ad323fb49b650031d5d5563a9693bcf2ae9",
			FS: map[string]*fstest.MapFile{
				"chunks/foobar/Dockerfile": {
					Data: []byte("FROM ubuntu"),
				},
				"tests/foobar.yal": {
					Data: []byte(`---
- desc: "it should run ls"
  command: ["ls"]
  assert:
  - "status == 0"
- desc: "it should run pwd"
  command: ["pwd"]
  assert:
  - "status == 0"
`),
				},
			},
			IncludeTests: true,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			chks, err := loadChunks(fstest.MapFS(test.FS), "", test.Base, test.Chunk)
			if err != nil {
				t.Errorf("could not load chunks: %v", err)
				return
			}
			if len(chks) != 1 {
				t.Error("can only test 1 chunk prohect")
				return
			}
			chk := chks[0]
			act, err := chk.hash(test.BaseRef, !test.IncludeTests)
			if err != nil {
				t.Errorf("could not compute hash: %v", err)
				return
			}
			if diff := cmp.Diff(test.Expectation, act); diff != "" {
				t.Errorf("hash() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLoadProjectFromRefs(t *testing.T) {
	imgs := map[string]mockRegistryImage{
		"gitpod.io/base:ref": {
			Manifest: &ociv1.Manifest{
				Annotations: map[string]string{
					mfAnnotationEnvVar + "foobar": string(EnvVarCombineMergeUnique),
					"ignored-annotation":          "something",
				},
			},
		},
		"gitpod.io/chunk:one": {
			Manifest: &ociv1.Manifest{
				Annotations: map[string]string{
					mfAnnotationBaseRef: "gitpod.io/base:ref",
				},
			},
		},
		"gitpod.io/chunk:two": {
			Manifest: &ociv1.Manifest{
				// use config to ensure this Manifest has a different content hash than gitpod.io/chunk:one
				Config: ociv1.Descriptor{
					Digest:    digest.FromString(""),
					Size:      0,
					MediaType: ociv1.MediaTypeImageConfig,
				},
				Annotations: map[string]string{
					mfAnnotationBaseRef: "gitpod.io/base:ref",
				},
			},
		},
		"gitpod.io/chunk:three": {
			Manifest: &ociv1.Manifest{
				Annotations: map[string]string{
					mfAnnotationBaseRef: "gitpod.io/other-base:ref",
				},
			},
		},
		"gitpod.io/chunk:no-base": {
			Manifest: &ociv1.Manifest{},
		},
		"gitpod.io/chunk:invalid-base": {
			Manifest: &ociv1.Manifest{
				Annotations: map[string]string{
					mfAnnotationBaseRef: "not a valid reference",
				},
			},
		},
	}
	for ref, img := range imgs {
		pref, err := reference.ParseNamed(ref)
		if err != nil {
			t.Fatalf("test fixture error: %v", err)
		}
		mf, err := yaml.Marshal(img.Manifest)
		if err != nil {
			t.Fatalf("test fixture error in %s: %v", ref, err)
		}
		img.AbsRef, err = reference.WithDigest(pref, digest.FromBytes(mf))
		if err != nil {
			t.Fatalf("test fixture error in %s: %v", ref, err)
		}
		imgs[ref] = img
	}

	type Expectation struct {
		Error   string
		Base    ProjectChunk
		Chunks  []ProjectChunk
		EnvVars []EnvVarCombination
	}

	tests := []struct {
		Name        string
		Images      map[string]mockRegistryImage
		Refs        []string
		Opts        LoadProjectFromRefsOpts
		Expectation Expectation
	}{
		{
			Name:   "happy path",
			Images: imgs,
			Refs:   []string{"gitpod.io/chunk:one", "gitpod.io/chunk:two"},
			Expectation: Expectation{
				Base:    ProjectChunk{Name: "base", Ref: imgs["gitpod.io/base:ref"].AbsRef},
				Chunks:  []ProjectChunk{{Name: "gitpod.io/chunk:one", Ref: imgs["gitpod.io/chunk:one"].AbsRef}, {Name: "gitpod.io/chunk:two", Ref: imgs["gitpod.io/chunk:two"].AbsRef}},
				EnvVars: []EnvVarCombination{{Name: "foobar", Action: EnvVarCombineMergeUnique}},
			},
		},
		{
			Name:   "ignore different base-refs",
			Images: imgs,
			Refs:   []string{"gitpod.io/chunk:one", "gitpod.io/chunk:three"},
			Opts: LoadProjectFromRefsOpts{
				IgnoreDifferingBaseRefs: true,
			},
			Expectation: Expectation{
				Base:    ProjectChunk{Name: "base", Ref: imgs["gitpod.io/base:ref"].AbsRef},
				Chunks:  []ProjectChunk{{Name: "gitpod.io/chunk:one", Ref: imgs["gitpod.io/chunk:one"].AbsRef}, {Name: "gitpod.io/chunk:three", Ref: imgs["gitpod.io/chunk:three"].AbsRef}},
				EnvVars: []EnvVarCombination{{Name: "foobar", Action: EnvVarCombineMergeUnique}},
			},
		},
		{
			Name:   "invalid base ref",
			Images: imgs,
			Refs:   []string{"gitpod.io/chunk:invalid-base"},
			Expectation: Expectation{
				Error: "cannot parse base ref not a valid reference: invalid reference format",
			},
		},
		{
			Name:   "unkown base ref",
			Images: imgs,
			Refs:   []string{"gitpod.io/chunk:three"},
			Expectation: Expectation{
				Error: "cannot download base ref metadata: not found",
			},
		},
		{
			Name:   "invalid chunk refs",
			Images: imgs,
			Refs:   []string{"not a valid chunk"},
			Expectation: Expectation{
				Error: "not a valid chunk: invalid reference format",
			},
		},
		{
			Name:   "chunk not found",
			Images: imgs,
			Refs:   []string{"gitpod.io/chunk-not-found:latest"},
			Expectation: Expectation{
				Error: "gitpod.io/chunk-not-found:latest: not found",
			},
		},
		{
			Name:   "chunk without base",
			Images: imgs,
			Refs:   []string{"gitpod.io/chunk:no-base"},
			Expectation: Expectation{
				Error: "chunk gitpod.io/chunk:no-base has no dazzle.gitpod.io/base-ref annotation - please build that chunk with an up-to-date version of dazzle",
			},
		},
		{
			Name:   "different base-refs",
			Images: imgs,
			Refs:   []string{"gitpod.io/chunk:one", "gitpod.io/chunk:three"},
			Expectation: Expectation{
				Error: "cannot combine chunks with different base images: chunks gitpod.io/chunk:one is based on gitpod.io/base:ref, while chunk gitpod.io/chunk:three is based on gitpod.io/other-base:ref",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			sess, err := NewSession(nil, "foobar.com/result", WithResolver(testResolver{}))
			if err != nil {
				t.Fatal(err)
			}
			sess.opts.Registry = mockRegistry{imgs}

			var act Expectation
			prj, err := LoadProjectFromRefs(context.Background(), sess, test.Refs, test.Opts)
			if err != nil {
				act = Expectation{Error: err.Error()}
			} else {
				act = Expectation{
					Base:    prj.Base,
					Chunks:  prj.Chunks,
					EnvVars: prj.Config.Combiner.EnvVars,
				}
			}

			if diff := cmp.Diff(test.Expectation, act, cmpopts.IgnoreUnexported(ProjectChunk{}, imgs["gitpod.io/base:ref"].AbsRef)); diff != "" {
				t.Errorf("LoadProjectFromRefs() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

type mockRegistryImage struct {
	Manifest *ociv1.Manifest
	Config   interface{}
	AbsRef   reference.Canonical
}

type mockRegistry struct {
	Images map[string]mockRegistryImage
}

func (t mockRegistry) Push(ctx context.Context, ref reference.Named, opts storeInRegistryOptions) (absref reference.Canonical, err error) {
	return nil, fmt.Errorf("not implemented")
}

func (t mockRegistry) Pull(ctx context.Context, ref reference.Reference, cfg interface{}) (manifest *ociv1.Manifest, absref reference.Canonical, err error) {
	img, exists := t.Images[ref.String()]
	if !exists {
		err = errdefs.ErrNotFound
		return
	}

	absref = img.AbsRef
	manifest = img.Manifest
	return
}

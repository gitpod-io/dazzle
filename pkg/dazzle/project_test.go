// Copyright © 2022 Gitpod

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
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"
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
		Expectation  map[string]string
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
			Expectation: map[string]string{"base": "02e46ef9c6d86deea6ffb67b6cd04a99e3600bb8d2c01f60359ed7a1ba2ed295"},
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
			Expectation: map[string]string{"base": "02e46ef9c6d86deea6ffb67b6cd04a99e3600bb8d2c01f60359ed7a1ba2ed295"},
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
			Expectation:  map[string]string{"base": "11f7021f65b55230c0e1105b1dc013d635a9a6d38e1476277df521400aec375a"},
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
			Expectation:  map[string]string{"base": "51ba9ff43996cf11afb5695b76b9e5d7c0134c83b27efc3063da8122069c4926"},
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
			Expectation: map[string]string{"base": "02e46ef9c6d86deea6ffb67b6cd04a99e3600bb8d2c01f60359ed7a1ba2ed295"},
		},
		{
			Name:        "chunk only no tests",
			Base:        "chunks",
			BaseRef:     "",
			Chunk:       "foobar",
			Expectation: map[string]string{"foobar": "6991b773b801a8eafb74dd95d5544d499ba1da5c9a677dbc5084dd6a03e5affa"},
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
			Expectation: map[string]string{"foobar": "6991b773b801a8eafb74dd95d5544d499ba1da5c9a677dbc5084dd6a03e5affa"},
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
			Expectation: map[string]string{"foobar": "7eac1330365e4e8c08c95a343380693b435e00f6d9246f47e7194ce3d749d489"},
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
		{
			Name:    "chunk with variants should produce different hashes",
			Base:    "chunks",
			BaseRef: "",
			Chunk:   "foobar",
			Expectation: map[string]string{
				"foobar:1.16.3": "1d6cf828c405001a5dcbf034c638dace2ae5ab20d27c6c33519a7f6b5ca3eae6",
				"foobar:1.16.4": "983b53b4df52485fe2c4a7cdc005b957d03909459d4a10de3463cf4facf45ee2",
			},
			FS: map[string]*fstest.MapFile{
				"chunks/foobar/Dockerfile": {
					Data: []byte("FROM ubuntu"),
				},
				"chunks/foobar/chunk.yaml": {
					Data: []byte(`variants:
- name: "1.16.3"
  args:
    GO_VERSION: 1.16.3
- name: "1.16.4"
  args:
    GO_VERSION: 1.16.4
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

			act := make(map[string]string, len(chks))
			for _, chk := range chks {
				hash, err := chk.hash(test.BaseRef, !test.IncludeTests)
				if err != nil {
					t.Errorf("could not compute hash: %v", err)
					return
				}
				act[chk.Name] = hash
			}

			if diff := cmp.Diff(test.Expectation, act); diff != "" {
				t.Errorf("hash() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

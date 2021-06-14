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
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/containerd/containerd/remotes"
	"github.com/containerd/containerd/remotes/docker"
	"github.com/docker/distribution/reference"

	"github.com/moby/buildkit/client"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

func TestProjectChunk_test(t *testing.T) {
	ctx := context.Background()
	sess, err := NewSession(nil, "localhost:9999/test")
	if err != nil {
		t.Errorf("could not create session:%v", err)
	}
	sess.opts.Resolver = fakeResolver{}

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
				Registry: fakeRegistry{
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
			gotOk, _, err := chks[0].test(tt.args.ctx, tt.args.sess)
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

type fakeRegistry struct {
	testResult *StoredTestResult
}

func (t fakeRegistry) Push(ctx context.Context, ref reference.Named, opts storeInRegistryOptions) (absref reference.Digested, err error) {
	return nil, nil
}

func (t fakeRegistry) Pull(ctx context.Context, ref reference.Reference, cfg interface{}) (manifest *ociv1.Manifest, absref reference.Digested, err error) {
	if t.testResult != nil {
		r := cfg.(*StoredTestResult)
		r.Passed = t.testResult.Passed
	}
	return nil, nil, nil
}

type fakeResolver struct{}

func (t fakeResolver) Resolve(ctx context.Context, ref string) (name string, desc ocispec.Descriptor, err error) {
	return "test", ocispec.Descriptor{}, nil
}

func (t fakeResolver) Fetcher(ctx context.Context, ref string) (remotes.Fetcher, error) {
	return nil, nil
}

func (t fakeResolver) Pusher(ctx context.Context, ref string) (remotes.Pusher, error) {
	return nil, nil
}

type tagResponse struct {
	Name string
	Tags []string
}

func TestProjectChunk_test_integration(t *testing.T) {
	// NOTE: requires a running Buildkit daemon and registry
	buildkitAddr := os.Getenv("BUILDKIT_ADDR")
	if buildkitAddr == "" {
		t.Skip("set BUILDKIT_ADDR to run this test")
	}
	targetRef := os.Getenv("TARGET_REF")
	if targetRef == "" {
		t.Skip("set TARGET_REF to run this test")
	}
	// NOTE: using a ~unique target here to allow identification of the output from this test
	targetRepo := fmt.Sprintf("integration_%d", time.Now().UnixNano())
	fullTargetRef := fmt.Sprintf("%s/%s", targetRef, targetRepo)
	ctx := context.Background()
	cl, err := client.New(ctx, buildkitAddr, client.WithFailFast())
	if err != nil {
		t.Errorf("Could not create client: %v", err)
		return
	}
	resolver := docker.NewResolver(docker.ResolverOptions{})
	session, err := NewSession(cl, fullTargetRef,
		WithResolver(resolver),
		WithNoCache(true),
		WithPlainOutput(true),
		WithChunkedWithoutHash(false),
	)
	if err != nil {
		t.Errorf("Could not create session: %v", err)
		return
	}

	tmpDir := t.TempDir()
	targetDir := tmpDir + "/testdata"
	err = CopyDir("./testdata", targetDir)
	if err != nil {
		t.Errorf("TestProjectChunk_test_integration() could not copy testdata: %v", err)
		return
	}

	prj, err := LoadFromDir(targetDir, LoadFromDirOpts{})
	if err != nil {
		t.Errorf("TestProjectChunk_test_integration() could not load project: %v", err)
		return
	}

	// Ensure we don't have any existing tags in repository
	httpClient := &http.Client{}
	req, _ := http.NewRequest("GET", "http://"+targetRef+"/v2/"+targetRepo+"/tags/list", nil)
	req.Header.Add("Accept", "application/json")

	// Should not have any tags for this project
	{
		resp, err := httpClient.Do(req)
		if err != nil {
			t.Errorf("TestProjectChunk_test_integration() could not get tags from registry: %v", err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("TestProjectChunk_test_integration() should not have tags: returned %v", resp)
			return
		}
	}

	err = prj.Build(context.Background(), session)
	if err != nil {
		t.Errorf("ProjectChunk.test() unexpected Build error = %v", err)
		return
	}

	// Should now have tags for this project
	{
		resp, err := httpClient.Do(req)
		if err != nil {
			t.Errorf("TestProjectChunk_test_integration() could not get tags from registry: %v", err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("TestProjectChunk_test_integration() should have tags: returned %v", resp)
			return
		}

		var tagResp tagResponse
		// Decode the data
		if err := json.NewDecoder(resp.Body).Decode(&tagResp); err != nil {
			t.Errorf("TestProjectChunk_test_integration() could not get decode tags from registry: %v", err)
			return
		}
		if len(tagResp.Tags) != 5 {
			t.Errorf("TestProjectChunk_test_integration() expected 5 tags from registry: got %v", tagResp.Tags)
			return
		}
	}

	// Re-running build should reuse existing images & tags
	err = prj.Build(context.Background(), session)
	if err != nil {
		t.Errorf("ProjectChunk.test() unexpected rebuild 1 error = %v", err)
		return
	}

	// Should not have any new tags for this project
	{
		resp, err := httpClient.Do(req)
		if err != nil {
			t.Errorf("TestProjectChunk_test_integration() could not get tags from registry: %v", err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("TestProjectChunk_test_integration() should have tags: returned %v", resp)
			return
		}

		var tagResp tagResponse
		// Decode the data
		if err := json.NewDecoder(resp.Body).Decode(&tagResp); err != nil {
			t.Errorf("TestProjectChunk_test_integration() could not get decode tags from registry: %v", err)
			return
		}
		if len(tagResp.Tags) != 5 {
			t.Errorf("TestProjectChunk_test_integration() expected 5 tags from registry: got %v", tagResp.Tags)
			return
		}
	}

	// Individually check each chunk to ensure it doesn't rebuild
	for _, chk := range prj.Chunks {
		ok, didRun, err := chk.test(ctx, session)
		if err != nil || !ok || didRun {
			t.Errorf("TestProjectChunk_test_integration() error:%v testing chunk: %s with results: %v:%v", err, chk.Name, ok, didRun)
			return
		}

		_, didBuild, err := chk.build(ctx, session)
		if err != nil || didBuild {
			t.Errorf("TestProjectChunk_test_integration() error:%v building chunk: %s didBuild:%v", err, chk.Name, didBuild)
			return
		}
	}

	// Modify a test and ensure it is rebuilt
	newTest := []byte(`- desc: "it should have xxx"
  command: ["sh", "-c", "ls -tl xxx"]
  assert:
    - "status == 0"
    - stdout.indexOf("xxx") != -1`)

	err = ioutil.WriteFile(targetDir+"/tests/basic.yaml", newTest, 0644)
	if err != nil {
		t.Errorf("TestProjectChunk_test_integration() error:%v writing new test", err)
		return
	}

	// Reload to get new test
	prj, err = LoadFromDir(targetDir, LoadFromDirOpts{})
	if err != nil {
		t.Errorf("TestProjectChunk_test_integration() could not reload project: %v", err)
		return
	}

	// Re-running build should create a new test tags
	err = prj.Build(context.Background(), session)
	if err != nil {
		t.Errorf("ProjectChunk.test() unexpected rebuild 2 error = %v", err)
		return
	}

	// Should now have new test tags for this project
	{
		resp, err := httpClient.Do(req)
		if err != nil {
			t.Errorf("TestProjectChunk_test_integration() could not get tags from registry: %v", err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("TestProjectChunk_test_integration() should have tags: returned %v", resp)
			return
		}

		var tagResp tagResponse
		// Decode the data
		if err := json.NewDecoder(resp.Body).Decode(&tagResp); err != nil {
			t.Errorf("TestProjectChunk_test_integration() could not get decode tags from registry: %v", err)
			return
		}
		if len(tagResp.Tags) != 7 {
			t.Errorf("TestProjectChunk_test_integration() expected 7 tags from registry: got %v", tagResp.Tags)
			return
		}
	}
}

// FROM: https://gist.github.com/r0l1/92462b38df26839a3ca324697c8cba04
// CopyFile copies the contents of the file named src to the file named
// by dst. The file will be created if it does not already exist. If the
// destination file exists, all it's contents will be replaced by the contents
// of the source file. The file mode will be copied from the source and
// the copied data is synced/flushed to stable storage.
func CopyFile(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return
	}
	defer func() {
		if e := out.Close(); e != nil {
			err = e
		}
	}()

	_, err = io.Copy(out, in)
	if err != nil {
		return
	}

	err = out.Sync()
	if err != nil {
		return
	}

	si, err := os.Stat(src)
	if err != nil {
		return
	}
	err = os.Chmod(dst, si.Mode())
	if err != nil {
		return
	}

	return
}

// CopyDir recursively copies a directory tree, attempting to preserve permissions.
// Source directory must exist, destination directory must *not* exist.
// Symlinks are ignored and skipped.
func CopyDir(src string, dst string) (err error) {
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)

	si, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !si.IsDir() {
		return fmt.Errorf("source is not a directory")
	}

	_, err = os.Stat(dst)
	if err != nil && !os.IsNotExist(err) {
		return
	}
	if err == nil {
		return fmt.Errorf("destination already exists")
	}

	err = os.MkdirAll(dst, si.Mode())
	if err != nil {
		return
	}

	entries, err := ioutil.ReadDir(src)
	if err != nil {
		return
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			err = CopyDir(srcPath, dstPath)
			if err != nil {
				return
			}
		} else {
			// Skip symlinks.
			if entry.Mode()&os.ModeSymlink != 0 {
				continue
			}

			err = CopyFile(srcPath, dstPath)
			if err != nil {
				return
			}
		}
	}

	return
}

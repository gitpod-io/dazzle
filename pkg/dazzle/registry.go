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

	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/remotes"
	"github.com/docker/distribution/reference"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

const (
	mediaTypeTestResult = "application/vnd.gitpod.dazzle.tests.v1+json"
)

func storeInRegistry(ctx context.Context, resolver remotes.Resolver, ref reference.Named, mediaType string, content []byte) error {
	pusher, err := resolver.Pusher(ctx, ref.String())
	if err != nil {
		return fmt.Errorf("cannot store in registry: %v", err)
	}

	mf := ociv1.Manifest{
		Versioned: specs.Versioned{
			SchemaVersion: 2,
		},
		Config: ociv1.Descriptor{
			MediaType: mediaType,
			Size:      int64(len(content)),
			Digest:    digest.FromBytes(content),
		},
	}
	mfc, err := json.Marshal(mf)
	if err != nil {
		return err
	}
	mfdesc := ociv1.Descriptor{
		MediaType: ociv1.MediaTypeImageManifest,
		Size:      int64(len(mfc)),
		Digest:    digest.FromBytes(mfc),
	}

	cfgW, err := pusher.Push(ctx, mf.Config)
	if err == nil {
		n, err := cfgW.Write(content)
		if err != nil {
			return err
		} else if n < len(content) {
			return io.ErrShortWrite
		}
		err = cfgW.Commit(ctx, mf.Config.Size, mf.Config.Digest)
		if err != nil {
			return err
		}
		err = cfgW.Close()
		if err != nil {
			return err
		}
	} else if !errdefs.IsAlreadyExists(err) {
		return err
	}

	mfW, err := pusher.Push(ctx, mfdesc)
	if err != nil {
		return err
	}
	if err == nil {
		n, err := mfW.Write(mfc)
		if err != nil {
			return err
		}
		if n < len(content) {
			return io.ErrShortWrite
		}
		err = mfW.Commit(ctx, mfdesc.Size, mfdesc.Digest)
		if err != nil {
			return err
		}
		err = mfW.Close()
		if err != nil {
			return err
		}
	} else if !errdefs.IsAlreadyExists(err) {
		return err
	}

	return nil
}

func pullFromRegistry(ctx context.Context, resolver remotes.Resolver, ref reference.Reference, cfg interface{}) (manifest *ociv1.Manifest, absref reference.Digested, err error) {
	_, desc, err := resolver.Resolve(ctx, ref.String())
	if err != nil {
		return
	}
	fetcher, err := resolver.Fetcher(ctx, ref.String())
	if err != nil {
		return
	}

	// TODO: deal with this when the ref points to an image list rater than the image
	manifestr, err := fetcher.Fetch(ctx, desc)
	if err != nil {
		return
	}
	defer manifestr.Close()
	manifestraw, err := ioutil.ReadAll(manifestr)
	if err != nil {
		return
	}
	var mf ociv1.Manifest
	err = json.Unmarshal(manifestraw, &mf)
	if err != nil {
		return
	}

	cfgr, err := fetcher.Fetch(ctx, mf.Config)
	if err != nil {
		return
	}
	defer cfgr.Close()
	cfgraw, err := ioutil.ReadAll(cfgr)
	if err != nil {
		return
	}
	err = json.Unmarshal(cfgraw, &cfg)
	if err != nil {
		return
	}
	manifest = &mf

	if rr, ok := ref.(reference.Digested); ok {
		absref = rr
	} else if rr, ok := ref.(reference.Named); ok {
		absref, err = reference.WithDigest(rr, desc.Digest)
		if err != nil {
			return
		}
	} else {
		err = fmt.Errorf("invalid reference type")
		return
	}

	return
}

type StoredTestResult struct {
	Passed bool `json:"passed"`
}

func pushTestResult(ctx context.Context, resolver remotes.Resolver, ref reference.Named, r StoredTestResult) error {
	content, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return storeInRegistry(ctx, resolver, ref, mediaTypeTestResult, content)
}

func pullTestResult(ctx context.Context, resolver remotes.Resolver, ref reference.Named) (*StoredTestResult, error) {
	var (
		res StoredTestResult
		err error
	)
	_, _, err = pullFromRegistry(ctx, resolver, ref, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

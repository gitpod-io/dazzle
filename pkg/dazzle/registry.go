// Copyright © 2020 Gitpod

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

// Registry provides container registry services
type Registry interface {
	Push(ctx context.Context, ref reference.Named, opts storeInRegistryOptions) (absref reference.Digested, err error)
	Pull(ctx context.Context, ref reference.Reference, cfg interface{}) (manifest *ociv1.Manifest, absref reference.Digested, err error)
}

type resolverRegistry struct {
	resolver remotes.Resolver
}

func NewResolverRegistry(resolver remotes.Resolver) Registry {
	return resolverRegistry{
		resolver: resolver,
	}
}

type storeInRegistryOptions struct {
	Config          []byte
	ConfigMediaType string
	Manifest        *ociv1.Manifest
}

func (r resolverRegistry) Push(ctx context.Context, ref reference.Named, opts storeInRegistryOptions) (absref reference.Digested, err error) {
	pusher, err := r.resolver.Pusher(ctx, ref.String())
	if err != nil {
		return nil, fmt.Errorf("cannot store in registry: %v", err)
	}

	var mf ociv1.Manifest
	if opts.Manifest == nil {
		mf = ociv1.Manifest{
			Versioned: specs.Versioned{
				SchemaVersion: 2,
			},
			Config: ociv1.Descriptor{
				MediaType: opts.ConfigMediaType,
				Size:      int64(len(opts.Config)),
				Digest:    digest.FromBytes(opts.Config),
			},
		}
	} else {
		mf = *opts.Manifest
	}
	mfc, err := json.Marshal(mf)
	if err != nil {
		return nil, err
	}
	mfdesc := ociv1.Descriptor{
		MediaType: ociv1.MediaTypeImageManifest,
		Size:      int64(len(mfc)),
		Digest:    digest.FromBytes(mfc),
	}

	if len(opts.Config) > 0 {
		cfgW, err := pusher.Push(ctx, mf.Config)
		if err == nil {
			n, err := cfgW.Write(opts.Config)
			if err != nil {
				return nil, err
			} else if n < len(opts.Config) {
				return nil, io.ErrShortWrite
			}
			err = cfgW.Commit(ctx, mf.Config.Size, mf.Config.Digest)
			if err != nil {
				return nil, err
			}
			err = cfgW.Close()
			if err != nil {
				return nil, err
			}
		} else if !errdefs.IsAlreadyExists(err) {
			return nil, err
		}
	}

	mfW, err := pusher.Push(ctx, mfdesc)
	if err != nil {
		return nil, err
	}
	if err == nil {
		n, err := mfW.Write(mfc)
		if err != nil {
			return nil, err
		}
		if n < len(mfc) {
			return nil, io.ErrShortWrite
		}
		err = mfW.Commit(ctx, mfdesc.Size, mfdesc.Digest)
		if err != nil {
			return nil, err
		}
		err = mfW.Close()
		if err != nil {
			return nil, err
		}
	} else if !errdefs.IsAlreadyExists(err) {
		return nil, err
	}

	absref, err = reference.WithDigest(ref, mfdesc.Digest)
	if err != nil {
		return nil, err
	}
	return absref, nil
}

func (r resolverRegistry) Pull(ctx context.Context, ref reference.Reference, cfg interface{}) (manifest *ociv1.Manifest, absref reference.Digested, err error) {
	_, desc, err := r.resolver.Resolve(ctx, ref.String())
	if err != nil {
		return
	}
	fetcher, err := r.resolver.Fetcher(ctx, ref.String())
	if err != nil {
		return
	}

	// TODO: deal with this when the ref points to an image list rather than the image
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

func pushTestResult(ctx context.Context, registry Registry, ref reference.Named, r StoredTestResult) (absref reference.Digested, err error) {
	content, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	return registry.Push(ctx, ref, storeInRegistryOptions{
		Config:          content,
		ConfigMediaType: mediaTypeTestResult,
	})
}

func pullTestResult(ctx context.Context, registry Registry, ref reference.Named) (*StoredTestResult, error) {
	var (
		res StoredTestResult
		err error
	)
	_, _, err = registry.Pull(ctx, ref, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

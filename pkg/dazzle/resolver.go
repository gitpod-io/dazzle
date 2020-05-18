package dazzle

import (
	"context"

	"github.com/containerd/containerd/remotes"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/util/contentutil"
	"github.com/moby/buildkit/util/imageutil"
	"github.com/opencontainers/go-digest"
)

func newImageMetaResolver(resolver remotes.Resolver) *imageMetaResolver {
	return &imageMetaResolver{
		resolver: resolver,
		buffer:   contentutil.NewBuffer(),
	}
}

type imageMetaResolver struct {
	resolver remotes.Resolver
	buffer   contentutil.Buffer
}

func (imr *imageMetaResolver) ResolveImageConfig(ctx context.Context, ref string, opt llb.ResolveImageConfigOpt) (digest.Digest, []byte, error) {
	platform := opt.Platform

	dgst, config, err := imageutil.Config(ctx, ref, imr.resolver, imr.buffer, nil, platform)
	if err != nil {
		return "", nil, err
	}

	return dgst, config, nil
}

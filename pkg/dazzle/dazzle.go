package dazzle

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar"
	"github.com/containerd/console"
	"github.com/containerd/containerd/remotes"
	"github.com/containerd/containerd/remotes/docker"
	"github.com/csweichel/dazzle/pkg/test"
	"github.com/docker/distribution/reference"
	"github.com/minio/highwayhash"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/frontend/dockerfile/dockerfile2llb"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/auth/authprovider"
	"github.com/moby/buildkit/util/progress/progressui"
	"github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v2"
)

var (
	hashKey = []byte{0x03, 0x40, 0xf3, 0xc8, 0x94, 0x7c, 0xad, 0x78, 0x75, 0x14, 0x0f, 0x4c, 0x4a, 0xf7, 0xc6, 0x2b, 0x43, 0x13, 0x1d, 0xc2, 0xa8, 0xc7, 0xfc, 0x46, 0x28, 0xf0, 0x68, 0x5e, 0x36, 0x9a, 0x3b, 0x0b}
)

// Project is a dazzle build project
type Project struct {
	Base   ProjectChunk
	Chunks []ProjectChunk
}

// ProjectChunk represents a layer chunk in a project
type ProjectChunk struct {
	Name        string
	Dockerfile  []byte
	ContextPath string
	Tests       []test.Spec
}

// LoadFromDir loads a dazzle project from disk
func LoadFromDir(dir string) (*Project, error) {
	base, err := loadChunkFromDir(filepath.Join(dir, "_base"))
	if err != nil {
		return nil, err
	}

	res := &Project{
		Base: *base,
	}
	chds, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	res.Chunks = make([]ProjectChunk, 0, len(chds))
	for _, chd := range chds {
		if strings.HasPrefix(chd.Name(), "_") || strings.HasPrefix(chd.Name(), ".") {
			continue
		}
		chnk, err := loadChunkFromDir(filepath.Join(dir, chd.Name()))
		if err != nil {
			return nil, err
		}
		res.Chunks = append(res.Chunks, *chnk)
		log.WithField("name", chnk.Name).Info("added chunk to project")
	}

	return res, nil
}

func loadChunkFromDir(dir string) (*ProjectChunk, error) {
	res := &ProjectChunk{
		Name:        filepath.Base(dir),
		ContextPath: dir,
	}

	var err error
	res.Dockerfile, err = ioutil.ReadFile(filepath.Join(dir, "Dockerfile"))
	if err != nil {
		return nil, err
	}

	tf, err := ioutil.ReadFile(filepath.Join(dir, "tests.yaml"))
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("%s: cannot read tests.yaml: %w", dir, err)
	}
	err = yaml.UnmarshalStrict(tf, &res.Tests)
	if err != nil {
		return nil, fmt.Errorf("%s: cannot read tests.yaml: %w", dir, err)
	}
	return res, nil
}

type buildOpts struct {
	CacheRef    reference.Named
	Resolver    remotes.Resolver
	PlainOutput bool
}

// BuildOpt modifies build behaviour
type BuildOpt func(*buildOpts) error

// WithCacheRef makes dazzle use a cache ref other than the target ref
func WithCacheRef(ref string) BuildOpt {
	return func(b *buildOpts) error {
		r, err := reference.ParseNamed(ref)
		if err != nil {
			return fmt.Errorf("cannot parse cache ref: %w", err)
		}

		b.CacheRef = r
		return nil
	}
}

// WithResolver makes the builder use a custom resolver
func WithResolver(r remotes.Resolver) BuildOpt {
	return func(b *buildOpts) error {
		b.Resolver = r
		return nil
	}
}

// WithPlainOutput forces plain build output
func WithPlainOutput(enable bool) BuildOpt {
	return func(b *buildOpts) error {
		b.PlainOutput = enable
		return nil
	}
}

// Build builds all images in a project
func (p *Project) Build(ctx context.Context, cl *client.Client, targetRef string, options ...BuildOpt) error {
	target, err := reference.ParseNamed(targetRef)
	if err != nil {
		return err
	}

	opts := buildOpts{
		Resolver: docker.NewResolver(docker.ResolverOptions{}),
	}
	for _, o := range options {
		err := o(&opts)
		if err != nil {
			return err
		}
	}

	// Relying on the buildkit cache alone does not result in fixed content hashes.
	// We must locally build hashes and use them as unique image names.
	baseref, err := reference.WithTag(target, "base")
	if err != nil {
		return err
	}
	if opts.CacheRef == nil {
		opts.CacheRef = baseref
	}

	log.WithField("ref", baseref.String()).Warn("building base iamge")
	absbaseref, err := p.Base.buildAsBase(ctx, cl, baseref, opts)
	if err != nil {
		return fmt.Errorf("cannot build base image: %w", err)
	}

	_, basedesc, err := opts.Resolver.Resolve(ctx, absbaseref.String())
	if err != nil {
		return fmt.Errorf("cannot fetch base desc: %w", err)
	}
	basefetcher, err := opts.Resolver.Fetcher(ctx, absbaseref.String())
	if err != nil {
		return fmt.Errorf("cannot fetch base desc: %w", err)
	}
	basemanr, err := basefetcher.Fetch(ctx, basedesc)
	if err != nil {
		return fmt.Errorf("cannot fetch base desc: %w", err)
	}
	baseman, err := ioutil.ReadAll(basemanr)
	basemanr.Close()
	if err != nil {
		return fmt.Errorf("cannot fetch base desc: %w", err)
	}
	var spec ociv1.Image
	err = json.Unmarshal(baseman, &spec)
	if err != nil {
		return fmt.Errorf("cannot fetch base desc: %w", err)
	}

	for _, chk := range p.Chunks {
		// TODO: record built image name and run tests
		log.WithField("chunk", chk.Name).Warn("building chunk")
		_, err = chk.build(ctx, cl, absbaseref, target, opts)
		if err != nil {
			return fmt.Errorf("cannot build chunk %s: %w", chk.Name, err)
		}

		//
	}

	return nil
}

func (p *ProjectChunk) getLLB(ctx context.Context, base reference.Reference, resolver remotes.Resolver) (state *llb.State, err error) {
	args := make(map[string]string)
	if base != nil {
		args["base"] = base.String()
	}
	state, _, err = dockerfile2llb.Dockerfile2LLB(ctx, p.Dockerfile, dockerfile2llb.ConvertOpt{
		BuildArgs:    args,
		MetaResolver: newImageMetaResolver(resolver),
	})
	return
}

func (p *ProjectChunk) hash() (res string, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("cannot compute hash: %w", err)
		}
	}()

	hash, err := highwayhash.New(hashKey)
	if err != nil {
		return
	}

	err = p.manifest(hash)
	if err != nil {
		return
	}

	res = hex.EncodeToString(hash.Sum(nil))
	return
}

func (p *ProjectChunk) manifest(out io.Writer) (err error) {
	sources, err := doublestar.Glob(filepath.Join(p.ContextPath, "**/*"))
	if err != nil {
		return
	}

	res := make([]string, 0, len(sources))
	for _, src := range sources {
		if stat, err := os.Stat(src); err != nil {
			return err
		} else if stat.IsDir() {
			return fmt.Errorf("source list must not contain directories")
		}

		file, err := os.OpenFile(src, os.O_RDONLY, 0644)
		if err != nil {
			return err
		}

		hash, err := highwayhash.New(hashKey)
		if err != nil {
			file.Close()
			return err
		}

		_, err = io.Copy(hash, file)
		if err != nil {
			file.Close()
			return err
		}

		err = file.Close()
		if err != nil {
			return err
		}

		res = append(res, fmt.Sprintf("%s:%s", strings.TrimPrefix(src, p.ContextPath), hex.EncodeToString(hash.Sum(nil))))
	}

	fmt.Fprintf(out, "Dockerfile: %s\n", string(p.Dockerfile))
	fmt.Fprintf(out, "Sources:\n%s\n", strings.Join(res, "\n"))
	return nil
}

func (p *ProjectChunk) buildAsBase(ctx context.Context, cl *client.Client, dst reference.Named, opts buildOpts) (absref reference.Digested, err error) {
	defs, err := p.getLLB(ctx, nil, opts.Resolver)
	if err != nil {
		return
	}

	def, err := defs.Marshal()
	if err != nil {
		return
	}

	hash, err := p.hash()
	if err != nil {
		return
	}

	dest, err := reference.WithTag(dst, fmt.Sprintf("base-%s", hash))
	if err != nil {
		return
	}

	_, desc, err := opts.Resolver.Resolve(ctx, dest.String())
	if err == nil {
		// if err == nil the image exists already
		return reference.WithDigest(dest, desc.Digest)
	}

	eg, ctx := errgroup.WithContext(ctx)
	ch := make(chan *client.SolveStatus)

	var (
		cacheImport = client.CacheOptionsEntry{
			Type: "registry",
			Attrs: map[string]string{
				"ref": dest.String(),
			},
		}
		cacheExport = client.CacheOptionsEntry{
			Type: "inline",
		}
	)

	rchan := make(chan map[string]string, 1)
	eg.Go(func() error {
		resp, err := cl.Solve(ctx, def, client.SolveOpt{
			CacheImports: []client.CacheOptionsEntry{cacheImport},
			CacheExports: []client.CacheOptionsEntry{cacheExport},
			Session: []session.Attachable{
				authprovider.NewDockerAuthProvider(os.Stderr),
			},
			Exports: []client.ExportEntry{
				{
					Type: "image",
					Attrs: map[string]string{
						"name": dest.String(),
						"push": "true",
					},
				},
			},
			LocalDirs: map[string]string{
				"context": p.ContextPath,
			},
		}, ch)
		if err != nil {
			return err
		}
		rchan <- resp.ExporterResponse
		return nil
	})
	eg.Go(func() error {
		var c console.Console

		if !opts.PlainOutput {
			cf, err := console.ConsoleFromFile(os.Stderr)
			if err != nil {
				return err
			}
			c = cf
		}

		// not using shared context to not disrupt display but let is finish reporting errors
		return progressui.DisplaySolveStatus(context.TODO(), "", c, os.Stderr, ch)
	})
	err = eg.Wait()
	if err != nil {
		return
	}

	resp := <-rchan
	dgst, err := digest.Parse(resp["containerimage.digest"])
	if err != nil {
		return
	}
	resref, err := reference.WithDigest(dest, dgst)
	if err != nil {
		return
	}

	return resref, nil
}

func (p *ProjectChunk) build(ctx context.Context, cl *client.Client, base reference.Digested, dst reference.Named, opts buildOpts) (absref string, err error) {
	defs, err := p.getLLB(ctx, base, opts.Resolver)
	if err != nil {
		return
	}

	def, err := defs.Marshal()
	if err != nil {
		return
	}

	hash, err := p.hash()
	if err != nil {
		return
	}

	dest, err := reference.WithTag(dst, fmt.Sprintf("%s-%s", p.Name, hash))
	if err != nil {
		return
	}

	absref, _, err = opts.Resolver.Resolve(ctx, dest.String())
	if err == nil {
		// err == nil means the image exists already
		return
	}

	cacheTgt, err := reference.WithTag(opts.CacheRef, fmt.Sprintf("%s--cache", p.Name))
	if err != nil {
		return
	}
	// chunkTgt, err := reference.WithTag(dst, p.Name)
	// if err != nil {
	// 	return
	// }

	eg, ctx := errgroup.WithContext(ctx)
	ch := make(chan *client.SolveStatus)

	var (
		cacheImport = client.CacheOptionsEntry{
			Type: "registry",
			Attrs: map[string]string{
				"ref": cacheTgt.String(),
			},
		}
		cacheExport = client.CacheOptionsEntry{
			Type: "registry",
			Attrs: map[string]string{
				"ref": cacheTgt.String(),
			},
		}
	)

	// TODO: export locally and subtract base image
	rchan := make(chan map[string]string, 1)
	eg.Go(func() error {
		resp, err := cl.Solve(ctx, def, client.SolveOpt{
			CacheImports: []client.CacheOptionsEntry{cacheImport},
			CacheExports: []client.CacheOptionsEntry{cacheExport},
			Session: []session.Attachable{
				authprovider.NewDockerAuthProvider(os.Stderr),
			},
			Exports: []client.ExportEntry{
				{
					Type: "image",
					Attrs: map[string]string{
						"name": dest.String(),
						"push": "true",
					},
				},
			},
			LocalDirs: map[string]string{
				"context": p.ContextPath,
			},
		}, ch)
		if err != nil {
			return err
		}
		rchan <- resp.ExporterResponse
		return nil
	})
	eg.Go(func() error {
		var c console.Console

		if !opts.PlainOutput {
			cf, err := console.ConsoleFromFile(os.Stderr)
			if err != nil {
				return err
			}
			c = cf
		}

		// not using shared context to not disrupt display but let is finish reporting errors
		return progressui.DisplaySolveStatus(context.TODO(), "", c, os.Stderr, ch)
	})
	err = eg.Wait()
	if err != nil {
		return
	}

	resp := <-rchan
	dgst, err := digest.Parse(resp["containerimage.digest"])
	if err != nil {
		return
	}
	resref, err := reference.WithDigest(dest, dgst)
	if err != nil {
		return
	}

	return resref.String(), nil
}

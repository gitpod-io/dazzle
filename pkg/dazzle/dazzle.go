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
	"github.com/containerd/containerd/errdefs"
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
	Tests       []*test.Spec
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
	NoCache     bool
	NoTests     bool
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

// WithNoCache disables the buildkit build cache
func WithNoCache(enable bool) BuildOpt {
	return func(b *buildOpts) error {
		b.NoCache = enable
		return nil
	}
}

// WithNoTests disables the build-time tests
func WithNoTests(enable bool) BuildOpt {
	return func(b *buildOpts) error {
		b.NoCache = enable
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
	baseref, err := p.BaseRef(target)
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

	basemf, basecfg, err := getImageMetadata(ctx, absbaseref, opts.Resolver)
	if err != nil {
		return fmt.Errorf("cannot fetch base image: %w", err)
	}

	for _, chk := range p.Chunks {
		chkname, err := p.ChunkRef(target, chk.Name)
		if err != nil {
			return err
		}

		// TODO: record built image name and run tests
		log.WithField("chunk", chk.Name).Warn("building chunk")
		chkref, err := chk.build(ctx, cl, absbaseref, target, opts)
		if err != nil {
			return fmt.Errorf("cannot build chunk %s: %w", chk.Name, err)
		}

		log.WithField("chunk", chk.Name).Warn("building chunk without base")
		err = removeBaseLayer(ctx, opts.Resolver, basemf, basecfg, chkref, chkname)
		if err != nil {
			return err
		}

		if len(chk.Tests) == 0 {
			continue
		}
		if opts.NoTests {
			log.WithField("chunk", chk.Name).Warn("Skipping chunk tests (no-tests)")
			continue
		}
		log.WithField("chunk", chk.Name).Warn("Running chunk tests")
		executor := test.NewBuildkitExecutor(cl, chkref.String())
		test.RunTests(ctx, executor, chk.Tests)
	}

	return nil
}

func removeBaseLayer(ctx context.Context, resolver remotes.Resolver, basemf *ociv1.Manifest, basecfg *ociv1.Image, chunkref reference.Reference, dest reference.NamedTagged) (err error) {
	chkmf, chkcfg, err := getImageMetadata(ctx, chunkref, resolver)
	if err != nil {
		return
	}

	for i := range basemf.Layers {
		if len(chkmf.Layers) < i {
			return fmt.Errorf("chunk was not built from base image (too few layers)")
		}
		if len(chkcfg.RootFS.DiffIDs) < i {
			return fmt.Errorf("chunk was not built from base image (too few diffIDs)")
		}
		var (
			bl = basemf.Layers[i]
			bd = basecfg.RootFS.DiffIDs[i]
			cl = chkmf.Layers[i]
			cd = chkcfg.RootFS.DiffIDs[i]
		)
		if bl.Digest.String() != cl.Digest.String() {
			return fmt.Errorf("chunk was not built from base image: digest mistmatch on layer %d: base %s != chunk %s", i, bl.Digest.String(), cl.Digest.String())
		}
		if bd.String() != cd.String() {
			return fmt.Errorf("chunk was not built from base image: digest mistmatch on diffID %d: base %s != chunk %s", i, bd.String(), cd.String())
		}
	}

	n := len(basecfg.RootFS.DiffIDs)
	chkcfg.RootFS = ociv1.RootFS{
		Type:    chkcfg.RootFS.Type,
		DiffIDs: chkcfg.RootFS.DiffIDs[n:],
	}
	chkcfg.History = chkcfg.History[n:]
	ncfg, err := json.Marshal(chkcfg)
	if err != nil {
		return
	}

	chkmf.Config = ociv1.Descriptor{
		MediaType: chkmf.Config.MediaType,
		Digest:    digest.FromBytes(ncfg),
		Platform:  chkmf.Config.Platform,
		Size:      int64(len(ncfg)),
	}
	chkmf.Layers = chkmf.Layers[len(basemf.Layers):]
	nmf, err := json.Marshal(chkmf)
	if err != nil {
		return
	}
	mfdesc := ociv1.Descriptor{
		MediaType: ociv1.MediaTypeImageManifest,
		Platform:  chkmf.Config.Platform,
		Digest:    digest.FromBytes(nmf),
		Size:      int64(len(nmf)),
	}

	pusher, err := resolver.Pusher(ctx, dest.String())
	if err != nil {
		return
	}
	fetcher, err := resolver.Fetcher(ctx, chunkref.String())
	if err != nil {
		return
	}

	log.WithField("step", 0).WithField("dest", dest.String()).Info("pushing config")
	cfgw, err := pusher.Push(ctx, chkmf.Config)
	if errdefs.IsAlreadyExists(err) {
		// nothing to do
	} else if err != nil {
		return fmt.Errorf("cannot push image config: %w", err)
	} else {
		cfgw.Write(ncfg)
		err = cfgw.Commit(ctx, chkmf.Config.Size, chkmf.Config.Digest)
		if err != nil && !errdefs.IsAlreadyExists(err) {
			return fmt.Errorf("cannot push image config: %w", err)
		}
	}

	log.WithField("step", 1).WithField("dest", dest.String()).Info("pushing layers")
	for i, l := range chkmf.Layers {
		log.WithField("layer", l.Digest).WithField("step", 2+i).Info("copying layer")
		// this is just needed if the chunk and dest are not in the same repo
		err = copyLayer(ctx, fetcher, pusher, l)
		if err != nil {
			return
		}
	}

	log.WithField("step", 3+len(chkmf.Layers)).WithField("dest", dest.String()).Info("pushing manifest")
	mfw, err := pusher.Push(ctx, mfdesc)
	if errdefs.IsAlreadyExists(err) {
		// nothiong to do
	} else if err != nil {
		return fmt.Errorf("cannot push image manifest: %w", err)
	} else {
		mfw.Write(nmf)
		err = mfw.Commit(ctx, mfdesc.Size, mfdesc.Digest)
		if err != nil && !errdefs.IsAlreadyExists(err) {
			return fmt.Errorf("cannot push image: %w", err)
		}
	}

	return nil
}

func copyLayer(ctx context.Context, fetcher remotes.Fetcher, pusher remotes.Pusher, desc ociv1.Descriptor) (err error) {
	rc, err := fetcher.Fetch(ctx, desc)
	if err != nil {
		return
	}
	defer rc.Close()

	w, err := pusher.Push(ctx, desc)
	if errdefs.IsAlreadyExists(err) {
		return nil
	}
	if err != nil {
		return
	}
	defer w.Close()

	_, err = io.Copy(w, rc)
	if err != nil {
		return
	}

	return w.Commit(ctx, desc.Size, desc.Digest)
}

func getImageMetadata(ctx context.Context, ref reference.Reference, resolver remotes.Resolver) (manifest *ociv1.Manifest, config *ociv1.Image, err error) {
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
	var cfg ociv1.Image
	err = json.Unmarshal(cfgraw, &cfg)
	if err != nil {
		return
	}

	manifest = &mf
	config = &cfg
	return
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

func (p *ProjectChunk) hash(baseref string) (res string, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("cannot compute hash: %w", err)
		}
	}()

	hash, err := highwayhash.New(hashKey)
	if err != nil {
		return
	}

	err = p.manifest(baseref, hash)
	if err != nil {
		return
	}

	res = hex.EncodeToString(hash.Sum(nil))
	return
}

func (p *ProjectChunk) manifest(baseref string, out io.Writer) (err error) {
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

	if baseref != "" {
		fmt.Fprintf(out, "Baseref: %s\n", baseref)
	}
	fmt.Fprintf(out, "Dockerfile: %s\n", string(p.Dockerfile))
	fmt.Fprintf(out, "Sources:\n%s\n", strings.Join(res, "\n"))
	return nil
}

// BaseRef returns the ref of the base image of a project
func (p *Project) BaseRef(build reference.Named) (reference.NamedTagged, error) {
	hash, err := p.Base.hash("")
	if err != nil {
		return nil, err
	}
	return reference.WithTag(build, fmt.Sprintf("base--%s", hash))
}

// ChunkRef returns the ref of a chunk image
func (p *Project) ChunkRef(build reference.Named, chunk string) (reference.NamedTagged, error) {
	return reference.WithTag(build, chunk)
}

func (p *ProjectChunk) buildAsBase(ctx context.Context, cl *client.Client, dest reference.Named, opts buildOpts) (absref reference.Digested, err error) {
	defs, err := p.getLLB(ctx, nil, opts.Resolver)
	if err != nil {
		return
	}

	def, err := defs.Marshal()
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

func (p *ProjectChunk) build(ctx context.Context, cl *client.Client, base reference.Digested, dst reference.Named, opts buildOpts) (absref reference.Reference, err error) {
	defs, err := p.getLLB(ctx, base, opts.Resolver)
	if err != nil {
		return
	}

	def, err := defs.Marshal()
	if err != nil {
		return
	}

	hash, err := p.hash(base.String())
	if err != nil {
		return
	}

	dest, err := reference.WithTag(dst, fmt.Sprintf("%s--%s", p.Name, hash))
	if err != nil {
		return
	}

	resrefstr, _, err := opts.Resolver.Resolve(ctx, dest.String())
	if err == nil {
		// err == nil means the image exists already
		absref, err = reference.Parse(resrefstr)
		return
	}

	cacheTgt, err := reference.WithTag(opts.CacheRef, fmt.Sprintf("%s--cache", p.Name))
	if err != nil {
		return
	}

	eg, ctx := errgroup.WithContext(ctx)
	ch := make(chan *client.SolveStatus)

	var (
		cacheImports = []client.CacheOptionsEntry{
			{
				Type: "registry",
				Attrs: map[string]string{
					"ref": cacheTgt.String(),
				},
			},
		}
		cacheExports = []client.CacheOptionsEntry{
			{
				Type: "registry",
				Attrs: map[string]string{
					"ref": cacheTgt.String(),
				},
			},
		}
	)
	if opts.NoCache {
		cacheImports = []client.CacheOptionsEntry{}
		cacheExports = []client.CacheOptionsEntry{}
	}

	// TODO: export locally and subtract base image
	rchan := make(chan map[string]string, 1)
	eg.Go(func() error {
		resp, err := cl.Solve(ctx, def, client.SolveOpt{
			CacheImports: cacheImports,
			CacheExports: cacheExports,
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

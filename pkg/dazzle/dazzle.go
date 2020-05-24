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
	clog "github.com/containerd/containerd/log"
	"github.com/containerd/containerd/remotes"
	"github.com/containerd/containerd/remotes/docker"
	"github.com/csweichel/dazzle/pkg/test"
	"github.com/csweichel/dazzle/pkg/test/buildkit"
	"github.com/docker/distribution/reference"
	"github.com/mattn/go-isatty"
	"github.com/minio/highwayhash"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/auth/authprovider"
	"github.com/moby/buildkit/util/progress/progressui"
	"github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	ignore "github.com/sabhiram/go-gitignore"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v2"
)

var (
	hashKey = []byte{0x03, 0x40, 0xf3, 0xc8, 0x94, 0x7c, 0xad, 0x78, 0x75, 0x14, 0x0f, 0x4c, 0x4a, 0xf7, 0xc6, 0x2b, 0x43, 0x13, 0x1d, 0xc2, 0xa8, 0xc7, 0xfc, 0x46, 0x28, 0xf0, 0x68, 0x5e, 0x36, 0x9a, 0x3b, 0x0b}
)

// ProjectConfig is the structure of a project's dazzle.yaml
type ProjectConfig struct {
	ChunkIgnore  []string `yaml:"ignore"`
	chunkIgnores *ignore.GitIgnore
}

// Project is a dazzle build project
type Project struct {
	Base   ProjectChunk
	Chunks []ProjectChunk
	Config ProjectConfig
}

// ProjectChunk represents a layer chunk in a project
type ProjectChunk struct {
	Name        string
	Dockerfile  []byte
	ContextPath string
	Tests       []*test.Spec

	cachedHash string
}

// LoadFromDir loads a dazzle project from disk
func LoadFromDir(dir string) (*Project, error) {
	var (
		cfg   ProjectConfig
		cfgfn = filepath.Join(dir, "dazzle.yaml")
	)
	if fd, err := os.Open(cfgfn); err == nil {
		log.WithField("filename", cfgfn).Debug("loading dazzle config")
		err = yaml.NewDecoder(fd).Decode(&cfg)
		fd.Close()
		if err != nil {
			return nil, fmt.Errorf("cannot load config from %s: %w", cfgfn, err)
		}

		cfg.chunkIgnores, err = ignore.CompileIgnoreLines(cfg.ChunkIgnore...)
		if err != nil {
			return nil, fmt.Errorf("cannot load config from %s: %w", cfgfn, err)
		}
	}

	base, err := loadChunk(dir, "_base")
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
		if cfg.chunkIgnores != nil && cfg.chunkIgnores.MatchesPath(chd.Name()) {
			continue
		}
		if strings.HasPrefix(chd.Name(), "_") || strings.HasPrefix(chd.Name(), ".") {
			continue
		}
		if !chd.IsDir() {
			continue
		}
		chnk, err := loadChunk(dir, chd.Name())
		if err != nil {
			return nil, err
		}
		res.Chunks = append(res.Chunks, *chnk)
		log.WithField("name", chnk.Name).Info("added chunk to project")
	}

	return res, nil
}

func loadChunk(base, name string) (*ProjectChunk, error) {
	dir := filepath.Join(base, name)
	res := &ProjectChunk{
		Name:        name,
		ContextPath: dir,
	}

	var err error
	res.Dockerfile, err = ioutil.ReadFile(filepath.Join(dir, "Dockerfile"))
	if err != nil {
		return nil, err
	}

	tf, err := ioutil.ReadFile(filepath.Join(base, fmt.Sprintf("%s-tests.yaml", name)))
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
	CacheRef           reference.Named
	NoCache            bool
	NoTests            bool
	Resolver           remotes.Resolver
	PlainOutput        bool
	ChunkedWithoutHash bool
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

// WithChunkedWithoutHash disables the hash prefix for the chunked image tag
func WithChunkedWithoutHash(enable bool) BuildOpt {
	return func(b *buildOpts) error {
		b.ChunkedWithoutHash = enable
		return nil
	}
}

// Build builds all images in a project
func (p *Project) Build(ctx context.Context, session *BuildSession) error {
	ctx = clog.WithLogger(ctx, log.NewEntry(log.New()))

	// Relying on the buildkit cache alone does not result in fixed content hashes.
	// We must locally build hashes and use them as unique image names.
	baseref, err := p.BaseRef(session.Dest)
	if err != nil {
		return err
	}
	if session.opts.CacheRef == nil {
		session.opts.CacheRef = baseref
	}

	log.WithField("ref", baseref.String()).Warn("building base iamge")
	absbaseref, err := p.Base.buildAsBase(ctx, baseref, session)
	if err != nil {
		return fmt.Errorf("cannot build base image: %w", err)
	}

	_, basemf, basecfg, err := getImageMetadata(ctx, absbaseref, session.opts.Resolver)
	if err != nil {
		return fmt.Errorf("cannot fetch base image: %w", err)
	}
	session.baseBuildFinished(absbaseref, basemf, basecfg)

	for _, chk := range p.Chunks {
		_, _, err := chk.build(ctx, session)
		if err != nil {
			return fmt.Errorf("cannot build chunk %s: %w", chk.Name, err)
		}
	}

	return nil
}

// NewSession starts a new build session
func NewSession(cl *client.Client, targetRef string, options ...BuildOpt) (*BuildSession, error) {
	// disable verbose containerd resolver logging
	target, err := reference.ParseNamed(targetRef)
	if err != nil {
		return nil, err
	}

	opts := buildOpts{
		Resolver: docker.NewResolver(docker.ResolverOptions{}),
	}
	for _, o := range options {
		err := o(&opts)
		if err != nil {
			return nil, err
		}
	}

	return &BuildSession{
		Client: cl,
		Dest:   target,
		opts:   opts,
	}, nil
}

// BuildSession records all state of a build
type BuildSession struct {
	Client *client.Client
	Dest   reference.Named

	opts    buildOpts
	baseRef reference.Digested
	baseMF  *ociv1.Manifest
	baseCfg *ociv1.Image
}

// DownloadBaseInfo downloads the base image info
func (s *BuildSession) DownloadBaseInfo(ctx context.Context, p *Project) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("cannot download base image info: %w", err)
		}
	}()

	baseref, err := p.BaseRef(s.Dest)
	if err != nil {
		return err
	}
	log.WithField("ref", baseref).WithField("dest", s.Dest).Debug("downloading base image info")

	absrefs, mf, cfg, err := getImageMetadata(ctx, baseref, s.opts.Resolver)
	if err != nil {
		return err
	}

	s.baseBuildFinished(absrefs, mf, cfg)
	return nil
}

func (s *BuildSession) baseBuildFinished(ref reference.Digested, mf *ociv1.Manifest, cfg *ociv1.Image) {
	s.baseRef = ref
	s.baseMF = mf
	s.baseCfg = cfg
}

func removeBaseLayer(ctx context.Context, resolver remotes.Resolver, basemf *ociv1.Manifest, basecfg *ociv1.Image, chunkref reference.Named, dest reference.NamedTagged) (chkcfg *ociv1.Image, didbuild bool, err error) {
	_, chkmf, chkcfg, err := getImageMetadata(ctx, chunkref, resolver)
	if err != nil {
		return
	}

	for i := range basemf.Layers {
		if len(chkmf.Layers) < i {
			err = fmt.Errorf("chunk was not built from base image (too few layers)")
			return
		}
		if len(chkcfg.RootFS.DiffIDs) < i {
			err = fmt.Errorf("chunk was not built from base image (too few diffIDs)")
			return
		}
		var (
			bl = basemf.Layers[i]
			bd = basecfg.RootFS.DiffIDs[i]
			cl = chkmf.Layers[i]
			cd = chkcfg.RootFS.DiffIDs[i]
		)
		if bl.Digest.String() != cl.Digest.String() {
			err = fmt.Errorf("chunk was not built from base image: digest mistmatch on layer %d: base %s != chunk %s", i, bl.Digest.String(), cl.Digest.String())
			return
		}
		if bd.String() != cd.String() {
			err = fmt.Errorf("chunk was not built from base image: digest mistmatch on diffID %d: base %s != chunk %s", i, bd.String(), cd.String())
			return
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

	if _, dstmf, dstcfg, err := getImageMetadata(ctx, dest, resolver); err == nil {
		if dstmf.Config.Digest == chkmf.Config.Digest {
			// config is already pushed to remote from a previous run.
			// We just assume that the manifest must be up to date, too and stop here.
			return dstcfg, false, nil
		}
	}
	didbuild = true

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
		err = fmt.Errorf("cannot push image config: %w", err)
		return
	} else {
		cfgw.Write(ncfg)
		err = cfgw.Commit(ctx, chkmf.Config.Size, chkmf.Config.Digest)
		if err != nil && !errdefs.IsAlreadyExists(err) {
			err = fmt.Errorf("cannot push image config: %w", err)
			return
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
		err = fmt.Errorf("cannot push image manifest: %w", err)
		return
	} else {
		mfw.Write(nmf)
		err = mfw.Commit(ctx, mfdesc.Size, mfdesc.Digest)
		if err != nil && !errdefs.IsAlreadyExists(err) {
			err = fmt.Errorf("cannot push image: %w", err)
			return
		}
	}

	return chkcfg, true, nil
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

func getImageMetadata(ctx context.Context, ref reference.Reference, resolver remotes.Resolver) (absref reference.Digested, manifest *ociv1.Manifest, config *ociv1.Image, err error) {
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

func (p *ProjectChunk) hash(baseref string) (res string, err error) {
	if p.cachedHash != "" {
		return p.cachedHash, nil
	}

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
	p.cachedHash = res
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

func (p *ProjectChunk) buildAsBase(ctx context.Context, dest reference.Named, sess *BuildSession) (absref reference.Digested, err error) {
	_, desc, err := sess.opts.Resolver.Resolve(ctx, dest.String())
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
		resp, err := sess.Client.Solve(ctx, nil, client.SolveOpt{
			Frontend:     "dockerfile.v0",
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
				"context":    p.ContextPath,
				"dockerfile": p.ContextPath,
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

		isTTY := isatty.IsTerminal(os.Stderr.Fd())
		if !sess.opts.PlainOutput && isTTY {
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

func (p *ProjectChunk) build(ctx context.Context, sess *BuildSession) (chkRef reference.NamedTagged, didBuild bool, err error) {
	if !sess.opts.NoTests && len(p.Tests) > 0 {
		// build temp image for testing
		testRef, didBuild, err := p.buildImage(ctx, ImageTypeTest, sess)
		if err != nil {
			return nil, false, err
		}

		if didBuild {
			_, _, imgcfg, err := getImageMetadata(ctx, testRef, sess.opts.Resolver)
			if err != nil {
				return nil, false, err
			}

			log.WithField("chunk", p.Name).Warn("running tests")
			executor := buildkit.NewExecutor(sess.Client, testRef.String(), imgcfg)
			_, ok := test.RunTests(ctx, executor, p.Tests)
			if !ok {
				return nil, false, fmt.Errorf("%s: tests failed", p.Name)
			}
		}
	}

	// build actual full image
	fullRef, didBuild, err := p.buildImage(ctx, ImageTypeFull, sess)
	if err != nil {
		return
	}

	// remove base image
	chktpe := ImageTypeChunked
	if sess.opts.ChunkedWithoutHash {
		chktpe = ImageTypeChunkedNoHash
	}
	chkRef, err = p.ImageName(chktpe, sess)
	if err != nil {
		return
	}
	log.WithField("chunk", p.Name).WithField("ref", chkRef).Warn("building chunked image")
	_, didBuild, err = removeBaseLayer(ctx, sess.opts.Resolver, sess.baseMF, sess.baseCfg, fullRef, chkRef)
	if err != nil {
		return
	}

	return
}

// ChunkImageType describes the chunk build artifact type
type ChunkImageType string

const (
	// ImageTypeTest is an image built for testing
	ImageTypeTest ChunkImageType = "test"
	// ImageTypeFull is the full chunk image
	ImageTypeFull ChunkImageType = "full"
	// ImageTypeChunked is the chunk image with the base layers removed
	ImageTypeChunked ChunkImageType = "chunked"
	// ImageTypeChunkedNoHash is the chunk image with the base layers removed and no hash in the name
	ImageTypeChunkedNoHash ChunkImageType = "chunked-wohash"
)

// ImageName produces a chunk image name
func (p *ProjectChunk) ImageName(tpe ChunkImageType, sess *BuildSession) (reference.NamedTagged, error) {
	if sess.baseRef == nil {
		return nil, fmt.Errorf("base ref not set")
	}

	if tpe == ImageTypeChunkedNoHash {
		return reference.WithTag(sess.Dest, fmt.Sprintf("%s", p.Name))
	}

	hash, err := p.hash(sess.baseRef.String())
	if err != nil {
		return nil, fmt.Errorf("cannot compute chunk hash: %w", err)
	}
	return reference.WithTag(sess.Dest, fmt.Sprintf("%s--%s--%s", p.Name, hash, tpe))
}

func (p *ProjectChunk) buildImage(ctx context.Context, tpe ChunkImageType, sess *BuildSession) (tgt reference.Named, didBuild bool, err error) {
	tgt, err = p.ImageName(tpe, sess)
	if err != nil {
		return
	}

	_, _, err = sess.opts.Resolver.Resolve(ctx, tgt.String())
	if err == nil {
		// image is already built
		return tgt, false, nil
	}

	log.WithField("chunk", p.Name).WithField("ref", tgt).Warnf("building %s image", tpe)
	didBuild = true

	eg, ctx := errgroup.WithContext(ctx)
	ch := make(chan *client.SolveStatus)

	var (
		cacheImports = []client.CacheOptionsEntry{
			{
				Type: "registry",
				Attrs: map[string]string{
					"ref": tgt.String(),
				},
			},
		}
		cacheExports = []client.CacheOptionsEntry{
			{
				Type: "inline",
			},
		}
	)
	if sess.opts.NoCache {
		cacheImports = []client.CacheOptionsEntry{}
		cacheExports = []client.CacheOptionsEntry{}
	}

	rchan := make(chan map[string]string, 1)
	eg.Go(func() error {
		resp, err := sess.Client.Solve(ctx, nil, client.SolveOpt{
			Frontend: "dockerfile.v0",
			FrontendAttrs: map[string]string{
				"build-arg:base": sess.baseRef.String(),
			},
			CacheImports: cacheImports,
			CacheExports: cacheExports,
			Session: []session.Attachable{
				authprovider.NewDockerAuthProvider(os.Stderr),
			},
			Exports: []client.ExportEntry{
				{
					Type: "image",
					Attrs: map[string]string{
						"name": tgt.String(),
						"push": "true",
					},
				},
			},
			LocalDirs: map[string]string{
				"context":    p.ContextPath,
				"dockerfile": p.ContextPath,
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

		isTTY := isatty.IsTerminal(os.Stderr.Fd())
		if !sess.opts.PlainOutput && isTTY {
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
	resref, err := reference.WithDigest(tgt, dgst)
	if err != nil {
		return
	}
	return resref, didBuild, nil
}

package dazzle

import (
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar"
	"github.com/csweichel/dazzle/pkg/test"
	"github.com/docker/distribution/reference"
	"github.com/minio/highwayhash"
	ignore "github.com/sabhiram/go-gitignore"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

// ProjectConfig is the structure of a project's dazzle.yaml
type ProjectConfig struct {
	ChunkIgnore []string `yaml:"ignore"`

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

	var (
		testsDir  = filepath.Join(dir, "tests")
		chunksDir = filepath.Join(dir, "chunks")
	)

	base, err := loadChunk(dir, testsDir, "base")
	if err != nil {
		return nil, err
	}

	res := &Project{
		Base: *base,
	}
	chds, err := ioutil.ReadDir(chunksDir)
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
		chnk, err := loadChunk(chunksDir, testsDir, chd.Name())
		if err != nil {
			return nil, err
		}
		res.Chunks = append(res.Chunks, *chnk)
		log.WithField("name", chnk.Name).Info("added chunk to project")
	}

	return res, nil
}

func loadChunk(chunkbase, testbase, name string) (*ProjectChunk, error) {
	dir := filepath.Join(chunkbase, name)
	res := &ProjectChunk{
		Name:        name,
		ContextPath: dir,
	}

	var err error
	res.Dockerfile, err = ioutil.ReadFile(filepath.Join(dir, "Dockerfile"))
	if err != nil {
		return nil, err
	}

	tf, err := ioutil.ReadFile(filepath.Join(testbase, fmt.Sprintf("%s.yaml", name)))
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("%s: cannot read tests.yaml: %w", dir, err)
	}
	err = yaml.UnmarshalStrict(tf, &res.Tests)
	if err != nil {
		return nil, fmt.Errorf("%s: cannot read tests.yaml: %w", dir, err)
	}
	return res, nil
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
			res = append(res, strings.TrimPrefix(src, p.ContextPath))
			continue
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

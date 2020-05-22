module github.com/csweichel/dazzle

go 1.14

require (
	github.com/GeertJohan/go.rice v1.0.0
	github.com/alecthomas/jsonschema v0.0.0-20200514014646-0366d1034a17 // indirect
	github.com/alecthomas/repr v0.0.0-20200325044227-4184120f674c
	github.com/bmatcuk/doublestar v1.3.0
	github.com/containerd/console v0.0.0-20191219165238-8375c3424e4d
	github.com/containerd/containerd v1.4.0-0
	github.com/creack/pty v1.1.10
	github.com/docker/cli v0.0.0-20200227165822-2298e6a3fe24
	github.com/docker/distribution v0.0.0-20200223014041-6b972e50feee
	github.com/gookit/color v1.2.5
	github.com/manifoldco/promptui v0.7.0
	github.com/minio/highwayhash v1.0.0
	github.com/moby/buildkit v0.7.1
	github.com/opencontainers/go-digest v1.0.0-rc1
	github.com/opencontainers/image-spec v1.0.1
	github.com/robertkrimen/otto v0.0.0-20191219234010-c382bd3c16ff
	github.com/sirupsen/logrus v1.6.0
	github.com/spf13/cobra v1.0.0
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e
	gopkg.in/sourcemap.v1 v1.0.5 // indirect
	gopkg.in/yaml.v2 v2.3.0
)

replace (
	github.com/containerd/containerd => github.com/containerd/containerd v1.3.1-0.20200512144102-f13ba8f2f2fd
	github.com/docker/docker => github.com/docker/docker v17.12.0-ce-rc1.0.20200310163718-4634ce647cf2+incompatible
	github.com/hashicorp/go-immutable-radix => github.com/tonistiigi/go-immutable-radix v0.0.0-20170803185627-826af9ccf0fe
	github.com/jaguilar/vt100 => github.com/tonistiigi/vt100 v0.0.0-20190402012908-ad4c4a574305
)

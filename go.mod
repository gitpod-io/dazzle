module github.com/csweichel/dazzle

go 1.16

require (
	github.com/GeertJohan/go.rice v1.0.2
	github.com/alecthomas/jsonschema v0.0.0-20210526225647-edb03dcab7bc
	github.com/alecthomas/repr v0.0.0-20210301060118-828286944d6a
	github.com/bmatcuk/doublestar v1.3.4
	github.com/containerd/console v1.0.2
	github.com/containerd/containerd v1.5.4
	github.com/creack/pty v1.1.13
	github.com/docker/cli v20.10.7+incompatible
	github.com/docker/distribution v2.7.1+incompatible
	github.com/google/go-cmp v0.5.4
	github.com/gookit/color v1.4.2
	github.com/manifoldco/promptui v0.8.0
	github.com/mattn/go-isatty v0.0.13
	github.com/minio/highwayhash v1.0.2
	github.com/moby/buildkit v0.8.3
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.0.2
	github.com/robertkrimen/otto v0.0.0-20200922221731-ef014fd054ac
	github.com/sabhiram/go-gitignore v0.0.0-20201211210132-54b8a0bf510f
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/cobra v1.1.3
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	gopkg.in/sourcemap.v1 v1.0.5 // indirect
	gopkg.in/yaml.v2 v2.4.0
)

replace (
	// protobuf: corresponds to containerd
	github.com/golang/protobuf => github.com/golang/protobuf v1.3.5
	github.com/hashicorp/go-immutable-radix => github.com/tonistiigi/go-immutable-radix v0.0.0-20170803185627-826af9ccf0fe
	github.com/jaguilar/vt100 => github.com/tonistiigi/vt100 v0.0.0-20190402012908-ad4c4a574305
	// genproto: corresponds to containerd
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20200224152610-e50cd9704f63
	// grpc: corresponds to protobuf
	google.golang.org/grpc => google.golang.org/grpc v1.30.0
)

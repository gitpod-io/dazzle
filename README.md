<img src="logo.png" width="100" style="padding: 1em; background-color: white; border-radius: 10px;">

[![Setup Automated](https://img.shields.io/badge/setup-automated-blue?logo=gitpod)](https://gitpod.io/#https://github.com/csweichel/dazzle)
[![Go Report Card](https://goreportcard.com/badge/github.com/csweichel/dazzle)](https://goreportcard.com/report/github.com/csweichel/dazzle)
[![Stability: Experimental](https://masterminds.github.io/stability/experimental.svg)](https://masterminds.github.io/stability/experimental.html)

dazzle is a rather experimental Docker/OCI image builder. Its goal is to build independent layers where a change to one layer does *not* invalidate the ones sitting "above" it.

**Beware** Recently the format for [dazzle builds was changed](https://github.com/gitpod-io/dazzle/commit/ceaa19ef6562e03108c8ea9474d2c627a452a7ca), moving from a single Dockerfile to one per "chunk"/layer. It is also about 5x faster, more reliable and less hacky. 

## How does it work?
dazzle has three main capabilities.
1. _build independent layer chunks_: in a dazzle project there's a `chunks/` folder which contains individual Dockerfiles (e.g. `chunks/something/Dockerfile`). These chunk images are built independently of each other. All of them share the same base image using a special build argument `${base}`. Dazzle can build the base image (built from `base/Dockerfile`), as well as the chunk images. After each chunk image build dazzle will remove the base image layer from that image, leaving just the layers that were produced by the chunk Dockerfile.
2. _merge layers into one image_: dazzle can merge multiple OCI images/chunks (not just those built using dazzle) by building a new manifest and image config that pulls the layers/DiffIDs from the individual chunks and the base image they were built from.
3. _run tests against images_: to ensure that an image is capable of what we think it should be - especially after merging - dazzle supports simple tests and assertions that run against Docker images.

## Would I want to use this?
Not ordinarily, no. For example, if you're packing your service/app/software/unicorn you're probably better of with a regular Docker image build and well established means for optimizing that one (think multi-stage builds, proper layer ordering).

If however you are building images which consist of a lot of independent "concerns", i.e. chunks that can be strictly separated, then this might for you.
For example, if you're building an image that serves as a collection of tools, the layer hierarchy imposed by regular builds doesn't fit so well.

## Limitations and caveats
- build args are not supported at the moment
- there are virtually no tests covering this so things might just break
- consider this alpha-level software

### Requirements
Install and run [buildkit](https://github.com/moby/buildkit/releases) - currently 0.8.3 - in the background.
Pull and run a docker registry.

NOTE: if you are running it in Gitpod this is done for you! 

```bash
sudo su -c "cd /usr; curl -L https://github.com/moby/buildkit/releases/download/v0.8.3/buildkit-v0.8.3.linux-amd64.tar.gz | tar xvz"
docker run -p 5000:5000 --name registry --rm registry:2
```

## Getting started
```bash
# start a new project
dazzle project init

# add our first chunk
dazzle project init helloworld
echo hello world > chunks/helloworld/hello.txt
echo "COPY hello.txt /" >> chunks/helloworld/Dockerfile 

# add another chunk, just for fun
dazzle project init anotherchunk
echo some other chunk > chunks/anotherchunk/message.txt
echo "COPY message.txt /" >> chunks/anotherchunk/Dockerfile

# register a combination which takes in all the chunks
dazzle project add-combination full helloworld anotherchunk

# build the chunks
dazzle build eu.gcr.io/some-project/dazzle-test

# build all combinations
dazzle combine eu.gcr.io/some-project/dazzle-test --all
```

# Usage

## init
```
$ dazzle project init
Starts a new dazzle project

Usage:
  dazzle project init [chunk] [flags]

Flags:
  -h, --help   help for init

Global Flags:
      --addr string      address of buildkitd (default "unix:///run/buildkit/buildkitd.sock")
      --context string   context path (default "/workspace/workspace-images")
  -v, --verbose          enable verbose logging
```

Starts a new dazzle project. If you don't know where to start, this is the place.

## build
```
$ dazzle build --help
Builds a Docker image with independent layers

Usage:
  dazzle build <target-ref> [flags]

Flags:
      --chunked-without-hash   disable hash qualification for chunked image
  -h, --help                   help for build
      --no-cache               disables the buildkit build cache
      --plain-output           produce plain output

Global Flags:
      --addr string      address of buildkitd (default "unix:///run/buildkit/buildkitd.sock")
      --context string   context path (default "/workspace/workspace-images")
  -v, --verbose          enable verbose logging
```

Dazzle can build regular Docker files much like `docker build` would. `build` will build all images found under `chunks/`.

Dazzle cannot reproducibly build layers but can only re-use previously built ones. To ensure reusable layers and maximize Docker cache hits, dazzle itself caches the layers it builds in a Docker registry.

## combine
```
$ dazzle combine --help
Combines previously built chunks into a single image

Usage:
  dazzle combine <target-ref> [flags]

Flags:
      --all                  build all combinations
      --build-ref string     use a different build-ref than the target-ref
      --chunks string        combine a set of chunks - format is name=chk1,chk2,chkN
      --combination string   build a specific combination
  -h, --help                 help for combine
      --no-test              disables the tests

Global Flags:
      --addr string      address of buildkitd (default "unix:///run/buildkit/buildkitd.sock")
      --context string   context path (default "/workspace/workspace-images")
  -v, --verbose          enable verbose logging
```

Dazzle can combine previously built chunks into a single image. For example `dazzle combine some.registry.com/dazzle --chunks foo=chunk1,chunk2` will combine `base`, `chunk1` and `chunk2` into an image called `some.registry.com/dazzle:foo`.
One can pre-register such chunk combinations using `dazzle project add-combination`.

The `dazzle.yaml` file specifies the list of available combinations. Those combinations can also reference each other:
```yaml
combiner:
  combinations:
  - name: minimal
    chunks:
    - golang
  - name: some-more
    ref:
    - minimal
    chunks:
    - node
```

### Testing layers and merged images
During a dazzle build one can test the individual layers and the final image.
During the build dazzle will execute the layer tests for each individual layer, as well as the final image.
This makes finding and debugging issues created by the layer merge process tractable.

Each chunk gets its own set of tests found under `tests/chunk.yaml`.

For example:
```YAML
- desc: "it should demonstrate tests"
  command: ["echo", "hello world"]
  assert:
  - status == 0
  - stdout.indexOf("hello") != -1
  - stderr.length == 0
- desc: "it should handle exit codes"
  command: ["sh", "-c", "exit 1"]
  assert:
  - status == 1
- desc: "it should have environment variables"
  command: ["sh", "-c", "echo $MESSAGE"]
  env:
  - MESSAGE=foobar
  assert:
  - stdout.trim() == "foobar"
```

### Assertions
All test assertions are written in [ES5 Javascript](https://github.com/robertkrimen/otto).
Three variables are available in an assertion:
- `stdout` contains the standard output produced by the command
- `stderr` contains the standard error output produced by the command
- `status` contains the exit code of the command/container.

The assertion itself must evaluate to a boolean value, otherwise the test fails.

### Testing approach
While the test runner is standalone, the linux+amd64 version is embedded into the dazzle binary using [go.rice](https://github.com/GeertJohan/go.rice) and go generate - see [build.sh](./pkg/test/runner/build.sh).
TODO: use go:embed?
Note that if you make changes to code in the test runner you will need to re-embed the runner into the binary in order to use it via dazzle.
```bash
go generate ./...
```

The test runner binary is extracted and copied to the generated image where it is run using an encoded JSON version of the test specification - see [container.go](pkg/test/buildkit/container.go).
The exit code, stdout & stderr are captured and returned for evaluation against the assertions in the test specification.

While of limited practical use, it is *possible* to run the test runner standalone using a base64-encoded JSON blob as a parameter:
```bash
$ go run pkg/test/runner/main.go eyJEZXNjIjoiaXQgc2hvdWxkIGhhdmUgR28gaW4gdmVyc2lvbiAxLjEzIiwiU2tpcCI6ZmFsc2UsIlVzZXIiOiIiLCJDb21tYW5kIjpbImdvIiwidmVyc2lvbiJdLCJFbnRyeXBvaW50IjpudWxsLCJFbnYiOm51bGwsIkFzc2VydGlvbnMiOlsic3Rkb3V0LmluZGV4T2YoXCJnbzEuMTFcIikgIT0gLTEiXX0=
{"Stdout":"Z28gdmVyc2lvbiBnbzEuMTYuNCBsaW51eC9hbWQ2NAo=","Stderr":"","StatusCode":0}
```

The stdout/err are returned as base64-encoded values.
They can be extracted using jq e.g.:
```bash
$ go run pkg/test/runner/main.go eyJEZXNjIjoiaXQgc2hvdWxkIGhhdmUgR28gaW4gdmVyc2lvbiAxLjEzIiwiU2tpcCI6ZmFsc2UsIlVzZXIiOiIiLCJDb21tYW5kIjpbImdvIiwidmVyc2lvbiJdLCJFbnRyeXBvaW50IjpudWxsLCJFbnYiOm51bGwsIkFzc2VydGlvbnMiOlsic3Rkb3V0LmluZGV4T2YoXCJnbzEuMTFcIikgIT0gLTEiXX0= | jq -r '.Stdout | @base64d'
go version go1.16.4 linux/amd64
```

<img src="logo.png" width="100" style="padding: 1em; background-color: white; border-radius: 10px;">

[![Setup Automated](https://img.shields.io/badge/setup-automated-blue?logo=gitpod)](https://gitpod.io/#https://github.com/csweichel/dazzle)
[![Go Repord Cart](https://goreportcard.com/badge/github.com/csweichel/dazzle)](https://goreportcard.com/report/github.com/csweichel/dazzle)
[![Stability: Experimental](https://masterminds.github.io/stability/experimental.svg)](https://masterminds.github.io/stability/experimental.html)

dazzle is a rather experimental Docker/OCI image builder. Its goal is to build independent layers where a change to one layer does *not* invalidate the ones sitting "above" it.

**Beware** There's a bit of a [rewrite](https://github.com/csweichel/dazzle/tree/rewrite) going on which makes dazzle about 5x faster, more reliable and less hacky. It also changes the format for dazzle builds, moving from a single Dockerfile to one per "chunk"/layer.

## How does it work?
dazzle has three main capabilities.
1. _build indepedent layer chunks_: in a dazzle project there's a `chunks/` folder which contains individual Dockerfiles (e.g. `chunks/something/Dockerfile`). These chunk images are built indepedently of each other. All of them share the same base image using a special build argument `${base}`. Dazzle can build the base image (built from `base/Dockerfile`), as well as the chunk images. After each chunk image build dazzle will remove the base image layer from that image, leaving just the layers that were produced by the chunk Dockerfile.
2. _merge layers into one image_: dazzle can merge multiple OCI images/chunks (not just those built using dazzle) by building a new manifest and image config that pulls the layers/DiffIDs from the individual chunks and the base image they were built from.
3. _run tests against images_: to ensure that an image is capable of what we think it should be - epecially after merging - dazzle supports simple tests and assertions that run against Docker images.

## Would I want to use this?
Not ordinarily, no. For example, if you're packing your service/app/software/unicorn you're probably better of with a regular Docker image build and well established means for optimizing that one (think multi-stage builds, proper layer ordering).

If however you are building images which consist of a lot of independent "concerns", i.e. chunks that can be strictly seperated, then this might for you.
For example, if you're building an image that serves as a collection of tools, the layer hierarchy imposed by regular builds doesn't fit so well.

## Limitations and caveats
- build args are not suppported at the moment
- there are virtually no tests covering this so things might just break
- consider this alpa-level software

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

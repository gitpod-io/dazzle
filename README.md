![Dazzle](logo.png | width=100)

dazzle is a rather experimental Docker image builder. Its goal is to build independent layers where a change to one layer does *not* invalidate the ones sitting "above" it. To this end, dazzle uses black magic.

## How does it work?
dazzle has three main capabilities.
1. _build indepedent layers_: dazzle uses a special label in a Docker file to establish "boundaries of independence", or meta layers if you so like. Statements in the form of `LABEL dazzle/layer=somename` establish those bounds. All content prior to the first label is used as base image for the other layers. Come build-time, dazzle will split the Dockerfile at the label statement and build them individually. This prevents accidential cross-talk between the layers.
2. _merge layers into one image_: to merge any two Docker images (not just those built using dazzle), dazzle uses the Docker tar export to extract the base image and all "addons" (i.e. images that are to be merged). It then manipulates the manifests and image configurations such that upon re-import a single image exists. The process is a bit of a hack and like black magic fragile, possibly error prone and needs a black cat or two to work.
3. _run tests against images_: to ensure that an image is capable of what we think it should be - epecially after merging - dazzle supports simple tests and assertions that run against Docker images.

## Would I want to use this?
Not ordinarily, no. For example, if you're packing your service/app/software/unicorn you're probably better of with a regular Docker image build and well established means for optimizing that one (think multi-stage builds, proper layer ordering).

If however you are building images which consist of a lot of independent "concerns", i.e. chunks that can be strictly seperated, then this might for you.
For example, if you're building an image that serves as a collection of tools, the layer hierarchy imposed by regular builds doesn't fit so well.

## Limitations and caveats
- build args are not suppported at the moment
- multi-stage builds are not supported and probably never will be (there's no limitation on merging images that were created using multi-stage builds though)
- there are virtually no tests covering this so things might just break
- consider this alpa-level software

# Usage

## build
```
$ dazzle build --help
Builds a Docker image with independent layers

Usage:
  dazzle build [context] [flags]

Flags:
  -f, --file string         name of the Dockerfile (default "Dockerfile")
  -h, --help                help for build
  -r, --repository string   name of the Docker repository to work in (e.g. eu.gcr.io/someprj/dazzle-work) (default "dazzle-work")
  -t, --tag string          tag of the resulting image (default "dazzle-built:latest")
```

Dazzle can build a regular Docker file much like `docker build` would. To benefit from using dazzle the `Dockerfile` must contain labels that split the file into "layers" or chunks.
These layers remain stable across builds, even if prior/"lower" layers change.

Dazzle cannot reproducibly build layers but can only re-use previously built ones. To ensure reusable layers and maximize Docker cache hits, dazzle itself caches the layers it builds in a Docker registry.
It is important you use the `--repository` flag to enable this caching. Otherwise you will not have stable layers accross builds.

Consider the following Dockerfile:
```Dockerfile
FROM alpine:3.9

RUN touch /base-image

LABEL dazzle/layer=golang
LABEL dazzle/test=test-golang.yaml
RUN apk add --no-cache git make musl-dev go
ENV GOPATH=/root/go
ENV PATH=$GOPATH/bin:$PATH

LABEL dazzle/layer=node
RUN apk add --no-cache nodejs
```

Notice the `LABEL dazzle/layer` entries. A change to the `RUN apk add --no-cache git make musl-dev go` line would **not** re-create the layer created by `RUN apk add --no-cache nodejs` because both are separated by different `dazzle/layer` labels.

This Dockerfile would result in four seperate builds:
1. The "base image" contains everything before the first `dazzle/layer` entry
   ```Dockerfile
   FROM alpine:3.9
   RUN touch /base-image
   ```
2. The "golang" layer which contains
   ```Dockerfile
   FROM base-image
   RUN apk add --no-cache git make musl-dev go
   ENV GOPATH=/root/go
   ENV PATH=$GOPATH/bin:$PATH
   ```
3. The "node" layer which contains
   ```Dockerfile
   FROM base-image
   RUN apk add --no-cache nodejs
   ```
4. The _prologue_ build which is built after the first three were merged into one image and contains
   ```Dockerfile
   FROM merged-image
   ENV GOPATH=/root/go
   ENV PATH=$GOPATH/bin:$PATH
   ```

### Testing layers and merged images
During a multi-layer dazzle build one can test the individual layers and the final image.
To do this end add `LABEL dazzle/test=my-layer-test.yaml` statements within a dazzle layer.
During the build dazzle will execute the layer tests for each individual layer, as well as the final image.
This makes finding and debugging issues created by the layer merge process tractable.

## test
```
Runs a dazzle test suite

Usage:
  dazzle test <suite.yaml> [flags]

Flags:
  -h, --help                help for test
  -i, --image string        run test against this image (overwriting the images specified in the test suite)
      --output-xml string   save result as XML file
```

Dazzle can test images.
Each test consists of a command that is executed and a number of assertions against the result of that execution.
Tests are written in YAML whoose schema is available in `testspec.schema.json`.

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

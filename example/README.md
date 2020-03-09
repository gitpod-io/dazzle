Dazzle builds several images which all share the same base image.
We call those images `chunks` (because we treat them as chunks of layers - they're not useable images on their own).

A dazzle project consists of multiple folders, where each folder defines a single chunk.
There must be at least one folder called `base` which defines the common base image.

All chunks must have a `Dockerfile` and a `spec.yaml` - otherwise they are not a valid chunk and cannot be built.
A chunk can have variants, e.g. the version of a tool (see the `go/` for an example).
All chunks can have tests which are executed after the chunk has been built.

Several chunks can be assembled into a final image.
Creating multiple images from the same chunks will re-use the layers that make up the chunks.

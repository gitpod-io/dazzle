#!/bin/bash

# Build and run the example
gp await-port 5000

echo building chunks
go run main.go build --context example localhost:5000/dazzle

echo building chunks without hashes
go run main.go build --chunked-without-hash --context example localhost:5000/dazzle

echo combining using dazzle.yaml
go run main.go combine --context example localhost:5000/dazzle --all

echo combining using references
go run main.go combine-from-ref localhost:5000/dazzle:comb localhost:5000/dazzle/golang:1.16.4 localhost:5000/dazzle/node:14.17.0

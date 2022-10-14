#!/bin/bash

# Build and run the example
gp await-port 5000
go run main.go build --context example localhost:5000/dazzle --compression gzip
go run main.go combine --context example localhost:5000/dazzle --all --compression gzip

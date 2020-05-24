#!/bin/sh
set -e

GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/runner_linux_amd64 main.go
curl -L https://github.com/upx/upx/releases/download/v3.96/upx-3.96-amd64_linux.tar.xz | tar xJ
upx-3.96-amd64_linux/upx bin/runner_linux_amd64 
rm -r upx-3.96-amd64_linux
go get github.com/GeertJohan/go.rice/rice
rice embed-go -i github.com/csweichel/dazzle/pkg/test/runner

if [ $(ls -l bin/runner_linux_amd64 | cut -d ' ' -f 5) -gt 3437900 ]; then
    echo "runner binary is too big (> gRPC message size)"
    exit 1
fi
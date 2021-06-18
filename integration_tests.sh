#!/bin/bash

# Run the integration tests
# NOTE: the TARGET_REF is hostname:port 
# shellcheck disable=SC2034
BUILDKIT_ADDR=unix:///run/buildkit/buildkitd.sock TARGET_REF=127.0.0.1:5000 go test -count 1 -run ^Test.*_integration$ github.com/csweichel/dazzle/pkg/dazzle -v

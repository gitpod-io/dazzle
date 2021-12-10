//go:build runner
// +build runner

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/gitpod-io/dazzle/pkg/test"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <base64-encoded-spec>\n", os.Args[0])
		os.Exit(1)
	}

	buf, err := base64.StdEncoding.DecodeString(os.Args[1])
	if err != nil {
		fail(fmt.Errorf("cannot decode spec: %w", err))
	}

	var spec test.Spec
	err = json.Unmarshal(buf, &spec)
	if err != nil {
		fail(fmt.Errorf("cannot unmarshal spec: %w", err))
	}

	executor := test.LocalExecutor{}
	res, err := executor.Run(context.Background(), &spec)
	if err != nil {
		res = &test.RunResult{
			Stderr:     []byte(fmt.Sprintf("cannot run command: %+q\nenv: %s\n", err, strings.Join(os.Environ(), "\n\t"))),
			StatusCode: 255,
		}
	}

	err = json.NewEncoder(os.Stdout).Encode(res)
	if err != nil {
		fail(fmt.Errorf("cannot marshal result: %w", err))
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err.Error())
	os.Exit(2)
}

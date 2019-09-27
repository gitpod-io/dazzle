package main

import (
	"context"
	"fmt"
	"os"
	"io"

	"github.com/docker/docker/api/types"
	docker "github.com/docker/docker/client"
	_ "github.com/opencontainers/image-spec/specs-go/v1"
)

func main() {
	args := os.Args
	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s base-ref layers...\n", args[0])
		return
	}

	ctx := context.Background()
	cli, err := docker.NewEnvClient()
	if err != nil {
		panic(err)
	}

	baseimg := args[1]
	layerimgs := args[2:]
	for _, ref := range append(layerimgs, baseimg) {
		reader, err := cli.ImagePull(ctx, ref, types.ImagePullOptions{
			All: true,
		})
		if err != nil {
			panic(err)
		}
		io.Copy(os.Stdout, reader)
	}
}

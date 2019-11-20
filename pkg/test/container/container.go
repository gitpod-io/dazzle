package container

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/32leaves/dazzle/pkg/test"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

// DockerExecutor can run test commands in a Docker container
type DockerExecutor struct {
	Client   *client.Client
	ImageRef string
}

// Run executes the test command
func (e DockerExecutor) Run(ctx context.Context, s *test.Spec) (res *test.RunResult, err error) {
	containerName := fmt.Sprintf("dazzle-test-%x", sha256.Sum256([]byte(fmt.Sprintf("%s-%s-%d", e.ImageRef, s.Desc, time.Now().UnixNano()))))
	ctnt, err := e.Client.ContainerCreate(ctx, &container.Config{
		Image:        e.ImageRef,
		User:         s.User,
		Env:          s.Env,
		Cmd:          s.Command,
		Entrypoint:   s.Entrypoint,
		Tty:          false,
		AttachStdout: true,
		AttachStderr: true,
	}, &container.HostConfig{}, nil, containerName)
	if err != nil {
		return nil, err
	}
	defer e.Client.ContainerRemove(ctx, ctnt.ID, types.ContainerRemoveOptions{Force: true})

	atch, err := e.Client.ContainerAttach(ctx, ctnt.ID, types.ContainerAttachOptions{
		Stream: true,
		Stdin:  false,
		Stdout: true,
		Stderr: true,
	})
	if err != nil {
		return nil, err
	}

	errchan := make(chan error, 1)
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	go func() {
		_, err := stdcopy.StdCopy(stdout, stderr, atch.Reader)
		errchan <- err
	}()

	err = e.Client.ContainerStart(ctx, ctnt.ID, types.ContainerStartOptions{})
	if err != nil {
		return nil, err
	}

	var statusCode int64
	okchan, cerrchan := e.Client.ContainerWait(ctx, ctnt.ID, container.WaitConditionNotRunning)
	select {
	case ste := <-okchan:
		statusCode = ste.StatusCode
	case err := <-cerrchan:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	err = <-errchan
	if err != nil {
		return nil, err
	}

	return &test.RunResult{
		Stdout:     stdout.Bytes(),
		Stderr:     stderr.Bytes(),
		StatusCode: statusCode,
	}, nil
}

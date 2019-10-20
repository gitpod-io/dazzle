package test

//go:generate sh -c "go run generate-schema.go > ../../testspec.schema.json"

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/xml"
	"fmt"
	"time"

	"github.com/alecthomas/repr"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/robertkrimen/otto"
	log "github.com/sirupsen/logrus"
)

// Spec specifies a command execution test against a Docker image
type Spec struct {
	Desc string `yaml:"desc"`

	Skip       bool     `yaml:"skip,omitempty"`
	User       string   `yaml:"user,omitempty"`
	Command    []string `yaml:"command,flow"`
	Entrypoint []string `yaml:"entrypoint,omitempty,flow"`
	Env        []string `yaml:"env,omitempty"`

	Assertions []string `yaml:"assert"`
}

// Result is the result of a test
type Result struct {
	XMLName xml.Name `xml:"testsuite"`

	Desc     string `yaml:"desc" xml:"name,attr"`
	ImageRef string `yaml:"image" xml:"classname,attr"`

	Skipped bool       `yaml:"skipped,omitempty" xml:"skippped"`
	Error   *ErrResult `yaml:"error,omitempty" xml:"error"`
	Failure *ErrResult `yaml:"failure,omitempty" xml:"failure"`

	*RunResult
}

// ErrResult indicates failure
type ErrResult struct {
	Message string `yaml:"message" xml:"message,attr"`
	Type    string `yaml:"type" xml:"type,attr"`
}

// Results is a collection of test results
type Results struct {
	XMLName xml.Name `xml:"testsuites"`

	Result []*Result `yaml:"results" xml:"testsuite"`
}

// RunTests executes a series of tests
func RunTests(ctx context.Context, docker *client.Client, imageRef string, tests []*Spec) (res Results, success bool) {
	success = true

	var results []*Result
	for i, tst := range tests {
		if tst.Skip {
			log.WithField("step", i).Warnf("skipping \"%s\"", tst.Desc)
		} else {
			log.WithField("step", i).WithField("command", tst.Command).Infof("testing \"%s\"", tst.Desc)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		r := tst.Run(ctx, docker, imageRef)
		results = append(results, r)
		cancel()

		if r.Error != nil {
			success = false
			log.WithField("emoji", "🐲").WithField("message", r.Error.Message).Error("error")
			continue
		}
		if r.Failure != nil {
			success = false
			log.WithField("result", repr.String(r.RunResult)).WithField("message", r.Failure.Message).Error("failed")
			continue
		}
		if r.Skipped {
			continue
		}

		log.WithField("status", "passed").Info("passed")
		continue
	}

	res = Results{Result: results}
	return
}

// Run executes the test
func (s *Spec) Run(ctx context.Context, docker *client.Client, imageRef string) (res *Result) {
	res = &Result{
		Desc:     s.Desc,
		ImageRef: imageRef,
		Skipped:  s.Skip,
	}
	if s.Skip {
		return
	}

	runres, err := s.RunContainer(ctx, docker, imageRef)
	if err != nil {
		res.Error = &ErrResult{
			Message: err.Error(),
			Type:    "docker",
		}
		return
	}

	res.RunResult = runres
	err = ValidateAssertions(res, s.Assertions, runres)
	if err != nil {
		res.Error = &ErrResult{
			Message: err.Error(),
			Type:    "assertion",
		}
		return
	}

	return
}

// ValidateAssertions runs the assertions of a test spec against a run result and sets the result appropriately
func ValidateAssertions(res *Result, assertions []string, runres *RunResult) error {
	vm := otto.New()
	vm.Set("stdout", string(runres.Stdout))
	vm.Set("stderr", string(runres.Stderr))
	vm.Set("status", runres.StatusCode)

	for _, assertion := range assertions {
		log.Debugf("- %s", assertion)

		val, err := vm.Run(assertion)
		if err != nil {
			return err
		}

		if !val.IsBoolean() {
			return fmt.Errorf("assertion must evaluate to boolean value")
		}

		passed, err := val.ToBoolean()
		if err != nil {
			return err
		}

		if !passed {
			res.Failure = &ErrResult{
				Message: fmt.Sprintf("assertion failed: %s", assertion),
			}
			break
		}
	}

	return nil
}

// RunResult is the direct output produced by a test container
type RunResult struct {
	Stdout     []byte `yaml:"stdout,omitempty" xml:"system-out,omitempty"`
	Stderr     []byte `yaml:"stderr,omitempty" xml:"system-err,omitempty"`
	StatusCode int64  `yaml:"statusCode" xml:"-"`
}

// RunContainer executes the test spec in a Docker container
func (s *Spec) RunContainer(ctx context.Context, docker *client.Client, imageRef string) (*RunResult, error) {
	containerName := fmt.Sprintf("dazzle-test-%x", sha256.Sum256([]byte(fmt.Sprintf("%s-%s-%d", imageRef, s.Desc, time.Now().UnixNano()))))
	ctnt, err := docker.ContainerCreate(ctx, &container.Config{
		Image:        imageRef,
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
	defer docker.ContainerRemove(ctx, ctnt.ID, types.ContainerRemoveOptions{Force: true})

	atch, err := docker.ContainerAttach(ctx, ctnt.ID, types.ContainerAttachOptions{
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

	err = docker.ContainerStart(ctx, ctnt.ID, types.ContainerStartOptions{})
	if err != nil {
		return nil, err
	}

	var statusCode int64
	okchan, cerrchan := docker.ContainerWait(ctx, ctnt.ID, container.WaitConditionNotRunning)
	select {
	case ste := <-okchan:
		statusCode = ste.StatusCode
	case err := <-cerrchan:
		return nil, err
	}

	err = <-errchan
	if err != nil {
		return nil, err
	}

	return &RunResult{
		Stdout:     stdout.Bytes(),
		Stderr:     stderr.Bytes(),
		StatusCode: statusCode,
	}, nil
}

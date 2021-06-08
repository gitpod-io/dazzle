package test

//go:generate sh -c "go run generate-schema.go > ../../testspec.schema.json"

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/alecthomas/repr"
	"github.com/creack/pty"
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

	Desc string `yaml:"desc" xml:"name,attr"`

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

// Executor can run test commands in some environment
type Executor interface {
	Run(ctx context.Context, spec *Spec) (*RunResult, error)
}

// RunResult is the direct output produced by a test container
type RunResult struct {
	Stdout     []byte `yaml:"stdout,omitempty" xml:"system-out,omitempty"`
	Stderr     []byte `yaml:"stderr,omitempty" xml:"system-err,omitempty"`
	StatusCode int64  `yaml:"statusCode" xml:"-"`
}

// LocalExecutor executes tests against the current, local environment
type LocalExecutor struct{}

// Run executes the test
func (LocalExecutor) Run(ctx context.Context, s *Spec) (res *RunResult, err error) {
	env := os.Environ()
	for _, envvar := range s.Env {
		segs := strings.Split(envvar, "=")
		if len(segs) != 2 {
			log.WithField("test", s.Desc).WithField("envvar", envvar).Warn("invalid format - ignoring this envvar")
		}
		nme := segs[0]

		var found bool
		for i, exenvvar := range env {
			segs := strings.Split(exenvvar, "=")
			if segs[0] != nme {
				continue
			}

			env[i] = envvar
			found = true
		}
		if found {
			continue
		}

		env = append(env, envvar)
	}

	var cmd *exec.Cmd
	if len(s.Entrypoint) > 0 {
		var args []string
		args = append(args, s.Entrypoint[1:]...)
		args = append(args, s.Command...)
		cmd = exec.Command(s.Entrypoint[0], args...)
	} else {
		cmd = exec.Command(s.Command[0], s.Command[1:]...)
	}
	cmd.Env = env
	stdout, stderr := bytes.NewBuffer([]byte{}), bytes.NewBuffer([]byte{})
	if s.User != "" {
		user, err := user.LookupId(s.User)
		if err != nil {
			return nil, err
		}
		uid, err := strconv.ParseUint(user.Uid, 10, 32)
		if err != nil {
			return nil, err
		}
		gid, err := strconv.ParseUint(user.Gid, 10, 32)
		if err != nil {
			return nil, err
		}
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}}
	}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if len(s.Entrypoint) > 0 {
		_, err := pty.Start(cmd)
		if err != nil {
			return nil, err
		}
	} else {
		err = cmd.Start()
		if err != nil {
			return nil, err
		}
	}
	err = cmd.Wait()
	if _, ok := err.(*exec.ExitError); ok {
		// the command exited with non-zero exit code - that's no reason to fail here
		err = nil
	} else if err != nil {
		return nil, err
	}

	res = &RunResult{
		Stdout:     stdout.Bytes(),
		Stderr:     stderr.Bytes(),
		StatusCode: int64(cmd.ProcessState.ExitCode()),
	}
	return
}

// RunTests executes a series of tests
func RunTests(ctx context.Context, executor Executor, tests []*Spec) (res Results, success bool) {
	success = true

	var results []*Result
	for i, tst := range tests {
		if tst.Skip {
			log.WithField("step", i).Warnf("skipping \"%s\"", tst.Desc)
		} else {
			log.WithField("step", i).WithField("command", tst.Command).Infof("testing \"%s\"", tst.Desc)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		r := tst.Run(ctx, executor)
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

		log.Info("passed")
		continue
	}

	res = Results{Result: results}
	return
}

// Run executes the test
func (s *Spec) Run(ctx context.Context, executor Executor) (res *Result) {
	res = &Result{
		Desc:    s.Desc,
		Skipped: s.Skip,
	}
	if s.Skip {
		return
	}

	runres, err := executor.Run(ctx, s)
	if err != nil {
		res.Error = &ErrResult{
			Message: err.Error(),
			Type:    "runtime",
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
	_ = vm.Set("stdout", string(runres.Stdout))
	_ = vm.Set("stderr", string(runres.Stderr))
	_ = vm.Set("status", runres.StatusCode)

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

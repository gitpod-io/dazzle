package runner

import (
	"encoding/base64"
	"encoding/json"

	"github.com/csweichel/dazzle/pkg/test"
)

// Args produces the arguments which when passed on the the runner will execute the command
// in a testspec.
func Args(spec *test.Spec) ([]string, error) {
	buf, err := json.Marshal(spec)
	if err != nil {
		return nil, err
	}
	return []string{base64.StdEncoding.EncodeToString(buf)}, nil
}

// UnmarshalRunResult unmarshals run result as produced by the runner
func UnmarshalRunResult(out []byte) (*test.RunResult, error) {
	var res test.RunResult
	err := json.Unmarshal(out, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

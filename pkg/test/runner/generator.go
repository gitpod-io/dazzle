//go:generate sh build.sh

package runner

import (
	"fmt"

	rice "github.com/GeertJohan/go.rice"
)

// GetRunner returns the runner binary for a particular platform
func GetRunner(platform string) ([]byte, error) {
	if platform != "linux_amd64" {
		return nil, fmt.Errorf("unsupported platform %s", platform)
	}

	box, err := rice.FindBox("bin")
	if err != nil {
		return nil, err
	}
	return box.Bytes("runner_" + platform)
}

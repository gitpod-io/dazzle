// Copyright © 2020 Christian Weichel

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package core

import (
	"fmt"
	"os"

	"github.com/csweichel/dazzle/pkg/dazzle"
	"github.com/spf13/cobra"
)

var projectIgnoreCmd = &cobra.Command{
	Use:   "ignore <path> [path ...]",
	Short: "adds a chunk to the ignore list",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := dazzle.LoadProjectConfig(os.DirFS(rootCfg.ContextDir))
		if os.IsNotExist(err) {
			cfg = &dazzle.ProjectConfig{}
		} else if err != nil {
			return fmt.Errorf("cannot load project config: %w", err)
		}

		cfg.ChunkIgnore = append(cfg.ChunkIgnore, args...)

		return cfg.Write(rootCfg.ContextDir)
	},
}

func init() {
	projectCmd.AddCommand(projectIgnoreCmd)
}

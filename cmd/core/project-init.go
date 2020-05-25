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
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// projectInitCmd represents the version command
var projectInitCmd = &cobra.Command{
	Use:   "init [chunk]",
	Short: "Starts a new dazzle project",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) (err error) {
		if len(args) > 0 {
			chk := args[0]
			err = os.MkdirAll(filepath.Join("chunks", chk), 0755)
			if err != nil {
				return
			}
			err = ioutil.WriteFile(filepath.Join("chunks", chk, "Dockerfile"), []byte("ARG base\nFROM ${base}\n\n"), 0755)
			if err != nil {
				return
			}

			err = os.Mkdir("tests", 0755)
			if err != nil && !os.IsExist(err) {
				return
			}
			err = ioutil.WriteFile(fmt.Sprintf("tests/%s.yaml", chk), []byte("- desc: \"it should say hello\"\n  command: [\"echo\", \"hello\"]\n  assert:\n  - status == 0\n  - stdout.indexOf(\"hello\") != -1\n  - stderr.length == 0"), 0755)
			if err != nil {
				return
			}
			return
		}

		err = os.Mkdir("base", 0755)
		if err != nil {
			return
		}
		err = ioutil.WriteFile("base/Dockerfile", []byte("FROM ubuntu:latest\n"), 0755)
		if err != nil {
			return
		}

		err = ioutil.WriteFile("Makefile", []byte(`DAZZLE=dazzle
CACHE_REF=hub.docker.io/some-repo
DEST_REF=hub.docker.io/some-other-repo
BUILDKIT_ADDR=unix:///run/buildkit/buildkitd.sock

.PHONY: chunks combinations

chunks:
	${DAZZLE} build --addr ${BUILDKIT_ADDR} --no-cache ${CACHE_REF} .

combinations:
	${DAZZLE} combine --addr ${BUILDKIT_ADDR} ${DEST_REF}:all ${CACHE_REF} chunk1 chunk2

`), 0755)
		if err != nil {
			return
		}

		fmt.Println("dazzle project initialized - use `dazzle init <chunkname>` to add a chunk")
		return nil
	},
}

func init() {
	projectCmd.AddCommand(projectInitCmd)
}

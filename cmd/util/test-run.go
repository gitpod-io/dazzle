// Copyright © 2020 Gitpod

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

package util

import (
	"context"
	"encoding/xml"
	"io/ioutil"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	"github.com/gitpod-io/dazzle/pkg/fancylog"
	"github.com/gitpod-io/dazzle/pkg/test"
)

var testRunCmd = &cobra.Command{
	Use:   "run <test00.yaml> ... <testN.yaml>",
	Short: "Runs a dazzle test suite",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		log.SetFormatter(&fancylog.Formatter{})

		testFiles := args
		var tests []*test.Spec

		for _, fn := range testFiles {
			fc, err := ioutil.ReadFile(fn)
			if err != nil {
				log.Fatal(err)
			}

			var t []*test.Spec
			err = yaml.Unmarshal(fc, &t)
			if err != nil {
				log.WithField("file", fn).Fatal(err)
			}

			tests = append(tests, t...)
		}

		results, success := test.RunTests(context.Background(), test.LocalExecutor{}, tests)

		xmlout, _ := cmd.Flags().GetString("output-test-xml")
		if xmlout != "" {
			fc, err := xml.MarshalIndent(results, "  ", "    ")
			if err != nil {
				log.Fatal(err)
			}

			err = ioutil.WriteFile(xmlout, fc, 0644)
			if err != nil {
				log.Fatal(err)
			}
		}

		if !success {
			os.Exit(1)
		}

		os.Exit(0)
	},
}

func init() {
	testCmd.AddCommand(testRunCmd)

	testRunCmd.Flags().String("output-test-xml", "", "save result as JUnit XML file")
}

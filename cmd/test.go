// Copyright © 2019 Christian Weichel

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

package cmd

import (
	"context"
	"encoding/xml"
	"io/ioutil"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	"github.com/32leaves/dazzle/pkg/dazzle"
	"github.com/32leaves/dazzle/pkg/fancylog"
	"github.com/32leaves/dazzle/pkg/test"
)

// mergeCmd represents the merge command
var testCmd = &cobra.Command{
	Use:   "test <suite.yaml> <image>",
	Short: "Runs a dazzle test suite",
	Args:  cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		log.SetFormatter(&fancylog.Formatter{})
		env, err := dazzle.NewEnvironment()
		if err != nil {
			log.Fatal(err)
		}

		fc, err := ioutil.ReadFile(args[0])
		if err != nil {
			log.Fatal(err)
		}

		var tests []*test.Spec
		err = yaml.Unmarshal(fc, &tests)
		if err != nil {
			log.Fatal(err)
		}

		results, success := test.RunTests(context.Background(), env.Client, args[1], tests)

		xmlout, _ := cmd.Flags().GetString("output-xml")
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
	rootCmd.AddCommand(testCmd)

	testCmd.Flags().String("output-xml", "", "save result as JUnit XML file")
}

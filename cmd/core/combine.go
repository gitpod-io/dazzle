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
	"context"
	"os"

	"github.com/csweichel/dazzle/pkg/dazzle"
	"github.com/docker/distribution/reference"
	"github.com/moby/buildkit/client"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// combineCmd represents the build command
var combineCmd = &cobra.Command{
	Use:   "combine <dest> <build-ref> <chunks>",
	Short: "Combines previously built chunks into a single image",
	Args:  cobra.MinimumNArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctxdir, _ := cmd.Flags().GetString("context")
		prj, err := dazzle.LoadFromDir(ctxdir)
		if err != nil {
			return err
		}

		destref, err := reference.ParseNamed(args[0])
		if err != nil {
			log.WithError(err).Fatal("cannot parse dest ref")
		}
		buildref, err := reference.ParseNamed(args[1])
		if err != nil {
			log.WithError(err).Fatal("cannot parse build ref")
		}

		var opts []dazzle.CombinerOpt
		notest, _ := cmd.Flags().GetBool("no-test")
		if !notest {
			sckt, _ := cmd.Flags().GetString("addr")
			cl, err := client.New(context.Background(), sckt, client.WithFailFast())
			if err != nil {
				return err
			}
			opts = append(opts, dazzle.WithTests(cl))
		}

		return prj.Combine(context.Background(), args[2:], buildref, destref, getResolver(), opts...)
	},
}

func init() {
	rootCmd.AddCommand(combineCmd)

	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	combineCmd.Flags().String("addr", "unix:///run/buildkit/buildkitd.sock", "address of buildkitd")
	combineCmd.Flags().Bool("no-test", false, "disables the tests")
	combineCmd.Flags().String("context", wd, "context path")
}

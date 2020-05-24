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
	"github.com/moby/buildkit/client"
	"github.com/spf13/cobra"
)

// buildCmd represents the build command
var buildCmd = &cobra.Command{
	Use:   "build <target-ref>",
	Short: "Builds a Docker image with independent layers",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctxdir, _ := cmd.Flags().GetString("context")
		nocache, _ := cmd.Flags().GetBool("no-cache")
		plainOutput, _ := cmd.Flags().GetBool("plain-output")
		cwh, _ := cmd.Flags().GetBool("chunked-without-hash")

		var targetref = args[0]
		prj, err := dazzle.LoadFromDir(ctxdir)
		if err != nil {
			return err
		}

		sckt, _ := cmd.Flags().GetString("addr")
		cl, err := client.New(context.Background(), sckt, client.WithFailFast())
		if err != nil {
			return err
		}

		session, err := dazzle.NewSession(cl, targetref,
			dazzle.WithResolver(getResolver()),
			dazzle.WithNoCache(nocache),
			dazzle.WithPlainOutput(plainOutput),
			dazzle.WithChunkedWithoutHash(cwh),
		)
		if err != nil {
			return err
		}

		return prj.Build(context.Background(), session)
	},
}

func init() {
	rootCmd.AddCommand(buildCmd)

	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	buildCmd.Flags().String("addr", "unix:///run/buildkit/buildkitd.sock", "address of buildkitd")
	buildCmd.Flags().Bool("no-cache", false, "disables the buildkit build cache")
	buildCmd.Flags().String("context", wd, "context path")
	buildCmd.Flags().Bool("plain-output", false, "produce plain output")
	buildCmd.Flags().Bool("chunked-without-hash", false, "disable hash qualification for chunked image")
}

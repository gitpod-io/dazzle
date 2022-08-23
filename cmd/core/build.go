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

package core

import (
	"context"

	"github.com/moby/buildkit/client"
	"github.com/spf13/cobra"

	"github.com/gitpod-io/dazzle/pkg/dazzle"
)

// buildCmd represents the build command
var buildCmd = &cobra.Command{
	Use:   "build <target-ref>",
	Short: "Builds a Docker image with independent layers",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		nocache, _ := cmd.Flags().GetBool("no-cache")
		plainOutput, _ := cmd.Flags().GetBool("plain-output")
		cwh, _ := cmd.Flags().GetBool("chunked-without-hash")

		var targetref = args[0]
		prj, err := dazzle.LoadFromDir(rootCfg.ContextDir, dazzle.LoadFromDirOpts{})
		if err != nil {
			return err
		}

		cl, err := client.New(context.Background(), rootCfg.BuildkitAddr, client.WithFailFast())
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

		err = prj.Build(context.Background(), session)
		if err != nil {
			return err
		}

		session.PrintBuildInfo()

		return nil
	},
}

func init() {
	rootCmd.AddCommand(buildCmd)

	buildCmd.Flags().Bool("no-cache", false, "disables the buildkit build cache")
	buildCmd.Flags().Bool("plain-output", false, "produce plain output")
	buildCmd.Flags().Bool("chunked-without-hash", false, "disable hash qualification for chunked image")
}

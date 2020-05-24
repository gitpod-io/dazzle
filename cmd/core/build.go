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

	"github.com/csweichel/dazzle/pkg/dazzle"
	"github.com/moby/buildkit/client"
	"github.com/spf13/cobra"
)

// buildCmd represents the build command
var buildCmd = &cobra.Command{
	Use:   "build <target-ref> <context>",
	Short: "Builds a Docker image with independent layers",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		nocache, _ := cmd.Flags().GetBool("no-cache")

		var (
			targetref = args[0]
			wd        = args[1]
		)
		prj, err := dazzle.LoadFromDir(wd)
		if err != nil {
			return err
		}

		sckt, _ := cmd.Flags().GetString("addr")
		cl, err := client.New(context.Background(), sckt, client.WithFailFast())
		if err != nil {
			return err
		}
		return prj.Build(context.Background(), cl, targetref, dazzle.WithResolver(getResolver()), dazzle.WithNoCache(nocache))
	},
}

func init() {
	rootCmd.AddCommand(buildCmd)

	buildCmd.Flags().String("addr", "unix:///run/buildkit/buildkitd.sock", "address of buildkitd")

	buildCmd.Flags().Bool("no-cache", false, "disables the buildkit build cache")
}

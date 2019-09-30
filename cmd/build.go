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
	"fmt"
	"log"
	"os"

	"github.com/32leaves/dazzle/pkg/dazzle"
	"github.com/spf13/cobra"
)

// buildCmd represents the build command
var buildCmd = &cobra.Command{
	Use:   "build [context]",
	Short: "Builds a Docker image with independent layers",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var wd string
		if len(args) > 0 {
			wd = args[0]

			if stat, err := os.Stat(wd); os.IsNotExist(err) || !stat.IsDir() {
				return fmt.Errorf("context %s must be a directory", wd)
			}
		} else {
			var err error
			wd, err = os.Getwd()
			if err != nil {
				return err
			}
		}

		dfn, err := cmd.Flags().GetString("file")
		if err != nil {
			return err
		}
		tag, err := cmd.Flags().GetString("tag")
		if err != nil {
			return err
		}
		repo, err := cmd.Flags().GetString("repository")
		if err != nil {
			return err
		}
		pull, err := cmd.Flags().GetBool("pull")
		if err != nil {
			return err
		}
		repoChanged := cmd.Flags().Changed("repository")
		pullChanged := cmd.Flags().Changed("pull")

		const (
			colorRed    = "\x1B[01;91m"
			colorNormal = "\x1B[0m"
		)
		if pull {
			if !repoChanged {
				fmt.Fprintln(os.Stderr, colorRed+"Using --pull without --repository will likely produce bad results!"+colorNormal)
				fmt.Fprintln(os.Stderr)
			}
		} else if repoChanged {
			if !pullChanged {
				fmt.Fprintln(os.Stderr, colorRed+"--repository was set - enabling pull. Use --pull=false to disable auto-pull!"+colorNormal)
				fmt.Fprintln(os.Stderr)
				pull = true
			}
		}
		if !pull {
			fmt.Fprintln(os.Stderr, colorRed+"--pull is disabled - this will lead to unreproducible builds!"+colorNormal)
			fmt.Fprintln(os.Stderr)
		}

		env, err := dazzle.NewEnvironment()
		if err != nil {
			log.Fatal(err)
		}

		cfg := dazzle.BuildConfig{
			Env:            env,
			UseRegistry:    pull,
			BuildImageRepo: repo,
		}

		err = dazzle.Build(cfg, wd, dfn, tag)
		if err != nil {
			log.Fatal(err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(buildCmd)

	buildCmd.Flags().StringP("file", "f", "Dockerfile", "name of the Dockerfile")
	buildCmd.Flags().StringP("tag", "t", "dazzle-built:latest", "tag of the resulting image")
	buildCmd.Flags().StringP("repository", "r", "dazzle-work", "name of the Docker repository to work in (e.g. eu.gcr.io/someprj/dazzle-work)")
	buildCmd.Flags().BoolP("pull", "p", false, "attempt to pull partial images prior to building (enables reproducible image builds)")
}

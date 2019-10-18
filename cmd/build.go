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
	"os"

	"github.com/32leaves/dazzle/pkg/dazzle"
	"github.com/32leaves/dazzle/pkg/fancylog"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// buildCmd represents the build command
var buildCmd = &cobra.Command{
	Use:   "build [context]",
	Short: "Builds a Docker image with independent layers",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		formatter := &fancylog.Formatter{}
		log.SetFormatter(formatter)

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
		repoChanged := cmd.Flags().Changed("repository")
		if !repoChanged {
			log.Warn("Using dazzle without --repository will likely produce incorrect results!")
		}

		env, err := dazzle.NewEnvironment()
		if err != nil {
			log.Fatal(err)
		}
		env.Formatter = formatter

		cfg := dazzle.BuildConfig{
			Env:            env,
			UseRegistry:    true,
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
}

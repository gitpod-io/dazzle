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

package core

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/32leaves/dazzle/pkg/dazzle"
	"github.com/32leaves/dazzle/pkg/fancylog"
	"github.com/32leaves/dazzle/pkg/test"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// buildCmd represents the build command
var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Builds a Docker image with independent layers",
	Args:  cobra.ExactArgs(0),
	RunE: func(cmd *cobra.Command, args []string) error {
		formatter := &fancylog.Formatter{}
		log.SetFormatter(formatter)

		wd, err := os.Getwd()
		if err != nil {
			return err
		}

		chunk, err := cmd.Flags().GetString("chunk")
		if err != nil {
			return err
		}
		all, err := cmd.Flags().GetBool("all")
		if err != nil {
			return err
		}

		var chunks []string
		if chunk != "" && all {
			log.Fatal("cannot use --all and --chunk at the same time")
		} else if chunk != "" {
			chunks = append(chunks, chunk)
		} else if all {
			chunks = findChunks(wd)
		} else {
			log.Fatal("missing either --all or --chunk")
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

		log.WithField("version", version).Debug("this is dazzle")

		cfg := dazzle.BuildConfig{
			Env:            env,
			BuildImageRepo: repo,
			SourceLoc:      wd,
			Chunks:         chunks,
		}

		res, err := dazzle.Build(cfg)
		logBuildResult(res)
		if err != nil {
			log.Fatal(err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(buildCmd)

	buildCmd.Flags().BoolP("all", "a", false, "build all chunks")
	buildCmd.Flags().StringP("chunk", "c", "", "build a specific chunk")
	buildCmd.Flags().StringP("repository", "r", "dazzle-work", "name of the Docker repository to work in (e.g. eu.gcr.io/someprj/dazzle-work)")
}

func logBuildResult(res *dazzle.BuildResult) {
	if res == nil {
		return
	}

	log.Info("build done")
	log.WithField("size", res.BaseImage.Size).Debugf("base layer: %s", res.BaseImage.Ref)
	for _, l := range res.Layers {
		log.WithField("size", l.Size).WithField("ref", l.Ref).Debugf("  layer %s", l.LayerName)
	}
}

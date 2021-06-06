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
	"fmt"
	"strings"

	"github.com/csweichel/dazzle/pkg/dazzle"
	"github.com/docker/distribution/reference"
	"github.com/moby/buildkit/client"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// combineCmd represents the build command
var combineCmd = &cobra.Command{
	Use:   "combine <target-ref>",
	Short: "Combines previously built chunks into a single image",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		prj, err := dazzle.LoadFromDir(rootCfg.ContextDir, dazzle.LoadFromDirOpts{})
		if err != nil {
			return err
		}

		targetref, err := reference.ParseNamed(args[0])
		if err != nil {
			return fmt.Errorf("cannot parse target-ref: %w", err)
		}
		targetref = reference.TrimNamed(targetref)

		var cs []dazzle.ChunkCombination
		if all, _ := cmd.Flags().GetBool("all"); all {
			cs = prj.Config.Combiner.Combinations
		} else if cmbn, _ := cmd.Flags().GetString("combination"); cmbn != "" {
			var found bool
			for _, c := range prj.Config.Combiner.Combinations {
				if c.Name == cmbn {
					found = true
					cs = []dazzle.ChunkCombination{c}
					break
				}
			}
			if !found {
				return fmt.Errorf("combination %s not found", cmbn)
			}
		} else if chunks, _ := cmd.Flags().GetString("chunks"); chunks != "" {
			segs := strings.Split(chunks, "=")
			if len(segs) != 2 {
				return fmt.Errorf("chunks have invalid format")
			}
			cs = []dazzle.ChunkCombination{
				{
					Name:   segs[0],
					Chunks: strings.Split(segs[1], ","),
				},
			}
		} else {
			return fmt.Errorf("must use one of --all, --combination or --chunks")
		}

		bldref, _ := cmd.Flags().GetString("build-ref")
		if bldref == "" {
			bldref = targetref.String()
		}

		cl, err := client.New(context.Background(), rootCfg.BuildkitAddr, client.WithFailFast())
		if err != nil {
			return err
		}

		var opts []dazzle.CombinerOpt
		notest, _ := cmd.Flags().GetBool("no-test")
		if !notest {
			opts = append(opts, dazzle.WithTests(cl))
		}

		sess, err := dazzle.NewSession(cl, bldref, dazzle.WithResolver(getResolver()))
		if err != nil {
			return fmt.Errorf("cannot start build session: %w", err)
		}
		err = sess.DownloadBaseInfo(context.Background(), prj)
		if err != nil {
			return fmt.Errorf("cannot download base-image info: %w", err)
		}

		for _, cmb := range cs {
			destref, err := reference.WithTag(targetref, cmb.Name)
			if err != nil {
				return fmt.Errorf("cannot produce target reference for chunk %s: %w", cmb.Name, err)
			}

			log.WithField("combination", cmb.Name).WithField("chunks", cmb.Chunks).WithField("ref", destref.String()).Warn("producing chunk combination")
			err = prj.Combine(context.Background(), cmb.Chunks, destref, sess, opts...)
			if err != nil {
				return err
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(combineCmd)

	combineCmd.Flags().Bool("no-test", false, "disables the tests")
	combineCmd.Flags().String("chunks", "", "combine a set of chunks - format is name=chk1,chk2,chkN")
	combineCmd.Flags().String("combination", "", "build a specific combination")
	combineCmd.Flags().Bool("all", false, "build all combinations")
	combineCmd.Flags().String("build-ref", "", "use a different build-ref than the target-ref")
}

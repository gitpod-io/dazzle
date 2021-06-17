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

	"github.com/csweichel/dazzle/pkg/dazzle"
	"github.com/docker/distribution/reference"
	"github.com/moby/buildkit/client"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// combineFromRefCmd represents the build command
var combineFromRefCmd = &cobra.Command{
	Use:   "combine-from-ref <target-ref> <chunk-ref1> ... <chunk-refN>",
	Short: "Combines previously built chunks into a single image without a dazzle.yaml file",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		destref, err := reference.ParseNamed(args[0])
		if err != nil {
			return err
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

		sess, err := dazzle.NewSession(cl, args[0], dazzle.WithResolver(getResolver()))
		if err != nil {
			return fmt.Errorf("cannot start build session: %w", err)
		}

		ign, _ := cmd.Flags().GetBool("ignore-differing-base-refs")
		prj, err := dazzle.LoadProjectFromRefs(context.Background(), sess, args[1:], dazzle.LoadProjectFromRefsOpts{IgnoreDifferingBaseRefs: ign})
		if err != nil {
			return err
		}

		err = sess.DownloadBaseInfo(context.Background(), prj)
		if err != nil {
			return fmt.Errorf("cannot download base-image info: %w", err)
		}

		var chks []string
		for _, chk := range prj.Chunks {
			chks = append(chks, chk.Name)
		}

		log.WithField("chunks", args[1:]).WithField("ref", destref.String()).Warn("producing chunk combination")
		err = prj.Combine(context.Background(), chks, destref, sess, opts...)
		if err != nil {
			return err
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(combineFromRefCmd)

	combineFromRefCmd.Flags().Bool("no-test", false, "disables the tests")
	combineFromRefCmd.Flags().Bool("ignore-differing-base-refs", false, "demote differing base images to a warning")
}

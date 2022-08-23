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
	"fmt"

	"github.com/spf13/cobra"

	"github.com/gitpod-io/dazzle/pkg/dazzle"
)

var projectImageNameOpts struct {
	ImageType    string
	ExcludeTests bool
}

var projectImageNameCmd = &cobra.Command{
	Use:   "image-name <target-ref> [chunk]",
	Short: "prints the image-name of a chunk (or all of them)",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		prj, err := dazzle.LoadFromDir(rootCfg.ContextDir, dazzle.LoadFromDirOpts{})
		if err != nil {
			return err
		}

		sess, err := dazzle.NewSession(nil, args[0], dazzle.WithResolver(getResolver()), dazzle.WithNoTests(projectImageNameOpts.ExcludeTests))
		if err != nil {
			return err
		}
		err = sess.DownloadBaseInfo(context.Background(), prj)
		if err != nil {
			return err
		}

		var chunks []dazzle.ProjectChunk
		if len(args[1:]) == 0 {
			chunks = append(prj.Chunks, prj.Base)
		} else {
			for _, c := range args[1:] {
				if c == "base" {
					chunks = append(chunks, prj.Base)
					continue
				}

				var found bool
				for _, cs := range prj.Chunks {
					if cs.Name != c {
						continue
					}

					found = true
					chunks = append(chunks, cs)
				}

				if !found {
					return fmt.Errorf("chunk %s not found", c)
				}
			}
		}

		for _, c := range chunks {
			img, err := c.ImageName(dazzle.ChunkImageType(projectImageNameOpts.ImageType), sess)
			if err != nil {
				return err
			}

			fmt.Printf("%s: %s\n", c.Name, img)
		}

		return nil
	},
}

func init() {
	projectCmd.AddCommand(projectImageNameCmd)
	projectImageNameCmd.Flags().StringVarP(&projectImageNameOpts.ImageType, "type", "t", string(dazzle.ImageTypeChunked), "chunk image type")
	projectImageNameCmd.Flags().BoolVar(&projectImageNameOpts.ExcludeTests, "no-tests", false, "exclude tests")
}

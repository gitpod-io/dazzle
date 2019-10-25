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
	"log"

	"github.com/spf13/cobra"

	"github.com/32leaves/dazzle/pkg/dazzle"
)

// mergeCmd represents the merge command
var mergeCmd = &cobra.Command{
	Use:   "merge <dst> <base> <addons>...",
	Short: "Merges a set of Docker images onto a base image.",
	Long:  `Attempts to merge the layers of all addon images onto the base image producing the new dst image. We assume that all addon images have been built FROM base. All images must be present/pulled to the Docker damon already. All image names must be valid Docker references.`,
	Args:  cobra.MinimumNArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		env, err := dazzle.NewEnvironment()
		if err != nil {
			log.Fatal(err)
		}

		err = dazzle.MergeImages(env, args[0], args[1], args[2:]...)
		if err != nil {
			log.Fatal(err)
		}
	},
}

func init() {
	rootCmd.AddCommand(mergeCmd)
}

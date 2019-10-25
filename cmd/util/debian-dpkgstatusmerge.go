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

package util

import (
	"os"

	"github.com/32leaves/dazzle/pkg/util/debian"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var debianDpkgStatusMergeCmd = &cobra.Command{
	Use:   "dpkg-status-merge <old-status> <new-status>",
	Short: "Updates the old status file and overwrites it with values from the new status file",
	Args:  cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		old, err := debian.LoadDpkgStatus(args[0])
		if err != nil {
			log.WithField("filename", args[0]).Fatal(err)
		}

		new, err := debian.LoadDpkgStatus(args[1])
		if err != nil {
			log.WithField("filename", args[1]).Fatal(err)
		}

		for k, v := range new.Index {
			old.Index[k] = v
		}

		err = debian.SaveDpkgStatus(os.Stdout, old)
		if err != nil {
			log.Fatal(err)
		}
	},
}

func init() {
	debianCmd.AddCommand(debianDpkgStatusMergeCmd)
}

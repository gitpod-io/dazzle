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
	"fmt"
	"io"
	"os"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/gitpod-io/dazzle/pkg/dazzle"
)

const defaultTemplate = `# {{ .Name }}
{{ .Description }}

### References
{{ range $ref := .Ref -}}
  - {{ $ref }}
{{ end}}


### Contents
{{ range $chunk := .LinkedChunks -}}
  - {{ $chunk.Name }}
{{ end}}
`

// docsCmd represents the build command
var docsCmd = &cobra.Command{
	Use:   "docs",
	Short: "Produces readme file for some combinations",
	RunE: func(cmd *cobra.Command, args []string) error {
		prj, err := dazzle.LoadFromDir(rootCfg.ContextDir, dazzle.LoadFromDirOpts{})
		if err != nil {
			return err
		}

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
		} else {
			return fmt.Errorf("must use one of --all or --combination")
		}

		tplContent := defaultTemplate
		if fn, _ := cmd.Flags().GetString("template"); fn != "" {
			fc, err := io.ReadFile(fn)
			if err != nil {
				return fmt.Errorf("error reading template: %v", err)
			}
			tplContent = string(fc)
		}

		tpl, err := template.New("readme").Funcs(sprig.FuncMap()).Parse(tplContent)
		if err != nil {
			return err
		}

		for _, cmb := range cs {
			fn := cmb.Name + ".txt"
			f, err := os.OpenFile(fn, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
			if err != nil {
				return err
			}
			err = tpl.Execute(f, cmb)
			f.Close()
			if err != nil {
				return err
			}

			log.WithField("fn", fn).Info("Wrote docs file")
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(docsCmd)

	docsCmd.Flags().String("combination", "", "build a specific combination")
	docsCmd.Flags().Bool("all", false, "build all combinations")
	docsCmd.Flags().String("template", "", "path to a Go template file for the readme")
}

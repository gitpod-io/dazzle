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

package dazzle

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

func TestMergeEnv(t *testing.T) {
	tests := []struct {
		name   string
		base   *ociv1.Image
		others []*ociv1.Image
		vars   []EnvVarCombination
		expect []string
	}{
		{
			name: "EnvVarCombineMergeUnique",
			base: &ociv1.Image{
				Config: ociv1.ImageConfig{
					Env: []string{
						"PATH=first:second:third:common-value",
					},
				},
			},
			others: []*ociv1.Image{
				{
					Config: ociv1.ImageConfig{
						Env: []string{
							"PATH=fourth:fifth:common-value",
						},
					},
				},
				{
					Config: ociv1.ImageConfig{
						Env: []string{
							"PATH=sixth:sixth:common-value",
						},
					},
				},
				{
					Config: ociv1.ImageConfig{
						Env: []string{
							"PATH=seventh:eighth:seventh:common-value",
						},
					},
				},
			},
			vars: []EnvVarCombination{
				{
					Name:   "PATH",
					Action: EnvVarCombineMergeUnique,
				},
			},
			expect: []string{"PATH=first:second:third:common-value:fourth:fifth:sixth:seventh:eighth"},
		},
		{
			name: "EnvVarCombineMerge",
			base: &ociv1.Image{
				Config: ociv1.ImageConfig{
					Env: []string{
						"PATH=first:second:third:common-value",
					},
				},
			},
			others: []*ociv1.Image{
				{
					Config: ociv1.ImageConfig{
						Env: []string{
							"PATH=fourth:fifth:common-value",
						},
					},
				},
				{
					Config: ociv1.ImageConfig{
						Env: []string{
							"PATH=sixth:sixth:common-value",
						},
					},
				},
				{
					Config: ociv1.ImageConfig{
						Env: []string{
							"PATH=eighth:seventh:eighth:common-value",
						},
					},
				},
			},
			vars: []EnvVarCombination{
				{
					Name:   "PATH",
					Action: EnvVarCombineMerge,
				},
			},
			expect: []string{"PATH=first:second:third:common-value:fourth:fifth:common-value:sixth:sixth:common-value:eighth:seventh:eighth:common-value"},
		},
		{
			name: "EnvVarCombineUseLast",
			base: &ociv1.Image{
				Config: ociv1.ImageConfig{
					Env: []string{
						"PATH=first:second:third:common-value",
					},
				},
			},
			others: []*ociv1.Image{
				{
					Config: ociv1.ImageConfig{
						Env: []string{
							"PATH=fourth:fifth:common-value",
						},
					},
				},
			},
			vars: []EnvVarCombination{
				{
					Name:   "PATH",
					Action: EnvVarCombineUseLast,
				},
			},
			expect: []string{"PATH=fourth:fifth:common-value"},
		},
		{
			name: "EnvVarCombineUseFirst",
			base: &ociv1.Image{
				Config: ociv1.ImageConfig{
					Env: []string{
						"PATH=first:second:third:common-value",
					},
				},
			},
			others: []*ociv1.Image{
				{
					Config: ociv1.ImageConfig{
						Env: []string{
							"PATH=fourth:fifth:common-value",
						},
					},
				},
			},
			vars: []EnvVarCombination{
				{
					Name:   "PATH",
					Action: EnvVarCombineUseFirst,
				},
			},
			expect: []string{"PATH=first:second:third:common-value"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			envs, err := mergeEnv(test.base, test.others, test.vars)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(envs, test.expect); len(diff) != 0 {
				t.Errorf("mergeEnv() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

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

	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

func TestMergeEnv(t *testing.T) {
	type args struct {
	}
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
						"PATH=first:second:third:$PATH",
					},
				},
			},
			others: []*ociv1.Image{
				{
					Config: ociv1.ImageConfig{
						Env: []string{
							"PATH=fourth:fifth:$PATH",
						},
					},
				},
				{
					Config: ociv1.ImageConfig{
						Env: []string{
							"PATH=sixth:sixth:$PATH",
						},
					},
				},
				{
					Config: ociv1.ImageConfig{
						Env: []string{
							"PATH=eighth:seventh:eighth:$PATH",
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
			expect: []string{"PATH=first:second:third:fourth:fifth:sixth:seventh:eighth:$PATH"},
		},
		{
			name: "EnvVarCombineMerge",
			base: &ociv1.Image{
				Config: ociv1.ImageConfig{
					Env: []string{
						"PATH=first:second:third:$PATH",
					},
				},
			},
			others: []*ociv1.Image{
				{
					Config: ociv1.ImageConfig{
						Env: []string{
							"PATH=fourth:fifth:$PATH",
						},
					},
				},
				{
					Config: ociv1.ImageConfig{
						Env: []string{
							"PATH=sixth:sixth:$PATH",
						},
					},
				},
				{
					Config: ociv1.ImageConfig{
						Env: []string{
							"PATH=eighth:seventh:eighth:$PATH",
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
			expect: []string{"PATH=first:second:third:$PATH:fourth:fifth:$PATH:sixth:sixth:$PATH:eighth:seventh:eighth:$PATH"},
		},
		{
			name: "EnvVarCombineUseLast",
			base: &ociv1.Image{
				Config: ociv1.ImageConfig{
					Env: []string{
						"PATH=first:second:third:$PATH",
					},
				},
			},
			others: []*ociv1.Image{
				{
					Config: ociv1.ImageConfig{
						Env: []string{
							"PATH=fourth:fifth:$PATH",
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
			expect: []string{"PATH=fourth:fifth:$PATH"},
		},
		{
			name: "EnvVarCombineUseFirst",
			base: &ociv1.Image{
				Config: ociv1.ImageConfig{
					Env: []string{
						"PATH=first:second:third:$PATH",
					},
				},
			},
			others: []*ociv1.Image{
				{
					Config: ociv1.ImageConfig{
						Env: []string{
							"PATH=fourth:fifth:$PATH",
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
			expect: []string{"PATH=first:second:third:$PATH"},
		},
	}
	for _, test := range tests {
		envs, err := mergeEnv(test.base, test.others, test.vars)
		if err != nil {
			t.Fatal(err)
		}
		if len(envs) != len(test.expect) {
			t.Fatal("unexpected length", len(envs))
		}
		for i, env := range envs {
			if env != test.expect[i] {
				t.Fatal("unexpected env", envs)
			}

		}
	}
}

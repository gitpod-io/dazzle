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
	"context"
	"os"

	"github.com/containerd/containerd/remotes/docker"
	"github.com/csweichel/dazzle/pkg/dazzle"
	"github.com/csweichel/dazzle/pkg/fancylog"
	"github.com/docker/cli/cli/config"
	"github.com/moby/buildkit/client"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// buildCmd represents the build command
var buildCmd = &cobra.Command{
	Use:   "build <target-ref> <context>",
	Short: "Builds a Docker image with independent layers",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		formatter := &fancylog.Formatter{}
		log.SetFormatter(formatter)
		log.SetLevel(log.InfoLevel)
		if v, _ := cmd.Flags().GetBool("verbose"); v {
			log.SetLevel(log.DebugLevel)
		}

		nocache, _ := cmd.Flags().GetBool("no-cache")

		var (
			targetref = args[0]
			wd        = args[1]
		)
		prj, err := dazzle.LoadFromDir(wd)
		if err != nil {
			return err
		}

		dockerCfg := config.LoadDefaultConfigFile(os.Stderr)

		resolver := docker.NewResolver(docker.ResolverOptions{
			Authorizer: docker.NewDockerAuthorizer(docker.WithAuthCreds(func(host string) (user, pwd string, err error) {
				if dockerCfg == nil {
					return
				}

				if host == "registry-1.docker.io" {
					host = "https://index.docker.io/v1/"
				}
				ac, err := dockerCfg.GetAuthConfig(host)
				if err != nil {
					return
				}
				if ac.IdentityToken != "" {
					pwd = ac.IdentityToken
				} else {
					user = ac.Username
					pwd = ac.Password
				}
				log.WithField("host", host).Info("authenticating user")
				return
			})),
		})

		sckt, _ := cmd.Flags().GetString("addr")
		cl, err := client.New(context.Background(), sckt, client.WithFailFast())
		if err != nil {
			return err
		}
		return prj.Build(context.Background(), cl, targetref, dazzle.WithResolver(resolver), dazzle.WithNoCache(nocache))
	},
}

func init() {
	rootCmd.AddCommand(buildCmd)

	buildCmd.Flags().String("addr", "unix:///run/buildkit/buildkitd.sock", "address of buildkitd")
	buildCmd.Flags().BoolP("verbose", "v", false, "enable verbose logging")
	buildCmd.Flags().Bool("no-cache", false, "disables the buildkit build cache")
}

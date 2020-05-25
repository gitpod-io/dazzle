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
	"fmt"
	"os"

	"github.com/containerd/containerd/remotes"
	"github.com/containerd/containerd/remotes/docker"
	"github.com/csweichel/dazzle/pkg/fancylog"
	"github.com/docker/cli/cli/config"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var rootCfg struct {
	Verbose      bool
	ContextDir   string
	BuildkitAddr string
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "dazzle",
	Short: "Dazzle is a very experimental Docker image builder with independent layers",
	Long: `Dazzle breaks your usual Docker build by separating the layers. The idea is that
this way we can avoid needless cache invalidation.

THIS IS AN EXPERIEMENT. THINGS WILL BREAK. BEWARE.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		formatter := &fancylog.Formatter{}
		log.SetFormatter(formatter)
		log.SetLevel(log.InfoLevel)

		if rootCfg.Verbose {
			log.SetLevel(log.DebugLevel)
		}

		return nil
	},
}

func init() {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	rootCmd.PersistentFlags().BoolVarP(&rootCfg.Verbose, "verbose", "v", false, "enable verbose logging")
	rootCmd.PersistentFlags().StringVar(&rootCfg.ContextDir, "context", wd, "context path")
	rootCmd.PersistentFlags().StringVar(&rootCfg.BuildkitAddr, "addr", "unix:///run/buildkit/buildkitd.sock", "address of buildkitd")
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func getResolver() remotes.Resolver {
	dockerCfg := config.LoadDefaultConfigFile(os.Stderr)
	return docker.NewResolver(docker.ResolverOptions{
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
}

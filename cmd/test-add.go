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

package cmd

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/gookit/color"
	"github.com/manifoldco/promptui"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	"github.com/32leaves/dazzle/pkg/dazzle"
	"github.com/32leaves/dazzle/pkg/fancylog"
	"github.com/32leaves/dazzle/pkg/test"
)

var testAddCmd = &cobra.Command{
	Use:   "add <image> <suite.yaml>",
	Short: "Adds to a dazzle test suite",
	Args:  cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		log.SetFormatter(&fancylog.Formatter{})

		imageRef, fn := args[0], args[1]
		fc, err := ioutil.ReadFile(fn)
		if err != nil && !os.IsNotExist(err) {
			log.Fatal(err)
		}

		var tests []*test.Spec
		err = yaml.Unmarshal(fc, &tests)
		if err != nil {
			log.Fatal(err)
		}

		desc, _ := cmd.Flags().GetString("description")
		if desc == "" {
			p := promptui.Prompt{
				Label:    "Description",
				Validate: required,
			}
			desc, err = p.Run()
			if err != nil {
				log.Fatal(err)
			}
		}
		command, _ := cmd.Flags().GetString("command")
		if command == "" {
			p := promptui.Prompt{
				Label: "Test command",
				Validate: func(s string) error {
					if err = required(s); err != nil {
						return err
					}
					if _, err = splitCommand(s); err != nil {
						return err
					}
					return nil
				},
			}
			command, err = p.Run()
			if err != nil {
				log.Fatal(err)
			}
		}
		commandsegs, err := splitCommand(command)
		if err != nil {
			log.Fatal(err)
		}
		user, _ := cmd.Flags().GetString("user")
		envvars, _ := cmd.Flags().GetStringArray("env")
		entrypoint, _ := cmd.Flags().GetString("entrypoint")
		var epsegs []string
		if entrypoint != "" {
			epsegs, err = splitCommand(entrypoint)
			if err != nil {
				log.Fatal(err)
			}
		}

		spec := &test.Spec{
			Desc:       desc,
			Command:    commandsegs,
			User:       user,
			Env:        envvars,
			Entrypoint: epsegs,
			Skip:       false,
		}
		env, err := dazzle.NewEnvironment()
		if err != nil {
			log.Fatal(err)
		}
		tr, err := spec.RunContainer(context.Background(), env.Client, imageRef)
		if err != nil {
			log.Fatal(err)
		}

		err = addAssertions(spec, tr)
		if err != nil {
			log.Fatal(err)
		}

		tests = append(tests, spec)
		fc, err = yaml.Marshal(tests)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(string(fc))

		err = ioutil.WriteFile(args[0], fc, 0644)
		if err != nil {
			log.Fatal(err)
		}
	},
}

func init() {
	testCmd.AddCommand(testAddCmd)

	testAddCmd.Flags().StringP("description", "d", "", "test description")
	testAddCmd.Flags().StringP("command", "c", "", "test command to execute")
	testAddCmd.Flags().StringP("user", "u", "", "user to execute the command as")
	testAddCmd.Flags().StringArrayP("env", "e", []string{}, "set environment variables (VAR=VALUE) for running the test command")
	testAddCmd.Flags().String("entrypoint", "", "container entrypoint")
}

func required(s string) error {
	if len(strings.TrimSpace(s)) == 0 {
		return fmt.Errorf("required")
	}
	return nil
}

func splitCommand(cmd string) ([]string, error) {
	r := csv.NewReader(strings.NewReader(cmd))
	r.Comma = ' '
	return r.Read()
}

func addAssertions(spec *test.Spec, runres *test.RunResult) error {
	// don't let log messages interfere with prompt
	log.SetLevel(log.WarnLevel)

	stdout, _ := json.Marshal(string(runres.Stdout))
	stderr, _ := json.Marshal(string(runres.Stderr))
	fmt.Println("Available variables are:")
	color.Info.Print("stdout: ")
	fmt.Println(string(stdout))
	color.Info.Print("stderr: ")
	fmt.Println(string(stderr))
	color.Info.Print("status: ")
	fmt.Println(runres.StatusCode)

	for {
		p := promptui.Prompt{
			Label:     "Assertion",
			AllowEdit: true,
			Validate: func(a string) error {
				var res test.Result
				err := test.ValidateAssertions(&res, []string{a}, runres)
				if err != nil {
					return err
				}

				if res.Failure != nil {
					return fmt.Errorf(res.Failure.Message)
				}

				return nil
			},
		}
		a, err := p.Run()
		if err != nil {
			return err
		}
		spec.Assertions = append(spec.Assertions, a)

		p = promptui.Prompt{
			Label:     "Add another assertion?",
			IsConfirm: true,
			Default:   "y",
		}
		cont, err := p.Run()
		if err != nil && err != promptui.ErrAbort {
			return err
		}
		if strings.TrimSpace(cont) != "" && strings.ToLower(cont) != "y" {
			break
		}
	}

	return nil
}

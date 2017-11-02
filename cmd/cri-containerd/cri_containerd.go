/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/docker/docker/pkg/reexec"
	"github.com/golang/glog"
	"github.com/opencontainers/selinux/go-selinux"
	"github.com/renstrom/dedent"
	"github.com/spf13/cobra"
	"golang.org/x/net/context"
	"k8s.io/kubernetes/pkg/util/interrupt"

	"github.com/kubernetes-incubator/cri-containerd/cmd/cri-containerd/options"
	api "github.com/kubernetes-incubator/cri-containerd/pkg/api/v1"
	"github.com/kubernetes-incubator/cri-containerd/pkg/client"
	"github.com/kubernetes-incubator/cri-containerd/pkg/server"
	"github.com/kubernetes-incubator/cri-containerd/pkg/version"
)

// Add \u200B to avoid the space trimming.
const desc = "\u200B" + `             _                         __        _                      __
  __________(_)      _________  ____  / /_____ _(_)____  ___  _________/ /
 / ___/ ___/ /______/ ___/ __ \/ __ \/ __/ __ ` + "`" + `/ // __ \/ _ \/ ___/ __  /
/ /__/ /  / //_____/ /__/ /_/ / / / / /_/ /_/ / // / / /  __/ /  / /_/ /
\___/_/  /_/       \___/\____/_/ /_/\__/\__,_/_//_/ /_/\___/_/   \__,_/

A containerd based Kubernetes CRI implementation.
`

var cmd = &cobra.Command{
	Use:   "cri-containerd",
	Short: "A containerd based Kubernetes CRI implementation.",
	Long:  desc,
}

var (
	loadLong = dedent.Dedent(`
		Help for "load TAR" command

		TAR - The path to a tar archive containing a container image.

		Requirement:
			Containerd and cri-containerd daemons (grpc servers) must already be up and
			running before the load command is used.

		Description:
			Running as a client, cri-containerd implements the load command by sending the
			load request to the already running cri-containerd daemon/server, which in
			turn loads the image into containerd's image storage via the already running
			containerd daemon.`)

	loadExample = dedent.Dedent(`
		- use docker to pull the latest busybox image and save it as a tar archive:
			$ docker pull busybox:latest
			$ docker save busybox:latest -o busybox.tar
		- use cri-containerd to load the image into containerd's image storage:
			$ cri-containerd load busybox.tar`)
)

// Add golang flags as persistent flags.
func init() {
	cmd.PersistentFlags().AddGoFlagSet(flag.CommandLine)
}

func defaultConfigCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "default-config",
		Short: "Print default toml config of cri-containerd.",
		Run: func(cmd *cobra.Command, args []string) {
			options.PrintDefaultTomlConfig()
		},
	}
}

func versionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print cri-containerd version information.",
		Run: func(cmd *cobra.Command, args []string) {
			version.PrintVersion()
		},
	}
}

func loadImageCommand() *cobra.Command {
	c := &cobra.Command{
		Use:     "load TAR",
		Long:    loadLong,
		Short:   "Load an image from a tar archive.",
		Args:    cobra.ExactArgs(1),
		Example: loadExample,
	}
	c.SetUsageTemplate(strings.Replace(c.UsageTemplate(), "Examples:", "Example:", 1))
	endpoint, timeout := options.AddGRPCFlags(c.Flags())
	c.RunE = func(cmd *cobra.Command, args []string) error {
		cl, err := client.NewCRIContainerdClient(*endpoint, *timeout)
		if err != nil {
			return fmt.Errorf("failed to create grpc client: %v", err)
		}
		path, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("failed to get absolute path: %v", err)
		}
		res, err := cl.LoadImage(context.Background(), &api.LoadImageRequest{FilePath: path})
		if err != nil {
			return fmt.Errorf("failed to load image: %v", err)
		}
		images := res.GetImages()
		for _, image := range images {
			fmt.Println("Loaded image:", image)
		}
		return nil
	}
	return c
}

func main() {
	if reexec.Init() {
		return
	}
	o := options.NewCRIContainerdOptions()

	o.AddFlags(cmd.Flags())
	cmd.AddCommand(defaultConfigCommand())
	cmd.AddCommand(versionCommand())
	cmd.AddCommand(loadImageCommand())

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		setupDumpStacksTrap()
		if err := o.InitFlags(cmd.Flags()); err != nil {
			return fmt.Errorf("failed to init CRI containerd flags: %v", err)
		}
		validateConfig(o)

		glog.V(0).Infof("Run cri-containerd %+v", o)

		glog.V(2).Infof("Run cri-containerd grpc server on socket %q", o.SocketPath)
		s, err := server.NewCRIContainerdService(o.Config)
		if err != nil {
			return fmt.Errorf("failed to create CRI containerd service: %v", err)
		}
		// Use interrupt handler to make sure the server is stopped properly.
		// Pass in non-empty final function to avoid os.Exit(1). We expect `Run`
		// to return itself.
		h := interrupt.New(func(os.Signal) {}, s.Stop)
		if err := h.Run(func() error { return s.Run() }); err != nil {
			return fmt.Errorf("failed to run cri-containerd grpc server: %v", err)
		}
		return nil
	}

	if err := cmd.Execute(); err != nil {
		// Error should have been reported.
		os.Exit(1)
	}
}

func validateConfig(o *options.CRIContainerdOptions) {
	if o.EnableSelinux {
		if !selinux.GetEnabled() {
			glog.Warning("Selinux is not supported")
		}
	} else {
		selinux.SetDisabled()
	}
}

func setupDumpStacksTrap() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGUSR1)
	go func() {
		for range c {
			dumpStacks()
		}
	}()
}

func dumpStacks() {
	buf := make([]byte, 1024)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	glog.V(0).Infof("=== BEGIN goroutine stack dump ===\n%s\n=== END goroutine stack dump ===", buf)
}

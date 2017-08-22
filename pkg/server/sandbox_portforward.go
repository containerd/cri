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

package server

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/containerd/containerd"
	"github.com/golang/glog"
	"github.com/vishvananda/netns"
	"golang.org/x/net/context"
	"io"
	"k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1/runtime"
	"os/exec"
	goruntime "runtime"
)

// PortForward prepares a streaming endpoint to forward ports from a PodSandbox, and returns the address.
func (c *criContainerdService) PortForward(ctx context.Context, r *runtime.PortForwardRequest) (retRes *runtime.PortForwardResponse, retErr error) {
	// TODO(random-liu): Run a socat container inside the sandbox to do portforward.
	glog.V(2).Infof("Portforward for sandbox %q port %v", r.GetPodSandboxId(), r.GetPort())
	defer func() {
		if retErr == nil {
			glog.V(2).Infof("Portforward for %q returns URL %q", r.GetPodSandboxId(), retRes.GetUrl())
		}
	}()

	sandbox, err := c.sandboxStore.Get(r.GetPodSandboxId())
	if err != nil {
		return nil, fmt.Errorf("failed to find sandbox: %v", err)
	}

	t, err := sandbox.Container.Task(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get sandbox container: %v", err)
	}
	status, err := t.Status(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get sandbox container status: %v", err)
	}
	if status.Status != containerd.Running {
		return nil, errors.New("sandbox container is not running")
	}
	// TODO(random-liu): Verify that ports are exposed.
	return c.streamServer.GetPortForward(r)
}

// portForward requires `socat` on the node, it uses `socat` inside the
// namespace to forward stream for a specific port. The `socat` command
// keeps running until it exits or client disconnect.
func (c *criContainerdService) portForward(id string, port int32, stream io.ReadWriteCloser) error {
	s, err := c.sandboxStore.Get(id)
	if err != nil {
		return fmt.Errorf("failed to find sandbox in store: %v", err)
	}
	pid := s.Pid

	socat, err := exec.LookPath("socat")
	if err != nil {
		return fmt.Errorf("failed to find socat: %v", err)
	}

	// Check following links for meaning of the options:
	// * socat: https://linux.die.net/man/1/socat
	args := []string{"-", fmt.Sprintf("TCP4:localhost:%d", port)}

	goruntime.LockOSThread()
	defer goruntime.UnlockOSThread()

	// Save the current namespace
	origNS, err := netns.Get()
	if err != nil {
		return fmt.Errorf("failed to fetch current network namespace %v", err)
	}
	defer origNS.Close()
	// Get the namespace from the pid
	pidNS, err := netns.GetFromPid(int(pid))
	if err != nil {
		return fmt.Errorf("failed to fetch pid network namespace %v", err)
	}
	// Set namespace of the pid
	netns.Set(pidNS)
	defer pidNS.Close()

	cmd := exec.Command(socat, args...)
	cmd.Stdout = stream

	stderr := new(bytes.Buffer)
	cmd.Stderr = stderr

	// If we use Stdin, command.Run() won't return until the goroutine that's copying
	// from stream finishes. Unfortunately, if you have a client like telnet connected
	// via port forwarding, as long as the user's telnet client is connected to the user's
	// local listener that port forwarding sets up, the telnet session never exits. This
	// means that even if socat has finished running, command.Run() won't ever return
	// (because the client still has the connection and stream open).
	//
	// The work around is to use StdinPipe(), as Wait() (called by Run()) closes the pipe
	// when the command (socat) exits.
	in, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %v", err)
	}
	go func() {
		if _, err := io.Copy(in, stream); err != nil {
			glog.Errorf("Failed to copy port forward input for %q port %d: %v", id, port, err)
		}
		in.Close()
		glog.V(4).Infof("Finish copy port forward input for %q port %d: %v", id, port)
	}()

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("socat command returns error: %v, stderr: %q", err, stderr.String())
	}

	netns.Set(origNS)
	glog.V(2).Infof("Finish port forwarding for %q port %d", id, port)

	return nil
}

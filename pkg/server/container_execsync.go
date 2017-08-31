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
	"fmt"
	"io"
	"io/ioutil"
	"os/exec"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/errdefs"
	"github.com/golang/glog"
	"golang.org/x/net/context"
	"golang.org/x/sys/unix"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1/runtime"

	"github.com/kubernetes-incubator/cri-containerd/pkg/util"
)

// ExecSync executes a command in the container, and returns the stdout output.
// If command exits with a non-zero exit code, an error is returned.
func (c *criContainerdService) ExecSync(ctx context.Context, r *runtime.ExecSyncRequest) (*runtime.ExecSyncResponse, error) {
	output, _ := exec.Command("sudo", "iptables", "-L").CombinedOutput()
	glog.V(2).Infof("sudo iptables -L: %s", output)
	output, _ = exec.Command("sudo", "ifconfig").CombinedOutput()
	glog.V(2).Infof("sudo ifconfig: %s", output)
	output, _ = exec.Command("sudo", "ip", "addr").CombinedOutput()
	glog.V(2).Infof("sudo ip addr: %s", output)

	var stdout, stderr bytes.Buffer
	exitCode, err := c.execInContainer(ctx, r.GetContainerId(), execOptions{
		cmd:     r.GetCmd(),
		stdout:  &stdout,
		stderr:  &stderr,
		timeout: time.Duration(r.GetTimeout()) * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to exec in container: %v", err)
	}

	return &runtime.ExecSyncResponse{
		Stdout:   stdout.Bytes(),
		Stderr:   stderr.Bytes(),
		ExitCode: int32(*exitCode),
	}, nil
}

// execOptions specifies how to execute command in container.
type execOptions struct {
	cmd     []string
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
	tty     bool
	resize  <-chan remotecommand.TerminalSize
	timeout time.Duration
}

// execInContainer executes a command inside the container synchronously, and
// redirects stdio stream properly.
func (c *criContainerdService) execInContainer(ctx context.Context, id string, opts execOptions) (*uint32, error) {
	// Cancel the context before returning to ensure goroutines are stopped.
	// This is important, because if `Start` returns error, `Wait` will hang
	// forever unless we cancel the context.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Get container from our container store.
	cntr, err := c.containerStore.Get(id)
	if err != nil {
		return nil, fmt.Errorf("failed to find container in store: %v", err)
	}
	id = cntr.ID

	state := cntr.Status.Get().State()
	if state != runtime.ContainerState_CONTAINER_RUNNING {
		return nil, fmt.Errorf("container is in %s state", criContainerStateToString(state))
	}

	container := cntr.Container
	spec, err := container.Spec()
	if err != nil {
		return nil, fmt.Errorf("failed to get container spec: %v", err)
	}
	task, err := container.Task(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to load task: %v", err)
	}
	pspec := spec.Process
	pspec.Args = opts.cmd
	pspec.Terminal = opts.tty

	if opts.stdin == nil {
		opts.stdin = new(bytes.Buffer)
	}
	if opts.stdout == nil {
		opts.stdout = ioutil.Discard
	}
	if opts.stderr == nil {
		opts.stderr = ioutil.Discard
	}
	execID := util.GenerateID()
	process, err := task.Exec(ctx, execID, pspec, containerd.NewIOWithTerminal(
		opts.stdin,
		opts.stdout,
		opts.stderr,
		opts.tty,
	))
	if err != nil {
		return nil, fmt.Errorf("failed to create exec %q: %v", execID, err)
	}
	defer func() {
		if _, err := process.Delete(ctx); err != nil {
			glog.Errorf("Failed to delete exec process %q for container %q: %v", execID, id, err)
		}
	}()

	handleResizing(opts.resize, func(size remotecommand.TerminalSize) {
		if err := process.Resize(ctx, uint32(size.Width), uint32(size.Height)); err != nil {
			glog.Errorf("Failed to resize process %q console for container %q: %v", execID, id, err)
		}
	})

	exitCh, err := process.Wait(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to wait for process %q: %v", execID, err)
	}
	if err := process.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start exec %q: %v", execID, err)
	}

	var timeoutCh <-chan time.Time
	if opts.timeout == 0 {
		// Do not set timeout if it's 0.
		timeoutCh = make(chan time.Time)
	} else {
		timeoutCh = time.After(opts.timeout)
	}
	select {
	case <-timeoutCh:
		//TODO(Abhi) Use context.WithDeadline instead of timeout.
		// Ignore the not found error because the process may exit itself before killing.
		if err := process.Kill(ctx, unix.SIGKILL); err != nil && !errdefs.IsNotFound(err) {
			return nil, fmt.Errorf("failed to kill exec %q: %v", execID, err)
		}
		// Wait for the process to be killed.
		exitRes := <-exitCh
		glog.V(2).Infof("Timeout received while waiting for exec process kill %q code %d and error %v",
			execID, exitRes.ExitCode(), exitRes.Error())
		return nil, fmt.Errorf("timeout %v exceeded", opts.timeout)
	case exitRes := <-exitCh:
		code, _, err := exitRes.Result()
		glog.V(2).Infof("Exec process %q exits with exit code %d and error %v", execID, code, err)
		if err != nil {
			return nil, fmt.Errorf("failed while waiting for exec %q: %v", execID, err)
		}
		return &code, nil
	}
}

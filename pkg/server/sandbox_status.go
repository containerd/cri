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
	"fmt"

	"github.com/golang/glog"
	"golang.org/x/net/context"

	"github.com/containerd/containerd/api/services/execution"
	"github.com/containerd/containerd/api/types/task"

	runtime "k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1"

	"github.com/kubernetes-incubator/cri-containerd/pkg/metadata"
)

// PodSandboxStatus returns the status of the PodSandbox.
func (c *criContainerdService) PodSandboxStatus(ctx context.Context, r *runtime.PodSandboxStatusRequest) (retRes *runtime.PodSandboxStatusResponse, retErr error) {
	glog.V(4).Infof("PodSandboxStatus for sandbox %q", r.GetPodSandboxId())
	defer func() {
		if retErr == nil {
			glog.V(4).Infof("PodSandboxStatus for %q returns status %+v", r.GetPodSandboxId(), retRes.GetStatus())
		}
	}()

	sandbox, err := c.getSandbox(r.GetPodSandboxId())
	if err != nil {
		return nil, fmt.Errorf("an error occurred when try to find sandbox %q: %v",
			r.GetPodSandboxId(), err)
	}
	// Use the full sandbox id.
	id := sandbox.ID

	info, err := c.containerService.Info(ctx, &execution.InfoRequest{ContainerID: id})
	if err != nil && !isContainerdContainerNotExistError(err) {
		return nil, fmt.Errorf("failed to get sandbox container info for %q: %v", id, err)
	}

	// Set sandbox state to NOTREADY by default.
	state := runtime.PodSandboxState_SANDBOX_NOTREADY
	// If the sandbox container is running, treat it as READY.
	if info != nil && info.Task.Status == task.StatusRunning {
		state = runtime.PodSandboxState_SANDBOX_READY
	}

	ip, err := c.netPlugin.GetContainerNetworkStatus(sandbox.NetNS, sandbox.Config.GetMetadata().GetNamespace(), sandbox.Config.GetMetadata().GetName(), id)
	if err != nil {
		// Ignore the error on network status
		ip = ""
		glog.V(4).Infof("GetContainerNetworkStatus returns error: %v", err)
	}

	return &runtime.PodSandboxStatusResponse{Status: toCRISandboxStatus(sandbox, state, ip)}, nil
}

// toCRISandboxStatus converts sandbox metadata into CRI pod sandbox status.
func toCRISandboxStatus(meta *metadata.SandboxMetadata, state runtime.PodSandboxState, ip string) *runtime.PodSandboxStatus {
	nsOpts := meta.Config.GetLinux().GetSecurityContext().GetNamespaceOptions()
	return &runtime.PodSandboxStatus{
		Id:        meta.ID,
		Metadata:  meta.Config.GetMetadata(),
		State:     state,
		CreatedAt: meta.CreatedAt,
		Network:   &runtime.PodSandboxNetworkStatus{Ip: ip},
		Linux: &runtime.LinuxPodSandboxStatus{
			Namespaces: &runtime.Namespace{
				Options: &runtime.NamespaceOption{
					HostNetwork: nsOpts.GetHostNetwork(),
					HostPid:     nsOpts.GetHostPid(),
					HostIpc:     nsOpts.GetHostIpc(),
				},
			},
		},
		Labels:      meta.Config.GetLabels(),
		Annotations: meta.Config.GetAnnotations(),
	}
}

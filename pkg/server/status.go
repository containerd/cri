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
	"encoding/json"
	"fmt"
	goruntime "runtime"

	"golang.org/x/net/context"
	runtime "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
)

const (
	// runtimeNotReadyReason is the reason reported when runtime is not ready.
	runtimeNotReadyReason = "ContainerdNotReady"
	// networkNotReadyReason is the reason reported when network is not ready.
	networkNotReadyReason = "NetworkPluginNotReady"
)

// Status returns the status of the runtime.
func (c *criContainerdService) Status(ctx context.Context, r *runtime.StatusRequest) (*runtime.StatusResponse, error) {
	// As a containerd plugin, if CRI plugin is serving request,
	// containerd must be ready.
	runtimeCondition := &runtime.RuntimeCondition{
		Type:   runtime.RuntimeReady,
		Status: true,
	}
	networkCondition := &runtime.RuntimeCondition{
		Type:   runtime.NetworkReady,
		Status: true,
	}
	if err := c.netPlugin.Status(); err != nil {
		networkCondition.Status = false
		networkCondition.Reason = networkNotReadyReason
		networkCondition.Message = fmt.Sprintf("Network plugin returns error: %v", err)
	}

	resp := &runtime.StatusResponse{
		Status: &runtime.RuntimeStatus{Conditions: []*runtime.RuntimeCondition{
			runtimeCondition,
			networkCondition,
		}},
	}
	if r.Verbose {
		configByt, err := json.Marshal(c.config)
		if err != nil {
			return nil, err
		}
		resp.Info = make(map[string]string)
		resp.Info["config"] = string(configByt)
		versionByt, err := json.Marshal(goruntime.Version())
		if err != nil {
			return nil, err
		}
		resp.Info["golang"] = string(versionByt)
	}
	return resp, nil
}

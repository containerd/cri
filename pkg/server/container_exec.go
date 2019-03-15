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
	"net/http"
	"sync"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	runtime "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
)

var (
	mu                           sync.Mutex
	advertiseStreamServerRunning bool
)

// Exec prepares a streaming endpoint to execute a command in the container, and returns the address.
func (c *criService) Exec(ctx context.Context, r *runtime.ExecRequest) (*runtime.ExecResponse, error) {
	cntr, err := c.containerStore.Get(r.GetContainerId())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to find container %q in store", r.GetContainerId())
	}
	state := cntr.Status.Get().State()
	if state != runtime.ContainerState_CONTAINER_RUNNING {
		return nil, errors.Errorf("container is in %s state", criContainerStateToString(state))
	}

	if c.config.StreamServerAddress != c.config.AdvertiseStreamServerAddress {
		if !c.isAdvertiseStreamServerRunning() {
			// Use channel to make sure the first exec will be successful
			advertiseStreamServerCh := make(chan struct{})
			go func() {
				c.setAdvertiseStreamServerRunning(true)
				defer c.setAdvertiseStreamServerRunning(false)
				close(advertiseStreamServerCh)
				if err := c.advertiseStreamServer.Start(true); err != nil && err != http.ErrServerClosed {
					logrus.WithError(err).Error("Failed to start advertise streaming server")
				}
			}()

			<-advertiseStreamServerCh
		}
		return c.advertiseStreamServer.GetExec(r)
	}

	return c.streamServer.GetExec(r)
}

func (c *criService) isAdvertiseStreamServerRunning() bool {
	mu.Lock()
	defer mu.Unlock()

	return advertiseStreamServerRunning
}

func (c *criService) setAdvertiseStreamServerRunning(v bool) {
	mu.Lock()
	defer mu.Unlock()

	advertiseStreamServerRunning = v
}

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
	"time"

	"github.com/containerd/containerd/api/services/events/v1"
	"github.com/containerd/containerd/api/services/tasks/v1"
	"github.com/containerd/containerd/typeurl"
	"github.com/golang/glog"
	"github.com/jpillora/backoff"
	"golang.org/x/net/context"

	"github.com/kubernetes-incubator/cri-containerd/pkg/metadata"
)

const (
	// minRetryInterval is the minimum retry interval when lost connection with containerd.
	minRetryInterval = 100 * time.Millisecond
	// maxRetryInterval is the maximum retry interval when lost connection with containerd.
	maxRetryInterval = 30 * time.Second
	// exponentialFactor is the exponential backoff factor.
	exponentialFactor = 2.0
)

// startEventMonitor starts an event monitor which monitors and handles all
// container events.
// TODO(random-liu): [P1] Is it possible to drop event during containerd is running?
func (c *criContainerdService) startEventMonitor() {
	b := backoff.Backoff{
		Min:    minRetryInterval,
		Max:    maxRetryInterval,
		Factor: exponentialFactor,
	}
	go func() {
		for {
			evnts, err := c.eventService.Stream(context.Background(), &events.StreamEventsRequest{})
			if err != nil {
				glog.Errorf("Failed to connect to containerd event stream: %v", err)
				time.Sleep(b.Duration())
				continue
			}
			// Successfully connect with containerd, reset backoff.
			b.Reset()
			// TODO(random-liu): Relist to recover state, should prevent other operations
			// until state is fully recovered.
			for {
				if err := c.handleEventStream(evnts); err != nil {
					glog.Errorf("Failed to handle event stream: %v", err)
					break
				}
			}
		}
	}()
}

// handleEventStream receives an event from containerd and handles the event.
func (c *criContainerdService) handleEventStream(evnts events.Events_StreamClient) error {
	e, err := evnts.Recv()
	if err != nil {
		return err
	}
	glog.V(2).Infof("Received container event: %+v", e)
	c.handleEvent(e)
	return nil
}

// handleEvent handles a containerd event.
func (c *criContainerdService) handleEvent(e *events.Envelope) {
	any, err := typeurl.UnmarshalAny(e.Event)
	if err == nil {
		switch v := any.(type) {
		// If containerd-shim exits unexpectedly, there will be no corresponding event.
		// However, containerd could not retrieve container state in that case, so it's
		// fine to leave out that case for now.
		// TODO(random-liu): [P2] Handle containerd-shim exit.
		case events.TaskExit:
			meta, err := c.containerStore.Get(v.ID)
			if err != nil {
				glog.Errorf("Failed to get container %q metadata: %v", v.ID, err)
				return
			}
			if v.Pid != meta.Pid {
				// Non-init process died, ignore the event.
				return
			}
			// Delete the container from containerd.
			_, err = c.taskService.Delete(context.Background(), &tasks.DeleteTaskRequest{ContainerID: v.ID})
			if err != nil && !isContainerdGRPCNotFoundError(err) {
				// TODO(random-liu): [P0] Enqueue the event and retry.
				glog.Errorf("Failed to delete container %q: %v", v.ID, err)
				return
			}
			err = c.containerStore.Update(v.ID, func(meta metadata.ContainerMetadata) (metadata.ContainerMetadata, error) {
				// If FinishedAt has been set (e.g. with start failure), keep as
				// it is.
				if meta.FinishedAt != 0 {
					return meta, nil
				}
				meta.Pid = 0
				meta.FinishedAt = v.ExitedAt.UnixNano()
				meta.ExitCode = int32(v.ExitStatus)
				return meta, nil
			})
			if err != nil {
				glog.Errorf("Failed to update container %q state: %v", v.ID, err)
				// TODO(random-liu): [P0] Enqueue the event and retry.
				return
			}
		case events.TaskOOM:
			err := c.containerStore.Update(v.ContainerID, func(meta metadata.ContainerMetadata) (metadata.ContainerMetadata, error) {
				meta.Reason = oomExitReason
				return meta, nil
			})
			if err != nil && !metadata.IsNotExistError(err) {
				glog.Errorf("Failed to update container %q oom: %v", v.ContainerID, err)
				return
			}
		}
	}
}

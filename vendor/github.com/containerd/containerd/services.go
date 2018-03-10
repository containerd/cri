/*
   Copyright The containerd Authors.

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

package containerd

import (
	namespacesapi "github.com/containerd/containerd/api/services/namespaces/v1"
	"github.com/containerd/containerd/api/services/tasks/v1"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/snapshots"
)

type services struct {
	contentStore   content.Store
	imageStore     images.Store
	containerStore containers.Store
	namespaceStore namespaces.Store
	snapshotters   map[string]snapshots.Snapshotter
	taskService    tasks.TasksClient
	eventService   EventService
}

// ServicesOpt allows callers to set options on the services
type ServicesOpt func(c *services)

// WithContentStore sets the content store.
func WithContentStore(contentStore content.Store) ServicesOpt {
	return func(s *services) {
		s.contentStore = contentStore
	}
}

// WithImageStore sets the image store.
func WithImageStore(imageStore images.Store) ServicesOpt {
	return func(s *services) {
		s.imageStore = imageStore
	}
}

// WithSnapshotters sets the snapshotters.
func WithSnapshotters(snapshotters map[string]snapshots.Snapshotter) ServicesOpt {
	return func(s *services) {
		s.snapshotters = make(map[string]snapshots.Snapshotter)
		for n, sn := range snapshotters {
			s.snapshotters[n] = sn
		}
	}
}

// WithContainerStore sets the container store.
func WithContainerStore(containerStore containers.Store) ServicesOpt {
	return func(s *services) {
		s.containerStore = containerStore
	}
}

// WithTaskService sets the task service.
func WithTaskService(taskService tasks.TasksClient) ServicesOpt {
	return func(s *services) {
		s.taskService = taskService
	}
}

// WithEventService sets the event service.
func WithEventService(eventService EventService) ServicesOpt {
	return func(s *services) {
		s.eventService = eventService
	}
}

// WithNamespaceService sets the namespace service.
func WithNamespaceService(namespaceService namespacesapi.NamespacesClient) ServicesOpt {
	return func(s *services) {
		s.namespaceStore = NewNamespaceStoreFromClient(namespaceService)
	}
}

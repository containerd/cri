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
	"time"

	containersapi "github.com/containerd/containerd/api/services/containers/v1"
	diffapi "github.com/containerd/containerd/api/services/diff/v1"
	eventsapi "github.com/containerd/containerd/api/services/events/v1"
	imagesapi "github.com/containerd/containerd/api/services/images/v1"
	introspectionapi "github.com/containerd/containerd/api/services/introspection/v1"
	leasesapi "github.com/containerd/containerd/api/services/leases/v1"
	namespacesapi "github.com/containerd/containerd/api/services/namespaces/v1"
	snapshotsapi "github.com/containerd/containerd/api/services/snapshots/v1"
	"github.com/containerd/containerd/api/services/tasks/v1"
	versionservice "github.com/containerd/containerd/api/services/version/v1"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/dialer"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/snapshots"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// localServices is an implementation of Services through local function call.
type localServices struct {
	conn         *grpc.ClientConn
	connector    func() (*grpc.ClientConn, error)
	contentStore content.Store
}

// NewLocalServices returns a new services using the passed-in
// local services.
// Users of local services need to handle namespace themselves.
// TODO(random-liu): Change all services to local services and remove
// the grpc connection.
// TODO(random-liu): Design the arg list better, probably
// need some option functions. (containerd/containerd#2183)
func NewLocalServices(address, ns string, contentStore content.Store) (Services, error) {
	gopts := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithInsecure(),
		grpc.WithTimeout(60 * time.Second),
		grpc.FailOnNonTempDialError(true),
		grpc.WithBackoffMaxDelay(3 * time.Second),
		grpc.WithDialer(dialer.Dialer),
	}
	if ns != "" {
		unary, stream := newNSInterceptors(ns)
		gopts = append(gopts,
			grpc.WithUnaryInterceptor(unary),
			grpc.WithStreamInterceptor(stream),
		)
	}
	connector := func() (*grpc.ClientConn, error) {
		conn, err := grpc.Dial(dialer.DialAddress(address), gopts...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to dial %q", address)
		}
		return conn, nil
	}
	conn, err := connector()
	if err != nil {
		return nil, err
	}
	return &localServices{
		conn:         conn,
		connector:    connector,
		contentStore: contentStore,
	}, nil
}

// Reconnect re-establishes the GRPC connection to the containerd daemon
func (l *localServices) Reconnect() error {
	if l.connector == nil {
		return errors.New("unable to reconnect to containerd, no connector available")
	}
	l.conn.Close()
	conn, err := l.connector()
	if err != nil {
		return err
	}
	l.conn = conn
	return nil
}

// Close closes the clients connection to containerd
func (l *localServices) Close() error {
	return l.conn.Close()
}

// NamespaceService returns the underlying Namespaces Store
func (l *localServices) NamespaceService() namespaces.Store {
	return NewNamespaceStoreFromClient(namespacesapi.NewNamespacesClient(l.conn))
}

// ContainerService returns the underlying container Store
func (l *localServices) ContainerService() containers.Store {
	return NewRemoteContainerStore(containersapi.NewContainersClient(l.conn))
}

// ContentStore returns the underlying content Store
func (l *localServices) ContentStore() content.Store {
	return l.contentStore
}

// SnapshotService returns the underlying snapshotter for the provided snapshotter name
func (l *localServices) SnapshotService(snapshotterName string) snapshots.Snapshotter {
	return NewSnapshotterFromClient(snapshotsapi.NewSnapshotsClient(l.conn), snapshotterName)
}

// TaskService returns the underlying TasksClient
func (l *localServices) TaskService() tasks.TasksClient {
	return tasks.NewTasksClient(l.conn)
}

// ImageService returns the underlying image Store
func (l *localServices) ImageService() images.Store {
	return NewImageStoreFromClient(imagesapi.NewImagesClient(l.conn))
}

// DiffService returns the underlying Differ
func (l *localServices) DiffService() DiffService {
	return NewDiffServiceFromClient(diffapi.NewDiffClient(l.conn))
}

// IntrospectionService returns the underlying Introspection Client
func (l *localServices) IntrospectionService() introspectionapi.IntrospectionClient {
	return introspectionapi.NewIntrospectionClient(l.conn)
}

// LeasesService returns the underlying Leases Client
func (l *localServices) LeasesService() leasesapi.LeasesClient {
	return leasesapi.NewLeasesClient(l.conn)
}

// HealthService returns the underlying GRPC HealthClient
func (l *localServices) HealthService() grpc_health_v1.HealthClient {
	return grpc_health_v1.NewHealthClient(l.conn)
}

// EventService returns the underlying EventsClient
func (l *localServices) EventService() eventsapi.EventsClient {
	return eventsapi.NewEventsClient(l.conn)
}

// VersionService returns the underlying VersionClient
func (l *localServices) VersionService() versionservice.VersionClient {
	return versionservice.NewVersionClient(l.conn)
}

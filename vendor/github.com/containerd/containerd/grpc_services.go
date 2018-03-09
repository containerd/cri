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
	contentapi "github.com/containerd/containerd/api/services/content/v1"
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

// grpcServices is an implementation of Services through grpc connection
// with containerd grpc server.
type grpcServices struct {
	conn      *grpc.ClientConn
	connector func() (*grpc.ClientConn, error)
}

func newGRPCServices(address string, copts clientOpts) (Services, error) {
	gopts := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithInsecure(),
		grpc.WithTimeout(60 * time.Second),
		grpc.FailOnNonTempDialError(true),
		grpc.WithBackoffMaxDelay(3 * time.Second),
		grpc.WithDialer(dialer.Dialer),
	}
	if len(copts.dialOptions) > 0 {
		gopts = copts.dialOptions
	}
	// TODO(random-liu): Figure out whether we still need this.
	// (containerd/containerd#2183)
	if copts.defaultns != "" {
		unary, stream := newNSInterceptors(copts.defaultns)
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
	return &grpcServices{
		conn:      conn,
		connector: connector,
	}, nil
}

func newGRPCServicesWithConn(conn *grpc.ClientConn) (Services, error) {
	return &grpcServices{
		conn: conn,
	}, nil
}

// Reconnect re-establishes the GRPC connection to the containerd daemon
func (g *grpcServices) Reconnect() error {
	if g.connector == nil {
		return errors.New("unable to reconnect to containerd, no connector available")
	}
	g.conn.Close()
	conn, err := g.connector()
	if err != nil {
		return err
	}
	g.conn = conn
	return nil
}

// Close closes the clients connection to containerd
func (g *grpcServices) Close() error {
	return g.conn.Close()
}

// NamespaceService returns the underlying Namespaces Store
func (g *grpcServices) NamespaceService() namespaces.Store {
	return NewNamespaceStoreFromClient(namespacesapi.NewNamespacesClient(g.conn))
}

// ContainerService returns the underlying container Store
func (g *grpcServices) ContainerService() containers.Store {
	return NewRemoteContainerStore(containersapi.NewContainersClient(g.conn))
}

// ContentStore returns the underlying content Store
func (g *grpcServices) ContentStore() content.Store {
	return NewContentStoreFromClient(contentapi.NewContentClient(g.conn))
}

// SnapshotService returns the underlying snapshotter for the provided snapshotter name
func (g *grpcServices) SnapshotService(snapshotterName string) snapshots.Snapshotter {
	return NewSnapshotterFromClient(snapshotsapi.NewSnapshotsClient(g.conn), snapshotterName)
}

// TaskService returns the underlying TasksClient
func (g *grpcServices) TaskService() tasks.TasksClient {
	return tasks.NewTasksClient(g.conn)
}

// ImageService returns the underlying image Store
func (g *grpcServices) ImageService() images.Store {
	return NewImageStoreFromClient(imagesapi.NewImagesClient(g.conn))
}

// DiffService returns the underlying Differ
func (g *grpcServices) DiffService() DiffService {
	return NewDiffServiceFromClient(diffapi.NewDiffClient(g.conn))
}

// IntrospectionService returns the underlying Introspection Client
func (g *grpcServices) IntrospectionService() introspectionapi.IntrospectionClient {
	return introspectionapi.NewIntrospectionClient(g.conn)
}

// LeasesService returns the underlying Leases Client
func (g *grpcServices) LeasesService() leasesapi.LeasesClient {
	return leasesapi.NewLeasesClient(g.conn)
}

// HealthService returns the underlying GRPC HealthClient
func (g *grpcServices) HealthService() grpc_health_v1.HealthClient {
	return grpc_health_v1.NewHealthClient(g.conn)
}

// EventService returns the underlying event service
func (g *grpcServices) EventService() EventService {
	return NewEventServiceFromClient(eventsapi.NewEventsClient(g.conn))
}

// VersionService returns the underlying VersionClient
func (g *grpcServices) VersionService() versionservice.VersionClient {
	return versionservice.NewVersionClient(g.conn)
}

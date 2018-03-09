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

package containers

import (
	api "github.com/containerd/containerd/api/services/containers/v1"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/plugin"
	"github.com/containerd/containerd/services"
	ptypes "github.com/gogo/protobuf/types"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func init() {
	plugin.Register(&plugin.Registration{
		Type: plugin.GRPCPlugin,
		ID:   "containers",
		Requires: []plugin.Type{
			plugin.ServicePlugin,
		},
		InitFn: func(ic *plugin.InitContext) (interface{}, error) {
			plugins, err := ic.GetByType(plugin.ServicePlugin)
			if err != nil {
				return nil, err
			}
			p, ok := plugins[services.ContainersService]
			if !ok {
				return nil, errors.New("containers service not found")
			}
			cs, err := p.Instance()
			if err != nil {
				return nil, err
			}
			return newService(cs.(containers.Store)), nil
		},
	})
}

type service struct {
	store containers.Store
}

// newService returns the container GRPC server
func newService(store containers.Store) api.ContainersServer {
	return &service{store: store}
}

func (s *service) Register(server *grpc.Server) error {
	api.RegisterContainersServer(server, s)
	return nil
}

func (s *service) Get(ctx context.Context, req *api.GetContainerRequest) (*api.GetContainerResponse, error) {
	container, err := s.store.Get(ctx, req.ID)
	if err != nil {
		return nil, errdefs.ToGRPC(err)
	}

	return &api.GetContainerResponse{
		Container: containerToProto(&container),
	}, nil
}

func (s *service) List(ctx context.Context, req *api.ListContainersRequest) (*api.ListContainersResponse, error) {
	containers, err := s.store.List(ctx, req.Filters...)
	if err != nil {
		return nil, errdefs.ToGRPC(err)
	}

	return &api.ListContainersResponse{
		Containers: containersToProto(containers),
	}, nil
}

func (s *service) Create(ctx context.Context, req *api.CreateContainerRequest) (*api.CreateContainerResponse, error) {
	container := containerFromProto(&req.Container)
	created, err := s.store.Create(ctx, container)
	if err != nil {
		return nil, errdefs.ToGRPC(err)
	}

	return &api.CreateContainerResponse{
		Container: containerToProto(&created),
	}, nil
}

func (s *service) Update(ctx context.Context, req *api.UpdateContainerRequest) (*api.UpdateContainerResponse, error) {
	if req.Container.ID == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Container.ID required")
	}
	container := containerFromProto(&req.Container)
	var fieldpaths []string
	if req.UpdateMask != nil && len(req.UpdateMask.Paths) > 0 {
		for _, path := range req.UpdateMask.Paths {
			fieldpaths = append(fieldpaths, path)
		}
	}

	updated, err := s.store.Update(ctx, container, fieldpaths...)
	if err != nil {
		return nil, errdefs.ToGRPC(err)
	}

	return &api.UpdateContainerResponse{
		Container: containerToProto(&updated),
	}, nil
}

func (s *service) Delete(ctx context.Context, req *api.DeleteContainerRequest) (*ptypes.Empty, error) {
	return &ptypes.Empty{}, errdefs.ToGRPC(s.store.Delete(ctx, req.ID))
}

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
	"context"

	"github.com/boltdb/bolt"
	eventstypes "github.com/containerd/containerd/api/events"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/events"
	"github.com/containerd/containerd/metadata"
	"github.com/containerd/containerd/plugin"
	"github.com/containerd/containerd/services"
)

// store is a container metadata store.
type store struct {
	db        *metadata.DB
	publisher events.Publisher
}

func init() {
	plugin.Register(&plugin.Registration{
		Type: plugin.ServicePlugin,
		ID:   services.ContainersService,
		Requires: []plugin.Type{
			plugin.MetadataPlugin,
		},
		InitFn: func(ic *plugin.InitContext) (interface{}, error) {
			m, err := ic.Get(plugin.MetadataPlugin)
			if err != nil {
				return nil, err
			}

			return newContainerStore(m.(*metadata.DB), ic.Events), nil
		},
	})
}

func newContainerStore(db *metadata.DB, publisher events.Publisher) containers.Store {
	return &store{db: db, publisher: publisher}
}

func (s *store) Get(ctx context.Context, id string) (containers.Container, error) {
	var c containers.Container
	return c, s.withStoreView(ctx, func(ctx context.Context, store containers.Store) (err error) {
		c, err = store.Get(ctx, id)
		return err
	})
}

func (s *store) List(ctx context.Context, fs ...string) ([]containers.Container, error) {
	var cs []containers.Container
	return cs, s.withStoreView(ctx, func(ctx context.Context, store containers.Store) (err error) {
		cs, err = store.List(ctx, fs...)
		return err
	})
}

func (s *store) Create(ctx context.Context, container containers.Container) (containers.Container, error) {
	var c containers.Container
	if err := s.withStoreUpdate(ctx, func(ctx context.Context, store containers.Store) (err error) {
		c, err = store.Create(ctx, container)
		return err
	}); err != nil {
		return containers.Container{}, err
	}
	if err := s.publisher.Publish(ctx, "/containers/create", &eventstypes.ContainerCreate{
		ID:    container.ID,
		Image: container.Image,
		Runtime: &eventstypes.ContainerCreate_Runtime{
			Name:    container.Runtime.Name,
			Options: container.Runtime.Options,
		},
	}); err != nil {
		return containers.Container{}, err
	}
	return c, nil
}

func (s *store) Update(ctx context.Context, container containers.Container, fieldpaths ...string) (containers.Container, error) {
	var c containers.Container
	if err := s.withStoreUpdate(ctx, func(ctx context.Context, store containers.Store) (err error) {
		c, err = store.Update(ctx, container, fieldpaths...)
		return err
	}); err != nil {
		return containers.Container{}, err
	}
	if err := s.publisher.Publish(ctx, "/containers/update", &eventstypes.ContainerUpdate{
		ID:          container.ID,
		Image:       container.Image,
		Labels:      container.Labels,
		SnapshotKey: container.SnapshotKey,
	}); err != nil {
		return containers.Container{}, err
	}
	return c, nil
}

func (s *store) Delete(ctx context.Context, id string) error {
	if err := s.withStoreUpdate(ctx, func(ctx context.Context, store containers.Store) error {
		return store.Delete(ctx, id)
	}); err != nil {
		return err
	}
	if err := s.publisher.Publish(ctx, "/containers/delete", &eventstypes.ContainerDelete{
		ID: id,
	}); err != nil {
		return err
	}
	return nil
}

func (s *store) withStore(ctx context.Context, fn func(ctx context.Context, store containers.Store) error) func(tx *bolt.Tx) error {
	return func(tx *bolt.Tx) error { return fn(ctx, metadata.NewContainerStore(tx)) }
}

func (s *store) withStoreView(ctx context.Context, fn func(ctx context.Context, store containers.Store) error) error {
	return s.db.View(s.withStore(ctx, fn))
}

func (s *store) withStoreUpdate(ctx context.Context, fn func(ctx context.Context, store containers.Store) error) error {
	return s.db.Update(s.withStore(ctx, fn))
}

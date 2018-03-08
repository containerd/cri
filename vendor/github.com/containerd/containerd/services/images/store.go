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

package images

import (
	"context"

	eventstypes "github.com/containerd/containerd/api/events"
	"github.com/containerd/containerd/events"
	"github.com/containerd/containerd/gc"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/metadata"
	"github.com/containerd/containerd/plugin"
	"github.com/containerd/containerd/services"
)

// store wraps images.Store with proper event published and
// gc scheduled.
type store struct {
	images.Store
	gc        gcScheduler
	publisher events.Publisher
}

type gcScheduler interface {
	ScheduleAndWait(context.Context) (gc.Stats, error)
}

func init() {
	plugin.Register(&plugin.Registration{
		Type: plugin.ServicePlugin,
		ID:   services.ImagesService,
		Requires: []plugin.Type{
			plugin.MetadataPlugin,
			plugin.GCPlugin,
		},
		InitFn: func(ic *plugin.InitContext) (interface{}, error) {
			m, err := ic.Get(plugin.MetadataPlugin)
			if err != nil {
				return nil, err
			}
			g, err := ic.Get(plugin.GCPlugin)
			if err != nil {
				return nil, err
			}

			return newImageStore(metadata.NewImageStore(m.(*metadata.DB)), ic.Events, g.(gcScheduler)), nil
		},
	})
}

func newImageStore(is images.Store, publisher events.Publisher, gc gcScheduler) images.Store {
	return &store{
		Store:     is,
		gc:        gc,
		publisher: publisher,
	}
}

func (s *store) Create(ctx context.Context, image images.Image) (images.Image, error) {
	created, err := s.Store.Create(ctx, image)
	if err != nil {
		return images.Image{}, err
	}
	if err := s.publisher.Publish(ctx, "/images/create", &eventstypes.ImageCreate{
		Name:   image.Name,
		Labels: image.Labels,
	}); err != nil {
		return images.Image{}, err
	}
	return created, nil
}

func (s *store) Update(ctx context.Context, image images.Image, fieldpaths ...string) (images.Image, error) {
	updated, err := s.Store.Update(ctx, image, fieldpaths...)
	if err != nil {
		return images.Image{}, err
	}
	if err := s.publisher.Publish(ctx, "/images/update", &eventstypes.ImageUpdate{
		Name:   image.Name,
		Labels: image.Labels,
	}); err != nil {
		return images.Image{}, err
	}
	return updated, nil
}

func (s *store) Delete(ctx context.Context, name string, opts ...images.DeleteOpt) error {
	var deleteOpts images.DeleteOptions
	for _, o := range opts {
		if err := o(ctx, &deleteOpts); err != nil {
			return err
		}
	}
	if err := s.Store.Delete(ctx, name); err != nil {
		return err
	}
	if err := s.publisher.Publish(ctx, "/images/delete", &eventstypes.ImageDelete{
		Name: name,
	}); err != nil {
		return err
	}
	if deleteOpts.Synchronous {
		if _, err := s.gc.ScheduleAndWait(ctx); err != nil {
			return err
		}
	}
	return nil
}

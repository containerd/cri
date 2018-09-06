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
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/images"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	runtime "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
)

// RemoveImage removes the image.
// TODO(random-liu): Update CRI to pass image reference instead of ImageSpec. (See
// kubernetes/kubernetes#46255)
// TODO(random-liu): We should change CRI to distinguish image id and image spec.
// Remove the whole image no matter the it's image id or reference. This is the
// semantic defined in CRI now.
func (c *criService) RemoveImage(ctx context.Context, r *runtime.RemoveImageRequest) (*runtime.RemoveImageResponse, error) {
	image, err := c.localResolve(ctx, r.GetImage().GetImage())
	if err != nil {
		return nil, errors.Wrapf(err, "can not resolve %q locally", r.GetImage().GetImage())
	}
	if image == nil {
		// return empty without error when image not found.
		return &runtime.RemoveImageResponse{}, nil
	}

	// Exclude outdated image tag.
	for i, tag := range image.RepoTags {
		cImage, err := c.client.GetImage(ctx, tag)
		if err != nil {
			if errdefs.IsNotFound(err) {
				continue
			}
			return nil, errors.Wrapf(err, "failed to get image %q", tag)
		}
		target := cImage.Target()
		cID := target.Digest.String()
		if cID != image.ID {
			logrus.Debugf("Image tag %q for %q is outdated, it's currently used by %q", tag, image.ID, cID)
			image.RepoTags = append(image.RepoTags[:i], image.RepoTags[i+1:]...)
			continue
		}
	}

	// Include all image references, including RepoTag, RepoDigest and id.
	for _, ref := range append(image.RepoTags, image.RepoDigests...) {
		err = c.client.ImageService().Delete(ctx, ref)
		if err == nil || errdefs.IsNotFound(err) {
			continue
		}
		return nil, errors.Wrapf(err, "failed to delete image reference %q for image %q", ref, image.ID)
	}
	// Delete image id synchronously to trigger garbage collection.
	err = c.client.ImageService().Delete(ctx, image.ID, images.SynchronousDelete())
	if err != nil && !errdefs.IsNotFound(err) {
		return nil, errors.Wrapf(err, "failed to delete image id %q", image.ID)
	}
	c.imageStore.Delete(image.ID)
	return &runtime.RemoveImageResponse{}, nil
}

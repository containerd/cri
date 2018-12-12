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

package integration

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// Test to test that an image can't be removed when it is being used by a containerd.
func TestImageRemove(t *testing.T) {
	const testImage = "busybox:latest"

	t.Logf("Make sure no such image in cri")
	img, err := imageService.ImageStatus(&runtime.ImageSpec{Image: testImage})
	require.NoError(t, err)
	if img != nil {
		require.NoError(t, imageService.RemoveImage(&runtime.ImageSpec{Image: testImage}))
	}

	t.Logf("Create a container with the test image")
	sbConfig := PodSandboxConfig("sandbox", "image-remove")
	sb, err := runtimeService.RunPodSandbox(sbConfig, *runtimeHandler)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, runtimeService.StopPodSandbox(sb))
		assert.NoError(t, runtimeService.RemovePodSandbox(sb))
	}()

	t.Logf("Pull the test image")
	i, err := imageService.PullImage(&runtime.ImageSpec{Image: testImage}, nil, sbConfig)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, imageService.RemoveImage(&runtime.ImageSpec{Image: i}))
	}()

	cnConfig := ContainerConfig(
		"test-container",
		testImage,
		WithCommand("echo", "hello"),
	)
	cn, err := runtimeService.CreateContainer(sb, cnConfig, sbConfig)
	require.NoError(t, err)

	t.Logf("The image can't be removed when a container is using it")
	assert.Error(t, imageService.RemoveImage(&runtime.ImageSpec{Image: i}))

	t.Logf("The image can be removed when the container using it is removed")
	require.NoError(t, runtimeService.RemoveContainer(cn))
	assert.NoError(t, imageService.RemoveImage(&runtime.ImageSpec{Image: i}))
}

// Test to test that there is no race condition between image removal and container creation.
func TestImageRemoveRaceCondition(t *testing.T) {
	const (
		testImage = "busybox:latest"
		times     = 10
	)

	t.Logf("Make sure no such image in cri")
	img, err := imageService.ImageStatus(&runtime.ImageSpec{Image: testImage})
	require.NoError(t, err)
	if img != nil {
		require.NoError(t, imageService.RemoveImage(&runtime.ImageSpec{Image: testImage}))
	}

	f := func() {
		t.Logf("Create a container with the test image")
		sbConfig := PodSandboxConfig("sandbox", "image-remove")
		sb, err := runtimeService.RunPodSandbox(sbConfig, *runtimeHandler)
		require.NoError(t, err)
		defer func() {
			assert.NoError(t, runtimeService.StopPodSandbox(sb))
			assert.NoError(t, runtimeService.RemovePodSandbox(sb))
		}()

		t.Logf("Pull the test image")
		i, err := imageService.PullImage(&runtime.ImageSpec{Image: testImage}, nil, sbConfig)
		require.NoError(t, err)
		defer func() {
			assert.NoError(t, imageService.RemoveImage(&runtime.ImageSpec{Image: i}))
		}()

		var wg sync.WaitGroup
		var containerCreateErr, imageRemoveErr error
		wg.Add(1)
		go func() {
			cnConfig := ContainerConfig(
				"test-container",
				testImage,
				WithCommand("echo", "hello"),
			)
			_, containerCreateErr = runtimeService.CreateContainer(sb, cnConfig, sbConfig)
			wg.Done()
		}()

		t.Logf("Remove the image")
		wg.Add(1)
		go func() {
			imageRemoveErr = imageService.RemoveImage(&runtime.ImageSpec{Image: i})
			wg.Done()
		}()

		wg.Wait()
		t.Logf("Either image remove or container create should fail")
		assert.NotEqual(t, containerCreateErr != nil, imageRemoveErr != nil,
			"container create err: %v, image remove err: %v", containerCreateErr, imageRemoveErr)
	}

	for i := 1; i <= times; i++ {
		t.Logf("%d test run", i)
		f()
	}
}

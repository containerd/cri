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
	"context"
	"io/ioutil"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/containerd/containerd/namespaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func TestImageLoadNamespaced(t *testing.T) {
	const ns = "test.dev"
	ctx, crt, img, err := NamespaceContextAndRawClients(ns)
	assert.NoError(t, err)
	assert.NoError(t, StartManagedNamespace(ns, crt))
	t.Run(ns, testImageLoad(ctx, crt, img))
}

func testImageLoad(ctx context.Context, crt runtime.RuntimeServiceClient, img runtime.ImageServiceClient) func(*testing.T) {
	return func(t *testing.T) {
		ns, err := namespaces.NamespaceRequired(ctx)
		require.NoError(t, err)

		testImage := "busybox:latest"
		loadedImage := "docker.io/library/" + testImage
		_, err = exec.LookPath("docker")
		if err != nil {
			t.Skipf("Docker is not available: %v", err)
		}
		t.Logf("docker save image into tarball")
		output, err := exec.Command("docker", "pull", testImage).CombinedOutput()
		require.NoError(t, err, "output: %q", output)
		tarF, err := ioutil.TempFile("", "image-load")
		tar := tarF.Name()
		require.NoError(t, err)
		defer func() {
			assert.NoError(t, os.RemoveAll(tar))
		}()
		output, err = exec.Command("docker", "save", testImage, "-o", tar).CombinedOutput()
		require.NoError(t, err, "output: %q", output)

		t.Logf("make sure no such image in cri")
		res, err := img.ImageStatus(ctx, &runtime.ImageStatusRequest{Image: &runtime.ImageSpec{Image: testImage}})
		require.NoError(t, err)
		require.NotNil(t, res)
		if res.Image != nil {
			_, err = img.RemoveImage(ctx, &runtime.RemoveImageRequest{Image: &runtime.ImageSpec{Image: testImage}})
			require.NoError(t, err)
		}

		t.Logf("load image in cri")
		ctr, err := exec.LookPath("ctr")
		require.NoError(t, err, "ctr should be installed, make sure you've run `make install.deps`")
		output, err = exec.Command(ctr, "-address="+containerdEndpoint,
			"-n="+ns, "images", "import", tar).CombinedOutput()
		require.NoError(t, err, "output: %q", output)

		t.Logf("make sure image is loaded")
		// Use Eventually because the cri plugin needs a short period of time
		// to pick up images imported into containerd directly.
		require.NoError(t, Eventually(func() (bool, error) {
			res, err = img.ImageStatus(ctx, &runtime.ImageStatusRequest{Image: &runtime.ImageSpec{Image: testImage}})
			if err != nil {
				return false, err
			}
			return res != nil, nil
		}, 100*time.Millisecond, 10*time.Second))
		require.Equal(t, []string{loadedImage}, res.Image.RepoTags)

		t.Logf("create a container with the loaded image")
		sbConfig := PodSandboxConfig("sandbox", Randomize("image-load"))
		pbr, err := crt.RunPodSandbox(ctx, &runtime.RunPodSandboxRequest{
			Config: sbConfig, RuntimeHandler: *runtimeHandler,
		})
		require.NoError(t, err)
		require.NotNil(t, pbr)
		defer func() {
			_, err = crt.StopPodSandbox(ctx, &runtime.StopPodSandboxRequest{PodSandboxId: pbr.PodSandboxId})
			assert.NoError(t, err)
			_, err = crt.RemovePodSandbox(ctx, &runtime.RemovePodSandboxRequest{PodSandboxId: pbr.PodSandboxId})
			assert.NoError(t, err)
		}()
		containerConfig := ContainerConfig(
			"container",
			testImage,
			WithCommand("tail", "-f", "/dev/null"),
		)
		// Rely on sandbox clean to do container cleanup.
		ctc, err := crt.CreateContainer(ctx, &runtime.CreateContainerRequest{
			PodSandboxId: pbr.PodSandboxId, Config: containerConfig, SandboxConfig: sbConfig,
		})
		require.NoError(t, err)
		require.NotNil(t, ctc)

		cts, err := crt.StartContainer(ctx, &runtime.StartContainerRequest{
			ContainerId: ctc.ContainerId,
		})
		require.NoError(t, err)
		require.NotNil(t, cts)

		t.Logf("make sure container is running")
		ctx, err := crt.ContainerStatus(ctx, &runtime.ContainerStatusRequest{
			ContainerId: ctc.ContainerId,
		})
		require.NoError(t, err)
		require.NotNil(t, ctx)
		require.Equal(t, runtime.ContainerState_CONTAINER_RUNNING, ctx.Status.State)
	}
}

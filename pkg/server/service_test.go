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

package server

import (
	"context"

	"github.com/containerd/containerd/namespaces"

	criconfig "github.com/containerd/cri/pkg/config"
	"github.com/containerd/cri/pkg/constants"
	ostesting "github.com/containerd/cri/pkg/os/testing"
	"github.com/containerd/cri/pkg/registrar"
	servertesting "github.com/containerd/cri/pkg/server/testing"
	containerstore "github.com/containerd/cri/pkg/store/container"
	imagestore "github.com/containerd/cri/pkg/store/image"
	sandboxstore "github.com/containerd/cri/pkg/store/sandbox"
	snapshotstore "github.com/containerd/cri/pkg/store/snapshot"
)

const (
	testRootDir  = "/test/root"
	testStateDir = "/test/state"
	// Use an image id as test sandbox image to avoid image name resolve.
	// TODO(random-liu): Change this to image name after we have complete image
	// management unit test framework.
	testSandboxImage = "sha256:c75bebcdd211f41b3a460c7bf82970ed6c75acaab9cd4c9a4e125b03ca113798"
	testImageFSPath  = "/test/image/fs/path"
)

// newTestCRIService creates a fake criService for test.
func newTestCRIService() (*criService, context.Context) {
	return &criService{
		config: serviceConfig{
			RootDir:  testRootDir,
			StateDir: testStateDir,
			PluginConfig: criconfig.PluginConfig{
				SandboxImage: testSandboxImage,
			},
		},
		imageFSPath:        testImageFSPath,
		os:                 ostesting.NewFakeOS(),
		sandboxStore:       sandboxstore.NewStore(),
		imageStore:         imagestore.NewStore(nil),
		snapshotStore:      snapshotstore.NewStore(),
		sandboxNameIndex:   registrar.NewRegistrar(),
		containerStore:     containerstore.NewStore(),
		containerNameIndex: registrar.NewRegistrar(),
		netPlugin:          servertesting.NewFakeCNIPlugin(),
	}, namespaces.WithNamespace(context.Background(), constants.K8sContainerdNamespace)
}

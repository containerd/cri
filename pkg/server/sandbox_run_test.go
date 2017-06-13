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
	"encoding/json"
	"io"
	"os"
	"syscall"
	"testing"

	"github.com/containerd/containerd/api/services/execution"
	snapshotapi "github.com/containerd/containerd/api/services/snapshot"
	imagedigest "github.com/opencontainers/go-digest"
	imagespec "github.com/opencontainers/image-spec/specs-go/v1"
	runtimespec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
	runtime "k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1"

	"github.com/kubernetes-incubator/cri-containerd/pkg/metadata"
	ostesting "github.com/kubernetes-incubator/cri-containerd/pkg/os/testing"
	servertesting "github.com/kubernetes-incubator/cri-containerd/pkg/server/testing"
)

func getRunPodSandboxTestData() (*runtime.PodSandboxConfig, *imagespec.ImageConfig, func(*testing.T, string, *runtimespec.Spec)) {
	config := &runtime.PodSandboxConfig{
		Metadata: &runtime.PodSandboxMetadata{
			Name:      "test-name",
			Uid:       "test-uid",
			Namespace: "test-ns",
			Attempt:   1,
		},
		Hostname:     "test-hostname",
		LogDirectory: "test-log-directory",
		Labels:       map[string]string{"a": "b"},
		Annotations:  map[string]string{"c": "d"},
		Linux: &runtime.LinuxPodSandboxConfig{
			CgroupParent: "/test/cgroup/parent",
		},
	}
	imageConfig := &imagespec.ImageConfig{
		Env:        []string{"a=b", "c=d"},
		Entrypoint: []string{"/pause"},
		Cmd:        []string{"forever"},
		WorkingDir: "/workspace",
	}
	specCheck := func(t *testing.T, id string, spec *runtimespec.Spec) {
		assert.Equal(t, "test-hostname", spec.Hostname)
		assert.Equal(t, getCgroupsPath("/test/cgroup/parent", id), spec.Linux.CgroupsPath)
		assert.Equal(t, relativeRootfsPath, spec.Root.Path)
		assert.Equal(t, true, spec.Root.Readonly)
		assert.Contains(t, spec.Process.Env, "a=b", "c=d")
		assert.Equal(t, []string{"/pause", "forever"}, spec.Process.Args)
		assert.Equal(t, "/workspace", spec.Process.Cwd)
	}
	return config, imageConfig, specCheck
}

func TestGenerateSandboxContainerSpec(t *testing.T) {
	testID := "test-id"
	for desc, test := range map[string]struct {
		configChange      func(*runtime.PodSandboxConfig)
		imageConfigChange func(*imagespec.ImageConfig)
		specCheck         func(*testing.T, *runtimespec.Spec)
		expectErr         bool
	}{
		"spec should reflect original config": {
			specCheck: func(t *testing.T, spec *runtimespec.Spec) {
				// runtime spec should have expected namespaces enabled by default.
				require.NotNil(t, spec.Linux)
				assert.Contains(t, spec.Linux.Namespaces, runtimespec.LinuxNamespace{
					Type: runtimespec.NetworkNamespace,
				})
				assert.Contains(t, spec.Linux.Namespaces, runtimespec.LinuxNamespace{
					Type: runtimespec.PIDNamespace,
				})
				assert.Contains(t, spec.Linux.Namespaces, runtimespec.LinuxNamespace{
					Type: runtimespec.IPCNamespace,
				})
			},
		},
		"host namespace": {
			configChange: func(c *runtime.PodSandboxConfig) {
				c.Linux.SecurityContext = &runtime.LinuxSandboxSecurityContext{
					NamespaceOptions: &runtime.NamespaceOption{
						HostNetwork: true,
						HostPid:     true,
						HostIpc:     true,
					},
				}
			},
			specCheck: func(t *testing.T, spec *runtimespec.Spec) {
				// runtime spec should disable expected namespaces in host mode.
				require.NotNil(t, spec.Linux)
				assert.NotContains(t, spec.Linux.Namespaces, runtimespec.LinuxNamespace{
					Type: runtimespec.NetworkNamespace,
				})
				assert.NotContains(t, spec.Linux.Namespaces, runtimespec.LinuxNamespace{
					Type: runtimespec.PIDNamespace,
				})
				assert.NotContains(t, spec.Linux.Namespaces, runtimespec.LinuxNamespace{
					Type: runtimespec.IPCNamespace,
				})
			},
		},
		"should return error when entrypoint is empty": {
			imageConfigChange: func(c *imagespec.ImageConfig) {
				c.Entrypoint = nil
			},
			expectErr: true,
		},
		"should return error when env is invalid ": {
			// Also covers addImageEnvs.
			imageConfigChange: func(c *imagespec.ImageConfig) {
				c.Env = []string{"a"}
			},
			expectErr: true,
		},
	} {
		t.Logf("TestCase %q", desc)
		c := newTestCRIContainerdService()
		config, imageConfig, specCheck := getRunPodSandboxTestData()
		if test.configChange != nil {
			test.configChange(config)
		}
		if test.imageConfigChange != nil {
			test.imageConfigChange(imageConfig)
		}
		spec, err := c.generateSandboxContainerSpec(testID, config, imageConfig)
		if test.expectErr {
			assert.Error(t, err)
			assert.Nil(t, spec)
			continue
		}
		assert.NoError(t, err)
		assert.NotNil(t, spec)
		specCheck(t, testID, spec)
		if test.specCheck != nil {
			test.specCheck(t, spec)
		}
	}
}

func TestSetupSandboxFiles(t *testing.T) {
	testRootDir := "test-sandbox-root"
	for desc, test := range map[string]struct {
		dnsConfig     *runtime.DNSConfig
		hostIpc       bool
		expectedCalls []ostesting.CalledDetail
	}{
		"should check host /dev/shm existence when hostIpc is true": {
			hostIpc: true,
			expectedCalls: []ostesting.CalledDetail{
				{
					Name: "CopyFile",
					Arguments: []interface{}{
						"/etc/hosts", testRootDir + "/hosts", os.FileMode(0644),
					},
				},
				{
					Name: "CopyFile",
					Arguments: []interface{}{
						"/etc/resolv.conf", testRootDir + "/resolv.conf", os.FileMode(0644),
					},
				},
				{
					Name:      "Stat",
					Arguments: []interface{}{"/dev/shm"},
				},
			},
		},
		"should create new /etc/resolv.conf if DNSOptions is set": {
			dnsConfig: &runtime.DNSConfig{
				Servers:  []string{"8.8.8.8"},
				Searches: []string{"114.114.114.114"},
				Options:  []string{"timeout:1"},
			},
			hostIpc: true,
			expectedCalls: []ostesting.CalledDetail{
				{
					Name: "CopyFile",
					Arguments: []interface{}{
						"/etc/hosts", testRootDir + "/hosts", os.FileMode(0644),
					},
				},
				{
					Name: "WriteFile",
					Arguments: []interface{}{
						testRootDir + "/resolv.conf", []byte(`search 114.114.114.114
nameserver 8.8.8.8
options timeout:1
`), os.FileMode(0644),
					},
				},
				{
					Name:      "Stat",
					Arguments: []interface{}{"/dev/shm"},
				},
			},
		},
		"should create sandbox shm when hostIpc is false": {
			hostIpc: false,
			expectedCalls: []ostesting.CalledDetail{
				{
					Name: "CopyFile",
					Arguments: []interface{}{
						"/etc/hosts", testRootDir + "/hosts", os.FileMode(0644),
					},
				},
				{
					Name: "CopyFile",
					Arguments: []interface{}{
						"/etc/resolv.conf", testRootDir + "/resolv.conf", os.FileMode(0644),
					},
				},
				{
					Name: "MkdirAll",
					Arguments: []interface{}{
						testRootDir + "/shm", os.FileMode(0700),
					},
				},
				{
					Name: "Mount",
					// Ignore arguments which are too complex to check.
				},
			},
		},
	} {
		t.Logf("TestCase %q", desc)
		c := newTestCRIContainerdService()
		cfg := &runtime.PodSandboxConfig{
			DnsConfig: test.dnsConfig,
			Linux: &runtime.LinuxPodSandboxConfig{
				SecurityContext: &runtime.LinuxSandboxSecurityContext{
					NamespaceOptions: &runtime.NamespaceOption{
						HostIpc: test.hostIpc,
					},
				},
			},
		}
		c.setupSandboxFiles(testRootDir, cfg)
		calls := c.os.(*ostesting.FakeOS).GetCalls()
		assert.Len(t, calls, len(test.expectedCalls))
		for i, expected := range test.expectedCalls {
			if expected.Arguments == nil {
				// Ignore arguments.
				expected.Arguments = calls[i].Arguments
			}
			assert.Equal(t, expected, calls[i])
		}
	}
}

func TestRunPodSandbox(t *testing.T) {
	config, imageConfig, specCheck := getRunPodSandboxTestData()
	c := newTestCRIContainerdService()
	fakeSnapshotClient := c.snapshotService.(*servertesting.FakeSnapshotClient)
	fakeExecutionClient := c.containerService.(*servertesting.FakeExecutionClient)
	fakeCNIPlugin := c.netPlugin.(*servertesting.FakeCNIPlugin)
	fakeOS := c.os.(*ostesting.FakeOS)
	var dirs []string
	var pipes []string
	fakeOS.MkdirAllFn = func(path string, perm os.FileMode) error {
		dirs = append(dirs, path)
		return nil
	}
	fakeOS.OpenFifoFn = func(ctx context.Context, fn string, flag int, perm os.FileMode) (io.ReadWriteCloser, error) {
		pipes = append(pipes, fn)
		assert.Equal(t, syscall.O_RDONLY|syscall.O_CREAT|syscall.O_NONBLOCK, flag)
		assert.Equal(t, os.FileMode(0700), perm)
		return nopReadWriteCloser{}, nil
	}
	testChainID := imagedigest.Digest("test-sandbox-chain-id")
	imageMetadata := metadata.ImageMetadata{
		ID:      testSandboxImage,
		ChainID: testChainID.String(),
		Config:  imageConfig,
	}
	// Insert sandbox image metadata.
	assert.NoError(t, c.imageMetadataStore.Create(imageMetadata))
	// Insert fake chainID
	fakeSnapshotClient.SetFakeChainIDs([]imagedigest.Digest{testChainID})
	expectSnapshotClientCalls := []string{"prepare"}
	expectExecutionClientCalls := []string{"create", "start"}

	res, err := c.RunPodSandbox(context.Background(), &runtime.RunPodSandboxRequest{Config: config})
	assert.NoError(t, err)
	require.NotNil(t, res)
	id := res.GetPodSandboxId()

	sandboxRootDir := getSandboxRootDir(c.rootDir, id)
	assert.Contains(t, dirs[0], sandboxRootDir, "sandbox root directory should be created")

	assert.Len(t, pipes, 2)
	_, stdout, stderr := getStreamingPipes(sandboxRootDir)
	assert.Contains(t, pipes, stdout, "sandbox stdout pipe should be created")
	assert.Contains(t, pipes, stderr, "sandbox stderr pipe should be created")

	assert.Equal(t, expectSnapshotClientCalls, fakeSnapshotClient.GetCalledNames(), "expect snapshot functions should be called")
	calls := fakeSnapshotClient.GetCalledDetails()
	prepareOpts := calls[0].Argument.(*snapshotapi.PrepareRequest)
	assert.Equal(t, &snapshotapi.PrepareRequest{
		Name:     id,
		ChainID:  testChainID,
		Readonly: true,
	}, prepareOpts, "prepare request should be correct")

	assert.Equal(t, expectExecutionClientCalls, fakeExecutionClient.GetCalledNames(), "expect containerd functions should be called")
	calls = fakeExecutionClient.GetCalledDetails()
	createOpts := calls[0].Argument.(*execution.CreateRequest)
	assert.Equal(t, id, createOpts.ID, "create id should be correct")
	assert.Equal(t, stdout, createOpts.Stdout, "stdout pipe should be passed to containerd")
	assert.Equal(t, stderr, createOpts.Stderr, "stderr pipe should be passed to containerd")
	mountsResp, err := fakeSnapshotClient.Mounts(context.Background(), &snapshotapi.MountsRequest{Name: id})
	assert.NoError(t, err)
	assert.Equal(t, mountsResp.Mounts, createOpts.Rootfs, "rootfs mount should be correct")
	spec := &runtimespec.Spec{}
	assert.NoError(t, json.Unmarshal(createOpts.Spec.Value, spec))
	t.Logf("oci spec check")
	specCheck(t, id, spec)

	startID := calls[1].Argument.(*execution.StartRequest).ID
	assert.Equal(t, id, startID, "start id should be correct")

	meta, err := c.sandboxStore.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, id, meta.ID, "metadata id should be correct")
	err = c.sandboxNameIndex.Reserve(meta.Name, "random-id")
	assert.Error(t, err, "metadata name should be reserved")
	assert.Equal(t, config, meta.Config, "metadata config should be correct")
	// TODO(random-liu): [P2] Add clock interface and use fake clock.
	assert.NotZero(t, meta.CreatedAt, "metadata CreatedAt should be set")
	info, err := fakeExecutionClient.Info(context.Background(), &execution.InfoRequest{ID: id})
	assert.NoError(t, err)
	pid := info.Pid
	assert.Equal(t, meta.NetNS, getNetworkNamespace(pid), "metadata network namespace should be correct")

	gotID, err := c.sandboxIDIndex.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, id, gotID, "sandbox id should be indexed")

	expectedCNICalls := []string{"SetUpPod"}
	assert.Equal(t, expectedCNICalls, fakeCNIPlugin.GetCalledNames(), "expect SetUpPod should be called")
	calls = fakeCNIPlugin.GetCalledDetails()
	pluginArgument := calls[0].Argument.(servertesting.CNIPluginArgument)
	expectedPluginArgument := servertesting.CNIPluginArgument{meta.NetNS, config.GetMetadata().GetNamespace(), config.GetMetadata().GetName(), id}
	assert.Equal(t, expectedPluginArgument, pluginArgument, "SetUpPod should be called with correct arguments")
}

func TestParseDNSOption(t *testing.T) {
	for desc, test := range map[string]struct {
		servers         []string
		searches        []string
		options         []string
		expectedContent string
		expectErr       bool
	}{
		"empty dns options should return empty content": {},
		"non-empty dns options should return correct content": {
			servers:  []string{"8.8.8.8", "server.google.com"},
			searches: []string{"114.114.114.114"},
			options:  []string{"timeout:1"},
			expectedContent: `search 114.114.114.114
nameserver 8.8.8.8
nameserver server.google.com
options timeout:1
`,
		},
		"should return error if dns search exceeds limit(6)": {
			searches: []string{
				"server0.google.com",
				"server1.google.com",
				"server2.google.com",
				"server3.google.com",
				"server4.google.com",
				"server5.google.com",
				"server6.google.com",
			},
			expectErr: true,
		},
	} {
		t.Logf("TestCase %q", desc)
		resolvContent, err := parseDNSOptions(test.servers, test.searches, test.options)
		if test.expectErr {
			assert.Error(t, err)
			continue
		}
		assert.NoError(t, err)
		assert.Equal(t, resolvContent, test.expectedContent)
	}
}

// TODO(random-liu): [P1] Add unit test for different error cases to make sure
// the function cleans up on error properly.

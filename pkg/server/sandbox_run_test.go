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
	"net"
	"os"
	"path/filepath"
	"testing"

	cni "github.com/containerd/go-cni"
	"github.com/containerd/typeurl"
	imagespec "github.com/opencontainers/image-spec/specs-go/v1"
	runtimespec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	runtime "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"

	"github.com/containerd/cri/pkg/annotations"
	criconfig "github.com/containerd/cri/pkg/config"
	ostesting "github.com/containerd/cri/pkg/os/testing"
	sandboxstore "github.com/containerd/cri/pkg/store/sandbox"
	sandboxAnnotations "github.com/kata-containers/runtime/virtcontainers/pkg/annotations"
)

func getRunPodSandboxTestData() (*runtime.PodSandboxConfig, *imagespec.ImageConfig, func(*testing.T, string, *runtimespec.Spec)) {

	kernelPath := "/var/runtime/kernel"
	initrdPath := "/var/runtime/initrd.img"
	imagePath := "/var/runtime/image.img"
	hypervisorPath := "/usr/bin/qemu-custom"
	firmwarePath := "/var/run/custom-firmware/img"

	kernelHash := "3971EBF11A18BCE250A282E6FF78739C9A0F7BCB9BC3552FE9CD8191E4A4DA573BB38983A47B2A86A63BFC4E41D3361D5D6AE1A1480BF3F27F730CC75FBC84DE"
	initrdHash := "652DCF7057417D54C3236731A2CA47330B8809236B87DA89917403FEAAC34EA5DBE938EE60FDD606383B38DF9B51239D25968DDF1C0445C0E7AB448287197D64"
	imageHash := "31D329DA77A2D0EF2943209EF432E3F0C7ED5B57124AFF1E37A5C7E6A01D780A473BD0FD594A3FA7CFBC2573FB150702BB11014C090D9222F9A0C048771F6F2A"
	hypervisorHash := "0D4C6CCCCDCA141804EB236A07598FF118435450F58A181EED37E6E2D037720E1847473687558289F3972BDE6A9FD2E1F9EDC8C9B0550621D04198111FA955AD"
	firmwareHash := "259870D2FF9DCB1C44E2D0C6FB467364422D9643E0577A55397E15ECEADA4668A9D1AD7FF776D1A93B9A70FE8C8C4EA77FE75E4BD21E7C24F1490FE09811CBD8"

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
		Annotations: map[string]string{"c": "d",
			sandboxAnnotations.KernelPath:     kernelPath,
			sandboxAnnotations.InitrdPath:     initrdPath,
			sandboxAnnotations.ImagePath:      imagePath,
			sandboxAnnotations.HypervisorPath: hypervisorPath,
			sandboxAnnotations.FirmwarePath:   firmwarePath,
			sandboxAnnotations.KernelHash:     kernelHash,
			sandboxAnnotations.InitrdHash:     initrdHash,
			sandboxAnnotations.ImageHash:      imageHash,
			sandboxAnnotations.HypervisorHash: hypervisorHash,
			sandboxAnnotations.FirmwareHash:   firmwareHash,
		},
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
		assert.Equal(t, getCgroupsPath("/test/cgroup/parent", id, false), spec.Linux.CgroupsPath)
		assert.Equal(t, relativeRootfsPath, spec.Root.Path)
		assert.Equal(t, true, spec.Root.Readonly)
		assert.Contains(t, spec.Process.Env, "a=b", "c=d")
		assert.Equal(t, []string{"/pause", "forever"}, spec.Process.Args)
		assert.Equal(t, "/workspace", spec.Process.Cwd)
		assert.EqualValues(t, *spec.Linux.Resources.CPU.Shares, defaultSandboxCPUshares)
		assert.EqualValues(t, *spec.Process.OOMScoreAdj, defaultSandboxOOMAdj)

		t.Logf("Check PodSandbox annotations")
		assert.Contains(t, spec.Annotations, annotations.SandboxID)
		assert.EqualValues(t, spec.Annotations[annotations.SandboxID], id)

		assert.Contains(t, spec.Annotations, annotations.ContainerType)
		assert.EqualValues(t, spec.Annotations[annotations.ContainerType], annotations.ContainerTypeSandbox)

		assert.Contains(t, spec.Annotations, sandboxAnnotations.KernelPath)
		assert.EqualValues(t, spec.Annotations[sandboxAnnotations.KernelPath], kernelPath)
		assert.Contains(t, spec.Annotations, sandboxAnnotations.InitrdPath)
		assert.EqualValues(t, spec.Annotations[sandboxAnnotations.InitrdPath], initrdPath)
		assert.Contains(t, spec.Annotations, sandboxAnnotations.ImagePath)
		assert.EqualValues(t, spec.Annotations[sandboxAnnotations.ImagePath], imagePath)
		assert.Contains(t, spec.Annotations, sandboxAnnotations.HypervisorPath)
		assert.EqualValues(t, spec.Annotations[sandboxAnnotations.HypervisorPath], hypervisorPath)
		assert.Contains(t, spec.Annotations, sandboxAnnotations.FirmwarePath)
		assert.EqualValues(t, spec.Annotations[sandboxAnnotations.FirmwarePath], firmwarePath)

		assert.Contains(t, spec.Annotations, sandboxAnnotations.KernelHash)
		assert.EqualValues(t, spec.Annotations[sandboxAnnotations.KernelHash], kernelHash)
		assert.Contains(t, spec.Annotations, sandboxAnnotations.InitrdHash)
		assert.EqualValues(t, spec.Annotations[sandboxAnnotations.InitrdHash], initrdHash)
		assert.Contains(t, spec.Annotations, sandboxAnnotations.ImageHash)
		assert.EqualValues(t, spec.Annotations[sandboxAnnotations.ImageHash], imageHash)
		assert.Contains(t, spec.Annotations, sandboxAnnotations.HypervisorHash)
		assert.EqualValues(t, spec.Annotations[sandboxAnnotations.HypervisorHash], hypervisorHash)
		assert.Contains(t, spec.Annotations, sandboxAnnotations.FirmwareHash)
		assert.EqualValues(t, spec.Annotations[sandboxAnnotations.FirmwareHash], firmwareHash)
	}
	return config, imageConfig, specCheck
}

func TestGenerateSandboxContainerSpec(t *testing.T) {
	testID := "test-id"
	nsPath := "test-cni"
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
					Path: nsPath,
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
						Network: runtime.NamespaceMode_NODE,
						Pid:     runtime.NamespaceMode_NODE,
						Ipc:     runtime.NamespaceMode_NODE,
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
		"should return error when entrypoint and cmd are empty": {
			imageConfigChange: func(c *imagespec.ImageConfig) {
				c.Entrypoint = nil
				c.Cmd = nil
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
		"should set supplemental groups correctly": {
			configChange: func(c *runtime.PodSandboxConfig) {
				c.Linux.SecurityContext = &runtime.LinuxSandboxSecurityContext{
					SupplementalGroups: []int64{1111, 2222},
				}
			},
			specCheck: func(t *testing.T, spec *runtimespec.Spec) {
				require.NotNil(t, spec.Process)
				assert.Contains(t, spec.Process.User.AdditionalGids, uint32(1111))
				assert.Contains(t, spec.Process.User.AdditionalGids, uint32(2222))
			},
		},
	} {
		t.Logf("TestCase %q", desc)
		c := newTestCRIService()
		config, imageConfig, specCheck := getRunPodSandboxTestData()
		if test.configChange != nil {
			test.configChange(config)
		}
		if test.imageConfigChange != nil {
			test.imageConfigChange(imageConfig)
		}
		spec, err := c.generateSandboxContainerSpec(testID, config, imageConfig, nsPath)
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
	const (
		testID       = "test-id"
		realhostname = "test-real-hostname"
	)
	for desc, test := range map[string]struct {
		dnsConfig     *runtime.DNSConfig
		hostname      string
		ipcMode       runtime.NamespaceMode
		expectedCalls []ostesting.CalledDetail
	}{
		"should check host /dev/shm existence when ipc mode is NODE": {
			ipcMode: runtime.NamespaceMode_NODE,
			expectedCalls: []ostesting.CalledDetail{
				{
					Name: "Hostname",
				},
				{
					Name: "WriteFile",
					Arguments: []interface{}{
						filepath.Join(testRootDir, sandboxesDir, testID, "hostname"),
						[]byte(realhostname + "\n"),
						os.FileMode(0644),
					},
				},
				{
					Name: "CopyFile",
					Arguments: []interface{}{
						"/etc/hosts",
						filepath.Join(testRootDir, sandboxesDir, testID, "hosts"),
						os.FileMode(0644),
					},
				},
				{
					Name: "CopyFile",
					Arguments: []interface{}{
						"/etc/resolv.conf",
						filepath.Join(testRootDir, sandboxesDir, testID, "resolv.conf"),
						os.FileMode(0644),
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
			ipcMode: runtime.NamespaceMode_NODE,
			expectedCalls: []ostesting.CalledDetail{
				{
					Name: "Hostname",
				},
				{
					Name: "WriteFile",
					Arguments: []interface{}{
						filepath.Join(testRootDir, sandboxesDir, testID, "hostname"),
						[]byte(realhostname + "\n"),
						os.FileMode(0644),
					},
				},
				{
					Name: "CopyFile",
					Arguments: []interface{}{
						"/etc/hosts",
						filepath.Join(testRootDir, sandboxesDir, testID, "hosts"),
						os.FileMode(0644),
					},
				},
				{
					Name: "WriteFile",
					Arguments: []interface{}{
						filepath.Join(testRootDir, sandboxesDir, testID, "resolv.conf"),
						[]byte(`search 114.114.114.114
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
		"should create sandbox shm when ipc namespace mode is not NODE": {
			ipcMode: runtime.NamespaceMode_POD,
			expectedCalls: []ostesting.CalledDetail{
				{
					Name: "Hostname",
				},
				{
					Name: "WriteFile",
					Arguments: []interface{}{
						filepath.Join(testRootDir, sandboxesDir, testID, "hostname"),
						[]byte(realhostname + "\n"),
						os.FileMode(0644),
					},
				},
				{
					Name: "CopyFile",
					Arguments: []interface{}{
						"/etc/hosts",
						filepath.Join(testRootDir, sandboxesDir, testID, "hosts"),
						os.FileMode(0644),
					},
				},
				{
					Name: "CopyFile",
					Arguments: []interface{}{
						"/etc/resolv.conf",
						filepath.Join(testRootDir, sandboxesDir, testID, "resolv.conf"),
						os.FileMode(0644),
					},
				},
				{
					Name: "MkdirAll",
					Arguments: []interface{}{
						filepath.Join(testStateDir, sandboxesDir, testID, "shm"),
						os.FileMode(0700),
					},
				},
				{
					Name: "Mount",
					// Ignore arguments which are too complex to check.
				},
			},
		},
		"should create /etc/hostname when hostname is set": {
			hostname: "test-hostname",
			ipcMode:  runtime.NamespaceMode_NODE,
			expectedCalls: []ostesting.CalledDetail{
				{
					Name: "WriteFile",
					Arguments: []interface{}{
						filepath.Join(testRootDir, sandboxesDir, testID, "hostname"),
						[]byte("test-hostname\n"),
						os.FileMode(0644),
					},
				},
				{
					Name: "CopyFile",
					Arguments: []interface{}{
						"/etc/hosts",
						filepath.Join(testRootDir, sandboxesDir, testID, "hosts"),
						os.FileMode(0644),
					},
				},
				{
					Name: "CopyFile",
					Arguments: []interface{}{
						"/etc/resolv.conf",
						filepath.Join(testRootDir, sandboxesDir, testID, "resolv.conf"),
						os.FileMode(0644),
					},
				},
				{
					Name:      "Stat",
					Arguments: []interface{}{"/dev/shm"},
				},
			},
		},
	} {
		t.Logf("TestCase %q", desc)
		c := newTestCRIService()
		c.os.(*ostesting.FakeOS).HostnameFn = func() (string, error) {
			return realhostname, nil
		}
		cfg := &runtime.PodSandboxConfig{
			Hostname:  test.hostname,
			DnsConfig: test.dnsConfig,
			Linux: &runtime.LinuxPodSandboxConfig{
				SecurityContext: &runtime.LinuxSandboxSecurityContext{
					NamespaceOptions: &runtime.NamespaceOption{
						Ipc: test.ipcMode,
					},
				},
			},
		}
		c.setupSandboxFiles(testID, cfg)
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

func TestToCNIPortMappings(t *testing.T) {
	for desc, test := range map[string]struct {
		criPortMappings []*runtime.PortMapping
		cniPortMappings []cni.PortMapping
	}{
		"empty CRI port mapping should map to empty CNI port mapping": {},
		"CRI port mapping should be converted to CNI port mapping properly": {
			criPortMappings: []*runtime.PortMapping{
				{
					Protocol:      runtime.Protocol_UDP,
					ContainerPort: 1234,
					HostPort:      5678,
					HostIp:        "123.124.125.126",
				},
				{
					Protocol:      runtime.Protocol_TCP,
					ContainerPort: 4321,
					HostPort:      8765,
					HostIp:        "126.125.124.123",
				},
			},
			cniPortMappings: []cni.PortMapping{
				{
					HostPort:      5678,
					ContainerPort: 1234,
					Protocol:      "udp",
					HostIP:        "123.124.125.126",
				},
				{
					HostPort:      8765,
					ContainerPort: 4321,
					Protocol:      "tcp",
					HostIP:        "126.125.124.123",
				},
			},
		},
		"CRI port mapping without host port should be skipped": {
			criPortMappings: []*runtime.PortMapping{
				{
					Protocol:      runtime.Protocol_UDP,
					ContainerPort: 1234,
					HostIp:        "123.124.125.126",
				},
				{
					Protocol:      runtime.Protocol_TCP,
					ContainerPort: 4321,
					HostPort:      8765,
					HostIp:        "126.125.124.123",
				},
			},
			cniPortMappings: []cni.PortMapping{
				{
					HostPort:      8765,
					ContainerPort: 4321,
					Protocol:      "tcp",
					HostIP:        "126.125.124.123",
				},
			},
		},
		"CRI port mapping with unsupported protocol should be skipped": {
			criPortMappings: []*runtime.PortMapping{
				{
					Protocol:      runtime.Protocol_SCTP,
					ContainerPort: 1234,
					HostPort:      5678,
					HostIp:        "123.124.125.126",
				},
				{
					Protocol:      runtime.Protocol_TCP,
					ContainerPort: 4321,
					HostPort:      8765,
					HostIp:        "126.125.124.123",
				},
			},
			cniPortMappings: []cni.PortMapping{
				{
					HostPort:      8765,
					ContainerPort: 4321,
					Protocol:      "tcp",
					HostIP:        "126.125.124.123",
				},
			},
		},
	} {
		t.Logf("TestCase %q", desc)
		assert.Equal(t, test.cniPortMappings, toCNIPortMappings(test.criPortMappings))
	}
}

func TestSelectPodIP(t *testing.T) {
	for desc, test := range map[string]struct {
		ips      []string
		expected string
	}{
		"ipv4 should be picked even if ipv6 comes first": {
			ips:      []string{"2001:db8:85a3::8a2e:370:7334", "192.168.17.43"},
			expected: "192.168.17.43",
		},
		"ipv6 should be picked when there is no ipv4": {
			ips:      []string{"2001:db8:85a3::8a2e:370:7334"},
			expected: "2001:db8:85a3::8a2e:370:7334",
		},
	} {
		t.Logf("TestCase %q", desc)
		var ipConfigs []*cni.IPConfig
		for _, ip := range test.ips {
			ipConfigs = append(ipConfigs, &cni.IPConfig{
				IP: net.ParseIP(ip),
			})
		}
		assert.Equal(t, test.expected, selectPodIP(ipConfigs))
	}
}

func TestTypeurlMarshalUnmarshalSandboxMeta(t *testing.T) {
	for desc, test := range map[string]struct {
		configChange func(*runtime.PodSandboxConfig)
	}{
		"should marshal original config": {},
		"should marshal Linux": {
			configChange: func(c *runtime.PodSandboxConfig) {
				c.Linux.SecurityContext = &runtime.LinuxSandboxSecurityContext{
					NamespaceOptions: &runtime.NamespaceOption{
						Network: runtime.NamespaceMode_NODE,
						Pid:     runtime.NamespaceMode_NODE,
						Ipc:     runtime.NamespaceMode_NODE,
					},
					SupplementalGroups: []int64{1111, 2222},
				}
			},
		},
	} {
		t.Logf("TestCase %q", desc)
		meta := &sandboxstore.Metadata{
			ID:        "1",
			Name:      "sandbox_1",
			NetNSPath: "/home/cloud",
		}
		meta.Config, _, _ = getRunPodSandboxTestData()
		if test.configChange != nil {
			test.configChange(meta.Config)
		}

		any, err := typeurl.MarshalAny(meta)
		assert.NoError(t, err)
		data, err := typeurl.UnmarshalAny(any)
		assert.NoError(t, err)
		assert.IsType(t, &sandboxstore.Metadata{}, data)
		curMeta, ok := data.(*sandboxstore.Metadata)
		assert.True(t, ok)
		assert.Equal(t, meta, curMeta)
	}
}

func TestHostAccessingSandbox(t *testing.T) {
	privilegedContext := &runtime.PodSandboxConfig{
		Linux: &runtime.LinuxPodSandboxConfig{
			SecurityContext: &runtime.LinuxSandboxSecurityContext{
				Privileged: true,
			},
		},
	}
	nonPrivilegedContext := &runtime.PodSandboxConfig{
		Linux: &runtime.LinuxPodSandboxConfig{
			SecurityContext: &runtime.LinuxSandboxSecurityContext{
				Privileged: false,
			},
		},
	}
	hostNamespace := &runtime.PodSandboxConfig{
		Linux: &runtime.LinuxPodSandboxConfig{
			SecurityContext: &runtime.LinuxSandboxSecurityContext{
				Privileged: false,
				NamespaceOptions: &runtime.NamespaceOption{
					Network: runtime.NamespaceMode_NODE,
					Pid:     runtime.NamespaceMode_NODE,
					Ipc:     runtime.NamespaceMode_NODE,
				},
			},
		},
	}
	tests := []struct {
		name   string
		config *runtime.PodSandboxConfig
		want   bool
	}{
		{"Security Context is nil", nil, false},
		{"Security Context is privileged", privilegedContext, false},
		{"Security Context is not privileged", nonPrivilegedContext, false},
		{"Security Context namespace host access", hostNamespace, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hostAccessingSandbox(tt.config); got != tt.want {
				t.Errorf("hostAccessingSandbox() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetSandboxRuntime(t *testing.T) {
	untrustedWorkloadRuntime := criconfig.Runtime{
		Type:   "io.containerd.runtime.v1.linux",
		Engine: "untrusted-workload-runtime",
		Root:   "",
	}

	defaultRuntime := criconfig.Runtime{
		Type:   "io.containerd.runtime.v1.linux",
		Engine: "default-runtime",
		Root:   "",
	}

	fooRuntime := criconfig.Runtime{
		Type:   "io.containerd.runtime.v1.linux",
		Engine: "foo-bar",
		Root:   "",
	}

	for desc, test := range map[string]struct {
		sandboxConfig            *runtime.PodSandboxConfig
		runtimeHandler           string
		defaultRuntime           criconfig.Runtime
		untrustedWorkloadRuntime criconfig.Runtime
		runtimes                 map[string]criconfig.Runtime
		expectErr                bool
		expectedRuntime          criconfig.Runtime
	}{
		"should return error if untrusted workload requires host access": {
			sandboxConfig: &runtime.PodSandboxConfig{
				Linux: &runtime.LinuxPodSandboxConfig{
					SecurityContext: &runtime.LinuxSandboxSecurityContext{
						Privileged: false,
						NamespaceOptions: &runtime.NamespaceOption{
							Network: runtime.NamespaceMode_NODE,
							Pid:     runtime.NamespaceMode_NODE,
							Ipc:     runtime.NamespaceMode_NODE,
						},
					},
				},
				Annotations: map[string]string{
					annotations.UntrustedWorkload: "true",
				},
			},
			defaultRuntime:           defaultRuntime,
			untrustedWorkloadRuntime: untrustedWorkloadRuntime,
			expectErr:                true,
		},
		"should use untrusted workload runtime for untrusted workload": {
			sandboxConfig: &runtime.PodSandboxConfig{
				Annotations: map[string]string{
					annotations.UntrustedWorkload: "true",
				},
			},
			defaultRuntime:           defaultRuntime,
			untrustedWorkloadRuntime: untrustedWorkloadRuntime,
			expectedRuntime:          untrustedWorkloadRuntime,
		},
		"should use default runtime for regular workload": {
			sandboxConfig:            &runtime.PodSandboxConfig{},
			defaultRuntime:           defaultRuntime,
			untrustedWorkloadRuntime: untrustedWorkloadRuntime,
			expectedRuntime:          defaultRuntime,
		},
		"should use default runtime for trusted workload": {
			sandboxConfig: &runtime.PodSandboxConfig{
				Annotations: map[string]string{
					annotations.UntrustedWorkload: "false",
				},
			},
			defaultRuntime:           defaultRuntime,
			untrustedWorkloadRuntime: untrustedWorkloadRuntime,
			expectedRuntime:          defaultRuntime,
		},
		"should return error if untrusted workload runtime is required but not configured": {
			sandboxConfig: &runtime.PodSandboxConfig{
				Annotations: map[string]string{
					annotations.UntrustedWorkload: "true",
				},
			},
			defaultRuntime: defaultRuntime,
			expectErr:      true,
		},
		"should use 'untrusted' runtime for untrusted workload": {
			sandboxConfig: &runtime.PodSandboxConfig{
				Annotations: map[string]string{
					annotations.UntrustedWorkload: "true",
				},
			},
			defaultRuntime:  defaultRuntime,
			runtimes:        map[string]criconfig.Runtime{criconfig.RuntimeUntrusted: untrustedWorkloadRuntime},
			expectedRuntime: untrustedWorkloadRuntime,
		},
		"should use 'untrusted' runtime for untrusted workload & handler": {
			sandboxConfig: &runtime.PodSandboxConfig{
				Annotations: map[string]string{
					annotations.UntrustedWorkload: "true",
				},
			},
			runtimeHandler:  "untrusted",
			defaultRuntime:  defaultRuntime,
			runtimes:        map[string]criconfig.Runtime{criconfig.RuntimeUntrusted: untrustedWorkloadRuntime},
			expectedRuntime: untrustedWorkloadRuntime,
		},
		"should return an error if untrusted annotation with conflicting handler": {
			sandboxConfig: &runtime.PodSandboxConfig{
				Annotations: map[string]string{
					annotations.UntrustedWorkload: "true",
				},
			},
			runtimeHandler:           "foo",
			defaultRuntime:           defaultRuntime,
			untrustedWorkloadRuntime: untrustedWorkloadRuntime,
			runtimes:                 map[string]criconfig.Runtime{"foo": fooRuntime},
			expectErr:                true,
		},
		"should use correct runtime for a runtime handler": {
			sandboxConfig:            &runtime.PodSandboxConfig{},
			runtimeHandler:           "foo",
			defaultRuntime:           defaultRuntime,
			untrustedWorkloadRuntime: untrustedWorkloadRuntime,
			runtimes:                 map[string]criconfig.Runtime{"foo": fooRuntime},
			expectedRuntime:          fooRuntime,
		},
		"should return error if runtime handler is required but not configured": {
			sandboxConfig:  &runtime.PodSandboxConfig{},
			runtimeHandler: "bar",
			defaultRuntime: defaultRuntime,
			runtimes:       map[string]criconfig.Runtime{"foo": fooRuntime},
			expectErr:      true,
		},
	} {
		t.Run(desc, func(t *testing.T) {
			cri := newTestCRIService()
			cri.config = criconfig.Config{
				PluginConfig: criconfig.DefaultConfig(),
			}
			cri.config.ContainerdConfig.DefaultRuntime = test.defaultRuntime
			cri.config.ContainerdConfig.UntrustedWorkloadRuntime = test.untrustedWorkloadRuntime
			cri.config.ContainerdConfig.Runtimes = test.runtimes
			r, err := cri.getSandboxRuntime(test.sandboxConfig, test.runtimeHandler)
			assert.Equal(t, test.expectErr, err != nil)
			assert.Equal(t, test.expectedRuntime, r)
		})
	}
}

func TestSandboxDisableCgroup(t *testing.T) {
	config, imageConfig, _ := getRunPodSandboxTestData()
	c := newTestCRIService()
	c.config.DisableCgroup = true
	spec, err := c.generateSandboxContainerSpec("test-id", config, imageConfig, "test-cni")
	require.NoError(t, err)

	t.Log("resource limit should not be set")
	assert.Nil(t, spec.Linux.Resources.Memory)
	assert.Nil(t, spec.Linux.Resources.CPU)

	t.Log("cgroup path should be empty")
	assert.Empty(t, spec.Linux.CgroupsPath)
}

// TODO(random-liu): [P1] Add unit test for different error cases to make sure
// the function cleans up on error properly.

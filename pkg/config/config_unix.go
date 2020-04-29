// +build !windows

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

package config

import (
	"path/filepath"

	"github.com/containerd/containerd"
	"k8s.io/kubernetes/pkg/kubelet/server/streaming"

	"github.com/containerd/cri/pkg/constants"
)

var (
	// DefaultNetworkPluginBinDir is the default CNI directory for binaries
	DefaultNetworkPluginBinDir = "/opt/cni/bin"
	// DefaultNetworkPluginConfDir is the default CNI directory for configuration
	DefaultNetworkPluginConfDir = "/etc/cni/net.d"
)

// DefaultConfig returns default configurations of cri plugin.
func DefaultConfig() PluginConfig {
	return PluginConfig{
		CniConfig: CniConfig{
			NetworkPluginBinDir:       DefaultNetworkPluginBinDir,
			NetworkPluginConfDir:      DefaultNetworkPluginConfDir,
			NetworkPluginMaxConfNum:   1, // only one CNI plugin config file will be loaded
			NetworkPluginConfTemplate: "",
		},
		ContainerdConfig: ContainerdConfig{
			Snapshotter:        containerd.DefaultSnapshotter,
			DefaultRuntimeName: "runc",
			NoPivot:            false,
			Runtimes: map[string]Runtime{
				"runc": {
					Type: "io.containerd.runc.v2",
				},
			},
		},
		DisableTCPService:   true,
		StreamServerAddress: "127.0.0.1",
		StreamServerPort:    "0",
		StreamIdleTimeout:   streaming.DefaultConfig.StreamIdleTimeout.String(), // 4 hour
		EnableSelinux:       false,
		EnableTLSStreaming:  false,
		X509KeyPairStreaming: X509KeyPairStreaming{
			TLSKeyFile:  "",
			TLSCertFile: "",
		},
		SandboxImage:            "k8s.gcr.io/pause:3.2",
		StatsCollectPeriod:      10,
		SystemdCgroup:           false,
		MaxContainerLogLineSize: 16 * 1024,
		Registry: Registry{
			Mirrors: map[string]Mirror{
				"docker.io": {
					Endpoints: []string{"https://registry-1.docker.io"},
				},
			},
		},
		MaxConcurrentDownloads: 3,
		DisableProcMount:       false,
	}
}

// DefaultServiceConfig returns default configurations for a namespace.
func DefaultServiceConfig(ns string) PluginConfig {
	config := DefaultConfig()
	if ns != constants.K8sContainerdNamespace {
		config.NetworkPluginConfDir = filepath.Join("/opt/cri", ns, DefaultNetworkPluginConfDir)
	}
	return config
}

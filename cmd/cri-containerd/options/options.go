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

package options

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/containerd/containerd"
	"github.com/spf13/pflag"
)

const (
	// configFilePathArgName is the path to the config file.
	configFilePathArgName = "config"
	// defaultConfigFilePath is the default config file path.
	defaultConfigFilePath = "/etc/cri-containerd/config.toml"
)

// ContainerdConfig contains toml config related to containerd
type ContainerdConfig struct {
	// RootDir is the root directory path for containerd.
	// TODO(random-liu): Remove this field when no longer support cri-containerd standalone mode.
	RootDir string `toml:"root_dir" json:"rootDir,omitempty"`
	// Snapshotter is the snapshotter used by containerd.
	Snapshotter string `toml:"snapshotter" json:"snapshotter,omitempty"`
	// Endpoint is the containerd endpoint path.
	// TODO(random-liu): Remove this field when no longer support cri-containerd standalone mode.
	Endpoint string `toml:"endpoint" json:"endpoint,omitempty"`
	// Runtime is the runtime to use in containerd. We may support
	// other runtimes in the future.
	Runtime string `toml:"runtime" json:"runtime,omitempty"`
	// RuntimeEngine is the name of the runtime engine used by containerd.
	// Containerd default should be "runc"
	// We may support other runtime engines in the future.
	RuntimeEngine string `toml:"runtime_engine" json:"runtimeEngine,omitempty"`
	// RuntimeRoot is the directory used by containerd for runtime state.
	// Containerd default should be "/run/containerd/runc"
	RuntimeRoot string `toml:"runtime_root" json:"runtimeRoot,omitempty"`
}

// CniConfig contains toml config related to cni
type CniConfig struct {
	// NetworkPluginBinDir is the directory in which the binaries for the plugin is kept.
	NetworkPluginBinDir string `toml:"bin_dir" json:"binDir,omitempty"`
	// NetworkPluginConfDir is the directory in which the admin places a CNI conf.
	NetworkPluginConfDir string `toml:"conf_dir" json:"confDir,omitempty"`
}

// PluginConfig contains toml config related to CRI plugin,
// it is a subset of Config.
type PluginConfig struct {
	// ContainerdConfig contains config related to containerd
	ContainerdConfig `toml:"containerd" json:"containerd,omitempty"`
	// CniConfig contains config related to cni
	CniConfig `toml:"cni" json:"cni,omitempty"`
	// Registry contains config related to the registry
	Registry `toml:"registry" json:"registry,omitempty"`
	// StreamServerAddress is the ip address streaming server is listening on.
	StreamServerAddress string `toml:"stream_server_address" json:"streamServerAddress,omitempty"`
	// StreamServerPort is the port streaming server is listening on.
	StreamServerPort string `toml:"stream_server_port" json:"streamServerPort,omitempty"`
	// EnableSelinux indicates to enable the selinux support.
	EnableSelinux bool `toml:"enable_selinux" json:"enableSelinux,omitempty"`
	// SandboxImage is the image used by sandbox container.
	SandboxImage string `toml:"sandbox_image" json:"sandboxImage,omitempty"`
	// StatsCollectPeriod is the period (in seconds) of snapshots stats collection.
	StatsCollectPeriod int `toml:"stats_collect_period" json:"statsCollectPeriod,omitempty"`
	// SystemdCgroup enables systemd cgroup support.
	SystemdCgroup bool `toml:"systemd_cgroup" json:"systemdCgroup,omitempty"`
	// EnableIPv6DAD enables IPv6 DAD.
	// TODO(random-liu): Use optimistic_dad when it's GA.
	EnableIPv6DAD bool `toml:"enable_ipv6_dad" json:"enableIPv6DAD,omitempty"`
}

// Config contains toml config related cri-containerd daemon.
// TODO(random-liu): Make this an internal config object when we no longer support cri-containerd
// standalone mode. At that time, we can clean this up.
type Config struct {
	// PluginConfig is the config for CRI plugin.
	PluginConfig
	// ContainerdRootDir is the root directory path for containerd.
	ContainerdRootDir string `toml:"-" json:"containerdRootDir,omitempty"`
	// ContainerdEndpoint is the containerd endpoint path.
	ContainerdEndpoint string `toml:"-" json:"containerdEndpoint,omitempty"`
	// SocketPath is the path to the socket which cri-containerd serves on.
	// TODO(random-liu): Remove SocketPath when no longer support cri-containerd
	// standalone mode.
	SocketPath string `toml:"socket_path" json:"socketPath,omitempty"`
	// RootDir is the root directory path for managing cri-containerd files
	// (metadata checkpoint etc.)
	RootDir string `toml:"root_dir" json:"rootDir,omitempty"`
	// TODO(random-liu): Remove following fields when we no longer support cri-containerd
	// standalone mode.
	// CgroupPath is the path for the cgroup that cri-containerd is placed in.
	CgroupPath string `toml:"cgroup_path" json:"cgroupPath,omitempty"`
	// OOMScore adjust the cri-containerd's oom score
	OOMScore int `toml:"oom_score" json:"oomScore,omitempty"`
	// EnableProfiling is used for enable profiling via host:port/debug/pprof/
	EnableProfiling bool `toml:"profiling" json:"enableProfiling,omitempty"`
	// ProfilingPort is the port for profiling via host:port/debug/pprof/
	ProfilingPort string `toml:"profiling_port" json:"profilingPort,omitempty"`
	// ProfilingAddress is address for profiling via host:port/debug/pprof/
	ProfilingAddress string `toml:"profiling_addr" json:"profilingAddress,omitempty"`
	// LogLevel is the logrus log level.
	LogLevel string `toml:"log_level" json:"logLevel,omitempty"`
}

// CRIContainerdOptions contains cri-containerd command line and toml options.
type CRIContainerdOptions struct {
	// Config contains cri-containerd toml config
	Config
	// ConfigFilePath is the path to the TOML config file.
	ConfigFilePath string `toml:"-"`
}

// NewCRIContainerdOptions returns a reference to CRIContainerdOptions
func NewCRIContainerdOptions() *CRIContainerdOptions {
	return &CRIContainerdOptions{}
}

// AddFlags adds cri-containerd command line options to pflag.
func (c *CRIContainerdOptions) AddFlags(fs *pflag.FlagSet) {
	defaults := DefaultConfig()
	fs.StringVar(&c.ConfigFilePath, configFilePathArgName,
		defaultConfigFilePath, "Path to the config file.")
	fs.StringVar(&c.LogLevel, "log-level",
		defaults.LogLevel, "Set the logging level [trace, debug, info, warn, error, fatal, panic].")
	fs.StringVar(&c.SocketPath, "socket-path",
		defaults.SocketPath, "Path to the socket which cri-containerd serves on.")
	fs.StringVar(&c.RootDir, "root-dir",
		defaults.RootDir, "Root directory path for cri-containerd managed files (metadata checkpoint etc).")
	fs.StringVar(&c.ContainerdRootDir, "containerd-root-dir",
		defaults.ContainerdRootDir, "Root directory path where containerd stores persistent data.")
	fs.StringVar(&c.ContainerdEndpoint, "containerd-endpoint",
		defaults.ContainerdEndpoint, "Path to the containerd endpoint.")
	fs.StringVar(&c.ContainerdConfig.Snapshotter, "containerd-snapshotter",
		defaults.ContainerdConfig.Snapshotter, "The snapshotter used by containerd.")
	fs.StringVar(&c.ContainerdConfig.Runtime, "containerd-runtime",
		defaults.ContainerdConfig.Runtime, "The runtime used by containerd.")
	fs.StringVar(&c.ContainerdConfig.RuntimeEngine, "containerd-runtime-engine",
		defaults.ContainerdConfig.RuntimeEngine, "Runtime engine used by containerd. Defaults to containerd's default if not specified.")
	fs.StringVar(&c.ContainerdConfig.RuntimeRoot, "containerd-runtime-root",
		defaults.ContainerdConfig.RuntimeRoot, "The directory used by containerd for runtime state. Defaults to containerd's default if not specified.")
	fs.StringVar(&c.NetworkPluginBinDir, "network-bin-dir",
		defaults.NetworkPluginBinDir, "The directory for putting network binaries.")
	fs.StringVar(&c.NetworkPluginConfDir, "network-conf-dir",
		defaults.NetworkPluginConfDir, "The directory for putting network plugin configuration files.")
	fs.StringVar(&c.StreamServerAddress, "stream-addr",
		defaults.StreamServerAddress, "The ip address streaming server is listening on. The default host interface is used if not specified.")
	fs.StringVar(&c.StreamServerPort, "stream-port",
		defaults.StreamServerPort, "The port streaming server is listening on.")
	fs.StringVar(&c.CgroupPath, "cgroup-path",
		defaults.CgroupPath, "The cgroup that cri-containerd is part of. Cri-containerd is not placed in a cgroup if none is specified.")
	fs.BoolVar(&c.EnableSelinux, "enable-selinux",
		defaults.EnableSelinux, "Enable selinux support. By default not enabled.")
	fs.StringVar(&c.SandboxImage, "sandbox-image",
		defaults.SandboxImage, "The image used by sandbox container.")
	fs.IntVar(&c.StatsCollectPeriod, "stats-collect-period",
		defaults.StatsCollectPeriod, "The period (in seconds) of snapshots stats collection.")
	fs.BoolVar(&c.SystemdCgroup, "systemd-cgroup",
		defaults.SystemdCgroup, "Enables systemd cgroup support. By default not enabled.")
	fs.IntVar(&c.OOMScore, "oom-score",
		defaults.OOMScore, "Adjust the cri-containerd's oom score.")
	fs.BoolVar(&c.EnableProfiling, "profiling",
		defaults.EnableProfiling, "Enable profiling via web interface host:port/debug/pprof/.")
	fs.StringVar(&c.ProfilingPort, "profiling-port",
		defaults.ProfilingPort, "Profiling port for web interface host:port/debug/pprof/.")
	fs.StringVar(&c.ProfilingAddress, "profiling-addr",
		defaults.ProfilingAddress, "Profiling address for web interface host:port/debug/pprof/.")
	fs.BoolVar(&c.EnableIPv6DAD, "enable-ipv6-dad",
		defaults.EnableIPv6DAD, "Enable IPv6 DAD (duplicate address detection) for pod sandbox network. Enabling this will increase pod sandbox start latency by several seconds.")
	fs.Var(&c.Registry, "registry",
		"Registry config for image pull eg --registry=myregistry.io=https://mymirror.io/ --registry=myregistry2.io=https://mymirror2.io/")
}

// InitFlags load configurations from config file, and then overwrite with flags.
// This function must be called inside `Run`, at that time flags should have been
// parsed once.
// precedence:  commandline > configfile > default
func (c *CRIContainerdOptions) InitFlags(fs *pflag.FlagSet) error {
	// Load default config file if none provided
	if _, err := toml.DecodeFile(c.ConfigFilePath, &c.Config); err != nil {
		// the absence of default config file is normal case.
		if !fs.Changed(configFilePathArgName) && os.IsNotExist(err) {
			return nil
		}
		return err
	}
	// Add this for backward compatibility.
	// TODO(random-liu): Remove this when we no longer support cri-containerd standalone mode.
	c.ContainerdRootDir = c.ContainerdConfig.RootDir
	c.ContainerdEndpoint = c.ContainerdConfig.Endpoint

	// What is the reason for applying the command line twice?
	// Because the values from command line have the highest priority.
	// The path of toml configuration file if from the command line,
	// and triggers the first parse.
	// The first parse generates the default value and the value from command line at the same time.
	// But the priority of the toml config value is higher than the default value,
	// Without a way to insert the toml config value between the default value and the command line value.
	// We parse twice one for default value, one for commandline value.
	return fs.Parse(os.Args[1:])
}

// PrintDefaultTomlConfig print default toml config of cri-containerd.
func PrintDefaultTomlConfig() {
	if err := toml.NewEncoder(os.Stdout).Encode(DefaultConfig()); err != nil {
		fmt.Println(err)
		return
	}
}

// DefaultConfig returns default configurations of cri-containerd.
func DefaultConfig() Config {
	return Config{
		PluginConfig: PluginConfig{
			CniConfig: CniConfig{
				NetworkPluginBinDir:  "/opt/cni/bin",
				NetworkPluginConfDir: "/etc/cni/net.d",
			},
			ContainerdConfig: ContainerdConfig{
				Snapshotter:   containerd.DefaultSnapshotter,
				Runtime:       "io.containerd.runtime.v1.linux",
				RuntimeEngine: "",
				RuntimeRoot:   "",
				RootDir:       "/var/lib/containerd",
				Endpoint:      "/run/containerd/containerd.sock",
			},
			StreamServerAddress: "",
			StreamServerPort:    "10010",
			EnableSelinux:       false,
			SandboxImage:        "gcr.io/google_containers/pause:3.0",
			StatsCollectPeriod:  10,
			SystemdCgroup:       false,
			EnableIPv6DAD:       false,
			Registry: Registry{
				Mirrors: map[string]Mirror{
					"docker.io": {
						Endpoints: []string{"https://registry-1.docker.io"},
					},
				},
			},
		},
		ContainerdRootDir:  "/var/lib/containerd",
		ContainerdEndpoint: "/run/containerd/containerd.sock",
		SocketPath:         "/var/run/cri-containerd.sock",
		RootDir:            "/var/lib/cri-containerd",
		CgroupPath:         "",
		OOMScore:           -999,
		EnableProfiling:    true,
		ProfilingPort:      "10011",
		ProfilingAddress:   "127.0.0.1",
		LogLevel:           "info",
	}
}

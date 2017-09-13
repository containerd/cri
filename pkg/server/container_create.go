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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/contrib/apparmor"
	"github.com/golang/glog"
	imagespec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opencontainers/runc/libcontainer/devices"
	runtimespec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	"github.com/opencontainers/runtime-tools/validate"
	"github.com/opencontainers/selinux/go-selinux/label"
	"github.com/syndtr/gocapability/capability"
	"golang.org/x/net/context"
	"golang.org/x/sys/unix"
	"k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1/runtime"

	customopts "github.com/kubernetes-incubator/cri-containerd/pkg/opts"
	cio "github.com/kubernetes-incubator/cri-containerd/pkg/server/io"
	containerstore "github.com/kubernetes-incubator/cri-containerd/pkg/store/container"
	"github.com/kubernetes-incubator/cri-containerd/pkg/util"
)

const (
	// profileNamePrefix is the prefix for loading profiles on a localhost. Eg. AppArmor localhost/profileName.
	profileNamePrefix = "localhost/" // TODO (mikebrow): get localhost/ & runtime/default from CRI kubernetes/kubernetes#51747
	// runtimeDefault indicates that we should use or create a runtime default apparmor profile.
	runtimeDefault = "runtime/default"
	// appArmorDefaultProfileName is name to use when creating a default apparmor profile.
	appArmorDefaultProfileName = "cri-containerd.apparmor.d"
	// appArmorEnabled is a flag for globally enabling/disabling apparmor profiles for containers.
	appArmorEnabled = true // TODO (mikebrow): make these apparmor defaults configurable
)

// CreateContainer creates a new container in the given PodSandbox.
func (c *criContainerdService) CreateContainer(ctx context.Context, r *runtime.CreateContainerRequest) (_ *runtime.CreateContainerResponse, retErr error) {
	config := r.GetConfig()
	sandboxConfig := r.GetSandboxConfig()
	sandbox, err := c.sandboxStore.Get(r.GetPodSandboxId())
	if err != nil {
		return nil, fmt.Errorf("failed to find sandbox id %q: %v", r.GetPodSandboxId(), err)
	}
	sandboxID := sandbox.ID
	s, err := sandbox.Container.Task(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get sandbox container task: %v", err)
	}
	sandboxPid := s.Pid()

	// Generate unique id and name for the container and reserve the name.
	// Reserve the container name to avoid concurrent `CreateContainer` request creating
	// the same container.
	id := util.GenerateID()
	name := makeContainerName(config.GetMetadata(), sandboxConfig.GetMetadata())
	if err = c.containerNameIndex.Reserve(name, id); err != nil {
		return nil, fmt.Errorf("failed to reserve container name %q: %v", name, err)
	}
	defer func() {
		// Release the name if the function returns with an error.
		if retErr != nil {
			c.containerNameIndex.ReleaseByName(name)
		}
	}()

	// Create initial internal container metadata.
	meta := containerstore.Metadata{
		ID:        id,
		Name:      name,
		SandboxID: sandboxID,
		Config:    config,
	}

	// Prepare container image snapshot. For container, the image should have
	// been pulled before creating the container, so do not ensure the image.
	imageRef := config.GetImage().GetImage()
	image, err := c.localResolve(ctx, imageRef)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve image %q: %v", imageRef, err)
	}
	if image == nil {
		return nil, fmt.Errorf("image %q not found", imageRef)
	}

	// Create container root directory.
	containerRootDir := getContainerRootDir(c.rootDir, id)
	if err = c.os.MkdirAll(containerRootDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create container root directory %q: %v",
			containerRootDir, err)
	}
	defer func() {
		if retErr != nil {
			// Cleanup the container root directory.
			if err = c.os.RemoveAll(containerRootDir); err != nil {
				glog.Errorf("Failed to remove container root directory %q: %v",
					containerRootDir, err)
			}
		}
	}()

	// Create container volumes mounts.
	// TODO(random-liu): Add cri-containerd integration test for image volume.
	volumeMounts := c.generateVolumeMounts(containerRootDir, config.GetMounts(), image.Config)

	// Generate container runtime spec.
	mounts := c.generateContainerMounts(getSandboxRootDir(c.rootDir, sandboxID), config)

	spec, err := c.generateContainerSpec(id, sandboxPid, config, sandboxConfig, image.Config, append(mounts, volumeMounts...))
	if err != nil {
		return nil, fmt.Errorf("failed to generate container %q spec: %v", id, err)
	}
	glog.V(4).Infof("Container spec: %+v", spec)

	// Set snapshotter before any other options.
	opts := []containerd.NewContainerOpts{
		containerd.WithSnapshotter(c.snapshotter),
		// Prepare container rootfs. This is always writeable even if
		// the container wants a readonly rootfs since we want to give
		// the runtime (runc) a chance to modify (e.g. to create mount
		// points corresponding to spec.Mounts) before making the
		// rootfs readonly (requested by spec.Root.Readonly).
		containerd.WithNewSnapshot(id, image.Image),
	}

	if len(volumeMounts) > 0 {
		mountMap := make(map[string]string)
		for _, v := range volumeMounts {
			mountMap[v.HostPath] = v.ContainerPath
		}
		opts = append(opts, customopts.WithVolumes(mountMap))
	}
	meta.ImageRef = image.ID

	containerIO, err := cio.NewContainerIO(id,
		cio.WithStdin(config.GetStdin()),
		cio.WithTerminal(config.GetTty()),
		cio.WithRootDir(containerRootDir),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create container io: %v", err)
	}
	defer func() {
		if retErr != nil {
			if err := containerIO.Close(); err != nil {
				glog.Errorf("Failed to close container io %q : %v", id, err)
			}
		}
	}()

	metaBytes, err := meta.Encode()
	if err != nil {
		return nil, fmt.Errorf("failed to convert sandbox metadata: %+v, %v", meta, err)
	}
	labels := map[string]string{
		containerMetadataLabel: string(metaBytes),
	}

	var specOpts []containerd.SpecOpts
	// Set container username. This could only be done by containerd, because it needs
	// access to the container rootfs. Pass user name to containerd, and let it overwrite
	// the spec for us.
	if uid := config.GetLinux().GetSecurityContext().GetRunAsUser(); uid != nil {
		specOpts = append(specOpts, containerd.WithUserID(uint32(uid.GetValue())))
	}
	if username := config.GetLinux().GetSecurityContext().GetRunAsUsername(); username != "" {
		specOpts = append(specOpts, containerd.WithUsername(username))
	}
	// Set apparmor profile, (privileged or not) if apparmor is enabled
	if appArmorEnabled {
		appArmorProf := config.GetLinux().GetSecurityContext().GetApparmorProfile()
		switch appArmorProf {
		case runtimeDefault:
			// TODO (mikebrow): delete created apparmor default profile
			specOpts = append(specOpts, apparmor.WithDefaultProfile(appArmorDefaultProfileName))
		case "":
			// TODO (mikebrow): handle no apparmor profile case see kubernetes/kubernetes#51746
		default:
			// Require and Trim default profile name prefix
			if !strings.HasPrefix(appArmorProf, profileNamePrefix) {
				return nil, fmt.Errorf("invalid apparmor profile %q", appArmorProf)
			}
			specOpts = append(specOpts, apparmor.WithProfile(strings.TrimPrefix(appArmorProf, profileNamePrefix)))
		}
	}
	opts = append(opts,
		containerd.WithSpec(spec, specOpts...),
		containerd.WithRuntime(defaultRuntime, nil),
		containerd.WithContainerLabels(labels))
	var cntr containerd.Container
	if cntr, err = c.client.NewContainer(ctx, id, opts...); err != nil {
		return nil, fmt.Errorf("failed to create containerd container: %v", err)
	}
	defer func() {
		if retErr != nil {
			if err := cntr.Delete(ctx, containerd.WithSnapshotCleanup); err != nil {
				glog.Errorf("Failed to delete containerd container %q: %v", id, err)
			}
		}
	}()

	status := containerstore.Status{CreatedAt: time.Now().UnixNano()}
	container, err := containerstore.NewContainer(meta,
		containerstore.WithStatus(status, containerRootDir),
		containerstore.WithContainer(cntr),
		containerstore.WithContainerIO(containerIO),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create internal container object for %q: %v",
			id, err)
	}
	defer func() {
		if retErr != nil {
			// Cleanup container checkpoint on error.
			if err := container.Delete(); err != nil {
				glog.Errorf("Failed to cleanup container checkpoint for %q: %v", id, err)
			}
		}
	}()

	// Add container into container store.
	if err := c.containerStore.Add(container); err != nil {
		return nil, fmt.Errorf("failed to add container %q into store: %v", id, err)
	}

	return &runtime.CreateContainerResponse{ContainerId: id}, nil
}

func (c *criContainerdService) generateContainerSpec(id string, sandboxPid uint32, config *runtime.ContainerConfig,
	sandboxConfig *runtime.PodSandboxConfig, imageConfig *imagespec.ImageConfig, extraMounts []*runtime.Mount) (*runtimespec.Spec, error) {
	// Creates a spec Generator with the default spec.
	spec, err := defaultRuntimeSpec()
	if err != nil {
		return nil, err
	}
	g := generate.NewFromSpec(spec)

	// Set the relative path to the rootfs of the container from containerd's
	// pre-defined directory.
	g.SetRootPath(relativeRootfsPath)

	if err := setOCIProcessArgs(&g, config, imageConfig); err != nil {
		return nil, err
	}

	if config.GetWorkingDir() != "" {
		g.SetProcessCwd(config.GetWorkingDir())
	} else if imageConfig.WorkingDir != "" {
		g.SetProcessCwd(imageConfig.WorkingDir)
	}

	g.SetProcessTerminal(config.GetTty())
	if config.GetTty() {
		g.AddProcessEnv("TERM", "xterm")
	}

	// Apply envs from image config first, so that envs from container config
	// can override them.
	if err := addImageEnvs(&g, imageConfig.Env); err != nil {
		return nil, err
	}
	for _, e := range config.GetEnvs() {
		g.AddProcessEnv(e.GetKey(), e.GetValue())
	}

	securityContext := config.GetLinux().GetSecurityContext()
	selinuxOpt := securityContext.GetSelinuxOptions()
	processLabel, mountLabel, err := initSelinuxOpts(selinuxOpt)
	if err != nil {
		return nil, fmt.Errorf("failed to init selinux options %+v: %v", securityContext.GetSelinuxOptions(), err)
	}

	// Add extra mounts first so that CRI specified mounts can override.
	mounts := append(extraMounts, config.GetMounts()...)
	if err := c.addOCIBindMounts(&g, mounts, mountLabel); err != nil {
		return nil, fmt.Errorf("failed to set OCI bind mounts %+v: %v", mounts, err)
	}

	if securityContext.GetPrivileged() {
		if !sandboxConfig.GetLinux().GetSecurityContext().GetPrivileged() {
			return nil, fmt.Errorf("no privileged container allowed in sandbox")
		}
		if err := setOCIPrivileged(&g, config); err != nil {
			return nil, err
		}
	} else {
		if err := addOCIDevices(&g, config.GetDevices()); err != nil {
			return nil, fmt.Errorf("failed to set devices mapping %+v: %v", config.GetDevices(), err)
		}

		if err := setOCICapabilities(&g, securityContext.GetCapabilities()); err != nil {
			return nil, fmt.Errorf("failed to set capabilities %+v: %v",
				securityContext.GetCapabilities(), err)
		}
		// TODO(random-liu): [P2] Add seccomp not privileged only.
	}

	g.SetProcessSelinuxLabel(processLabel)
	g.SetLinuxMountLabel(mountLabel)

	// TODO: Figure out whether we should set no new privilege for sandbox container by default
	g.SetProcessNoNewPrivileges(securityContext.GetNoNewPrivs())

	// TODO(random-liu): [P1] Set selinux options (privileged or not).

	g.SetRootReadonly(securityContext.GetReadonlyRootfs())

	setOCILinuxResource(&g, config.GetLinux().GetResources())

	if sandboxConfig.GetLinux().GetCgroupParent() != "" {
		cgroupsPath := getCgroupsPath(sandboxConfig.GetLinux().GetCgroupParent(), id)
		g.SetLinuxCgroupsPath(cgroupsPath)
	}

	// Set namespaces, share namespace with sandbox container.
	setOCINamespaces(&g, securityContext.GetNamespaceOptions(), sandboxPid)

	supplementalGroups := securityContext.GetSupplementalGroups()
	for _, group := range supplementalGroups {
		g.AddProcessAdditionalGid(uint32(group))
	}

	return g.Spec(), nil
}

// generateVolumeMounts sets up image volumes for container. Rely on the removal of container
// root directory to do cleanup. Note that image volume will be skipped, if there is criMounts
// specified with the same destination.
func (c *criContainerdService) generateVolumeMounts(containerRootDir string, criMounts []*runtime.Mount, config *imagespec.ImageConfig) []*runtime.Mount {
	if len(config.Volumes) == 0 {
		return nil
	}
	var mounts []*runtime.Mount
	for dst := range config.Volumes {
		if isInCRIMounts(dst, criMounts) {
			// Skip the image volume, if there is CRI defined volume mapping.
			// TODO(random-liu): This should be handled by Kubelet in the future.
			// Kubelet should decide what to use for image volume, and also de-duplicate
			// the image volume and user mounts.
			continue
		}
		volumeID := util.GenerateID()
		src := filepath.Join(containerRootDir, "volumes", volumeID)
		// addOCIBindMounts will create these volumes.
		mounts = append(mounts, &runtime.Mount{
			ContainerPath: dst,
			HostPath:      src,
			// Use default mount propagation.
			// TODO(random-liu): What about selinux relabel?
		})
	}
	return mounts
}

// generateContainerMounts sets up necessary container mounts including /dev/shm, /etc/hosts
// and /etc/resolv.conf.
func (c *criContainerdService) generateContainerMounts(sandboxRootDir string, config *runtime.ContainerConfig) []*runtime.Mount {
	var mounts []*runtime.Mount
	securityContext := config.GetLinux().GetSecurityContext()
	if !isInCRIMounts(etcHosts, config.GetMounts()) {
		mounts = append(mounts, &runtime.Mount{
			ContainerPath: etcHosts,
			HostPath:      getSandboxHosts(sandboxRootDir),
			Readonly:      securityContext.GetReadonlyRootfs(),
		})
	}

	// Mount sandbox resolv.config.
	// TODO: Need to figure out whether we should always mount it as read-only
	if !isInCRIMounts(resolvConfPath, config.GetMounts()) {
		mounts = append(mounts, &runtime.Mount{
			ContainerPath: resolvConfPath,
			HostPath:      getResolvPath(sandboxRootDir),
			Readonly:      securityContext.GetReadonlyRootfs(),
		})
	}

	if !isInCRIMounts(devShm, config.GetMounts()) {
		sandboxDevShm := getSandboxDevShm(sandboxRootDir)
		if securityContext.GetNamespaceOptions().GetHostIpc() {
			sandboxDevShm = devShm
		}
		mounts = append(mounts, &runtime.Mount{
			ContainerPath: devShm,
			HostPath:      sandboxDevShm,
			Readonly:      false,
		})
	}
	return mounts
}

// setOCIProcessArgs sets process args. It returns error if the final arg list
// is empty.
func setOCIProcessArgs(g *generate.Generator, config *runtime.ContainerConfig, imageConfig *imagespec.ImageConfig) error {
	command, args := config.GetCommand(), config.GetArgs()
	// The following logic is migrated from https://github.com/moby/moby/blob/master/daemon/commit.go
	// TODO(random-liu): Clearly define the commands overwrite behavior.
	if len(command) == 0 {
		if len(args) == 0 {
			args = imageConfig.Cmd
		}
		if command == nil {
			command = imageConfig.Entrypoint
		}
	}
	if len(command) == 0 && len(args) == 0 {
		return fmt.Errorf("no command specified")
	}
	g.SetProcessArgs(append(command, args...))
	return nil
}

// addImageEnvs adds environment variables from image config. It returns error if
// an invalid environment variable is encountered.
func addImageEnvs(g *generate.Generator, imageEnvs []string) error {
	for _, e := range imageEnvs {
		kv := strings.SplitN(e, "=", 2)
		if len(kv) != 2 {
			return fmt.Errorf("invalid environment variable %q", e)
		}
		g.AddProcessEnv(kv[0], kv[1])
	}
	return nil
}

func setOCIPrivileged(g *generate.Generator, config *runtime.ContainerConfig) error {
	// Add all capabilities in privileged mode.
	g.SetupPrivileged(true)
	setOCIBindMountsPrivileged(g)
	if err := setOCIDevicesPrivileged(g); err != nil {
		return fmt.Errorf("failed to set devices mapping %+v: %v", config.GetDevices(), err)
	}
	return nil
}

func clearReadOnly(m *runtimespec.Mount) {
	var opt []string
	for _, o := range m.Options {
		if o != "ro" {
			opt = append(opt, o)
		}
	}
	m.Options = opt
}

// addDevices set device mapping without privilege.
func addOCIDevices(g *generate.Generator, devs []*runtime.Device) error {
	spec := g.Spec()
	for _, device := range devs {
		path, err := resolveSymbolicLink(device.HostPath)
		if err != nil {
			return err
		}
		dev, err := devices.DeviceFromPath(path, device.Permissions)
		if err != nil {
			return err
		}
		rd := runtimespec.LinuxDevice{
			Path:  device.ContainerPath,
			Type:  string(dev.Type),
			Major: dev.Major,
			Minor: dev.Minor,
			UID:   &dev.Uid,
			GID:   &dev.Gid,
		}
		g.AddDevice(rd)
		spec.Linux.Resources.Devices = append(spec.Linux.Resources.Devices, runtimespec.LinuxDeviceCgroup{
			Allow:  true,
			Type:   string(dev.Type),
			Major:  &dev.Major,
			Minor:  &dev.Minor,
			Access: dev.Permissions,
		})
	}
	return nil
}

// addDevices set device mapping with privilege.
func setOCIDevicesPrivileged(g *generate.Generator) error {
	spec := g.Spec()
	hostDevices, err := devices.HostDevices()
	if err != nil {
		return err
	}
	for _, hostDevice := range hostDevices {
		rd := runtimespec.LinuxDevice{
			Path:  hostDevice.Path,
			Type:  string(hostDevice.Type),
			Major: hostDevice.Major,
			Minor: hostDevice.Minor,
			UID:   &hostDevice.Uid,
			GID:   &hostDevice.Gid,
		}
		if hostDevice.Major == 0 && hostDevice.Minor == 0 {
			// Invalid device, most likely a symbolic link, skip it.
			continue
		}
		g.AddDevice(rd)
	}
	spec.Linux.Resources.Devices = []runtimespec.LinuxDeviceCgroup{
		{
			Allow:  true,
			Access: "rwm",
		},
	}
	return nil
}

// addOCIBindMounts adds bind mounts.
// TODO(random-liu): Figure out whether we need to change all CRI mounts to readonly when
// rootfs is readonly. (https://github.com/moby/moby/blob/master/daemon/oci_linux.go)
func (c *criContainerdService) addOCIBindMounts(g *generate.Generator, mounts []*runtime.Mount, mountLabel string) error {
	// Mount cgroup into the container as readonly, which inherits docker's behavior.
	g.AddCgroupsMount("ro") // nolint: errcheck
	for _, mount := range mounts {
		dst := mount.GetContainerPath()
		src := mount.GetHostPath()
		// Create the host path if it doesn't exist.
		// TODO(random-liu): Add CRI validation test for this case.
		if _, err := c.os.Stat(src); err != nil {
			if !os.IsNotExist(err) {
				return fmt.Errorf("failed to stat %q: %v", src, err)
			}
			if err := c.os.MkdirAll(src, 0755); err != nil {
				return fmt.Errorf("failed to mkdir %q: %v", src, err)
			}
		}
		options := []string{"rbind"}
		switch mount.GetPropagation() {
		case runtime.MountPropagation_PROPAGATION_PRIVATE:
			options = append(options, "rprivate")
		case runtime.MountPropagation_PROPAGATION_BIDIRECTIONAL:
			options = append(options, "rshared")
		case runtime.MountPropagation_PROPAGATION_HOST_TO_CONTAINER:
			options = append(options, "rslave")
		default:
			glog.Warningf("Unknown propagation mode for hostPath %q", mount.HostPath)
			options = append(options, "rprivate")
		}

		if mount.GetReadonly() {
			options = append(options, "ro")
		} else {
			options = append(options, "rw")
		}

		if mount.GetSelinuxRelabel() {
			if err := label.Relabel(src, mountLabel, true); err != nil && err != unix.ENOTSUP {
				return fmt.Errorf("relabel %q with %q failed: %v", src, mountLabel, err)
			}
		}
		g.AddBindMount(src, dst, options)
	}

	return nil
}

func setOCIBindMountsPrivileged(g *generate.Generator) {
	spec := g.Spec()
	// clear readonly for /sys and cgroup
	for i, m := range spec.Mounts {
		if spec.Mounts[i].Destination == "/sys" && !spec.Root.Readonly {
			clearReadOnly(&spec.Mounts[i])
		}
		if m.Type == "cgroup" {
			clearReadOnly(&spec.Mounts[i])
		}
	}
	spec.Linux.ReadonlyPaths = nil
	spec.Linux.MaskedPaths = nil
}

// setOCILinuxResource set container resource limit.
func setOCILinuxResource(g *generate.Generator, resources *runtime.LinuxContainerResources) {
	if resources == nil {
		return
	}
	g.SetLinuxResourcesCPUPeriod(uint64(resources.GetCpuPeriod()))
	g.SetLinuxResourcesCPUQuota(resources.GetCpuQuota())
	g.SetLinuxResourcesCPUShares(uint64(resources.GetCpuShares()))
	g.SetLinuxResourcesMemoryLimit(resources.GetMemoryLimitInBytes())
	g.SetProcessOOMScoreAdj(int(resources.GetOomScoreAdj()))
}

// getOCICapabilitiesList returns a list of all available capabilities.
func getOCICapabilitiesList() []string {
	var caps []string
	for _, cap := range capability.List() {
		if cap > validate.LastCap() {
			continue
		}
		caps = append(caps, "CAP_"+strings.ToUpper(cap.String()))
	}
	return caps
}

// setOCICapabilities adds/drops process capabilities.
func setOCICapabilities(g *generate.Generator, capabilities *runtime.Capability) error {
	if capabilities == nil {
		return nil
	}

	// Add/drop all capabilities if "all" is specified, so that
	// following individual add/drop could still work. E.g.
	// AddCapabilities: []string{"ALL"}, DropCapabilities: []string{"CHOWN"}
	// will be all capabilities without `CAP_CHOWN`.
	if util.InStringSlice(capabilities.GetAddCapabilities(), "ALL") {
		for _, c := range getOCICapabilitiesList() {
			if err := g.AddProcessCapability(c); err != nil {
				return err
			}
		}
	}
	if util.InStringSlice(capabilities.GetDropCapabilities(), "ALL") {
		for _, c := range getOCICapabilitiesList() {
			if err := g.DropProcessCapability(c); err != nil {
				return err
			}
		}
	}

	for _, c := range capabilities.GetAddCapabilities() {
		if strings.ToUpper(c) == "ALL" {
			continue
		}
		// Capabilities in CRI doesn't have `CAP_` prefix, so add it.
		if err := g.AddProcessCapability("CAP_" + strings.ToUpper(c)); err != nil {
			return err
		}
	}

	for _, c := range capabilities.GetDropCapabilities() {
		if strings.ToUpper(c) == "ALL" {
			continue
		}
		if err := g.DropProcessCapability("CAP_" + strings.ToUpper(c)); err != nil {
			return err
		}
	}
	return nil
}

// setOCINamespaces sets namespaces.
func setOCINamespaces(g *generate.Generator, namespaces *runtime.NamespaceOption, sandboxPid uint32) {
	g.AddOrReplaceLinuxNamespace(string(runtimespec.NetworkNamespace), getNetworkNamespace(sandboxPid)) // nolint: errcheck
	g.AddOrReplaceLinuxNamespace(string(runtimespec.IPCNamespace), getIPCNamespace(sandboxPid))         // nolint: errcheck
	g.AddOrReplaceLinuxNamespace(string(runtimespec.UTSNamespace), getUTSNamespace(sandboxPid))         // nolint: errcheck
	// Do not share pid namespace for now.
	if namespaces.GetHostPid() {
		g.RemoveLinuxNamespace(string(runtimespec.PIDNamespace)) // nolint: errcheck
	}
}

// defaultRuntimeSpec returns a default runtime spec used in cri-containerd.
func defaultRuntimeSpec() (*runtimespec.Spec, error) {
	spec, err := containerd.GenerateSpec(context.Background(), nil, nil)
	if err != nil {
		return nil, err
	}

	// Remove `/run` mount
	// TODO(random-liu): Mount tmpfs for /run and handle copy-up.
	var mounts []runtimespec.Mount
	for _, mount := range spec.Mounts {
		if mount.Destination == "/run" {
			continue
		}
		mounts = append(mounts, mount)
	}
	spec.Mounts = mounts
	return spec, nil
}

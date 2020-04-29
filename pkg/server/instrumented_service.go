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
	"errors"

	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/log"
	"golang.org/x/net/context"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"

	ctrdutil "github.com/containerd/cri/pkg/containerd/util"
)

// checkInitialized returns error if the server is not fully initialized.
// GRPC service request handlers should return error before server is fully
// initialized.
// NOTE(random-liu): All following functions MUST check initialized at the beginning.
func (m *criServiceManager) checkInitialized(ctx context.Context) (context.Context, *criService, error) {
	ctx = ctrdutil.WithNamespace(ctx)
	ns, err := m.namespaceRequired(ctx)
	if err != nil {
		return ctx, nil, err
	}
	if ns.initialized.IsSet() {
		return ctx, ns, nil
	}
	return ctx, nil, errors.New("namespace not initialized")
}

func (m *criServiceManager) RunPodSandbox(ctx context.Context, r *runtime.RunPodSandboxRequest) (res *runtime.RunPodSandboxResponse, _ error) {
	ctx, c, err := m.checkInitialized(ctx)
	if err != nil {
		return nil, err
	}
	log.G(ctx).Infof("RunPodsandbox for %+v", r.GetConfig().GetMetadata())
	defer func() {
		if err != nil {
			log.G(ctx).WithError(err).Errorf("RunPodSandbox for %+v failed, error", r.GetConfig().GetMetadata())
		} else {
			log.G(ctx).Infof("RunPodSandbox for %+v returns sandbox id %q", r.GetConfig().GetMetadata(), res.GetPodSandboxId())
		}
	}()
	res, err = c.RunPodSandbox(ctx, r)
	return res, errdefs.ToGRPC(err)
}

func (m *criServiceManager) ListPodSandbox(ctx context.Context, r *runtime.ListPodSandboxRequest) (res *runtime.ListPodSandboxResponse, _ error) {
	ctx, c, err := m.checkInitialized(ctx)
	if err != nil {
		return nil, err
	}
	log.G(ctx).Tracef("ListPodSandbox with filter %+v", r.GetFilter())
	defer func() {
		if err != nil {
			log.G(ctx).WithError(err).Error("ListPodSandbox failed")
		} else {
			log.G(ctx).Tracef("ListPodSandbox returns pod sandboxes %+v", res.GetItems())
		}
	}()
	res, err = c.ListPodSandbox(ctx, r)
	return res, errdefs.ToGRPC(err)
}

func (m *criServiceManager) PodSandboxStatus(ctx context.Context, r *runtime.PodSandboxStatusRequest) (res *runtime.PodSandboxStatusResponse, _ error) {
	ctx, c, err := m.checkInitialized(ctx)
	if err != nil {
		return nil, err
	}
	log.G(ctx).Tracef("PodSandboxStatus for %q", r.GetPodSandboxId())
	defer func() {
		if err != nil {
			log.G(ctx).WithError(err).Errorf("PodSandboxStatus for %q failed", r.GetPodSandboxId())
		} else {
			log.G(ctx).Tracef("PodSandboxStatus for %q returns status %+v", r.GetPodSandboxId(), res.GetStatus())
		}
	}()
	res, err = c.PodSandboxStatus(ctx, r)
	return res, errdefs.ToGRPC(err)
}

func (m *criServiceManager) StopPodSandbox(ctx context.Context, r *runtime.StopPodSandboxRequest) (_ *runtime.StopPodSandboxResponse, _ error) {
	ctx, c, err := m.checkInitialized(ctx)
	if err != nil {
		return nil, err
	}
	log.G(ctx).Infof("StopPodSandbox for %q", r.GetPodSandboxId())
	defer func() {
		if err != nil {
			log.G(ctx).WithError(err).Errorf("StopPodSandbox for %q failed", r.GetPodSandboxId())
		} else {
			log.G(ctx).Infof("StopPodSandbox for %q returns successfully", r.GetPodSandboxId())
		}
	}()
	res, err := c.StopPodSandbox(ctx, r)
	return res, errdefs.ToGRPC(err)
}

func (m *criServiceManager) RemovePodSandbox(ctx context.Context, r *runtime.RemovePodSandboxRequest) (_ *runtime.RemovePodSandboxResponse, _ error) {
	ctx, c, err := m.checkInitialized(ctx)
	if err != nil {
		return nil, err
	}
	log.G(ctx).Infof("RemovePodSandbox for %q", r.GetPodSandboxId())
	defer func() {
		if err != nil {
			log.G(ctx).WithError(err).Errorf("RemovePodSandbox for %q failed", r.GetPodSandboxId())
		} else {
			log.G(ctx).Infof("RemovePodSandbox %q returns successfully", r.GetPodSandboxId())
		}
	}()
	res, err := c.RemovePodSandbox(ctx, r)
	return res, errdefs.ToGRPC(err)
}

func (m *criServiceManager) PortForward(ctx context.Context, r *runtime.PortForwardRequest) (res *runtime.PortForwardResponse, _ error) {
	ctx, c, err := m.checkInitialized(ctx)
	if err != nil {
		return nil, err
	}
	log.G(ctx).Infof("Portforward for %q port %v", r.GetPodSandboxId(), r.GetPort())
	defer func() {
		if err != nil {
			log.G(ctx).WithError(err).Errorf("Portforward for %q failed", r.GetPodSandboxId())
		} else {
			log.G(ctx).Infof("Portforward for %q returns URL %q", r.GetPodSandboxId(), res.GetUrl())
		}
	}()
	res, err = c.PortForward(ctx, r)
	return res, errdefs.ToGRPC(err)
}

func (m *criServiceManager) CreateContainer(ctx context.Context, r *runtime.CreateContainerRequest) (res *runtime.CreateContainerResponse, _ error) {
	ctx, c, err := m.checkInitialized(ctx)
	if err != nil {
		return nil, err
	}
	log.G(ctx).Infof("CreateContainer within sandbox %q for container %+v", r.GetPodSandboxId(), r.GetConfig().GetMetadata())
	defer func() {
		if err != nil {
			log.G(ctx).WithError(err).Errorf("CreateContainer within sandbox %q for %+v failed",
				r.GetPodSandboxId(), r.GetConfig().GetMetadata())
		} else {
			log.G(ctx).Infof("CreateContainer within sandbox %q for %+v returns container id %q",
				r.GetPodSandboxId(), r.GetConfig().GetMetadata(), res.GetContainerId())
		}
	}()
	res, err = c.CreateContainer(ctx, r)
	return res, errdefs.ToGRPC(err)
}

func (m *criServiceManager) StartContainer(ctx context.Context, r *runtime.StartContainerRequest) (_ *runtime.StartContainerResponse, _ error) {
	ctx, c, err := m.checkInitialized(ctx)
	if err != nil {
		return nil, err
	}
	log.G(ctx).Infof("StartContainer for %q", r.GetContainerId())
	defer func() {
		if err != nil {
			log.G(ctx).WithError(err).Errorf("StartContainer for %q failed", r.GetContainerId())
		} else {
			log.G(ctx).Infof("StartContainer for %q returns successfully", r.GetContainerId())
		}
	}()
	res, err := c.StartContainer(ctx, r)
	return res, errdefs.ToGRPC(err)
}

func (m *criServiceManager) ListContainers(ctx context.Context, r *runtime.ListContainersRequest) (res *runtime.ListContainersResponse, _ error) {
	ctx, c, err := m.checkInitialized(ctx)
	if err != nil {
		return nil, err
	}
	log.G(ctx).Tracef("ListContainers with filter %+v", r.GetFilter())
	defer func() {
		if err != nil {
			log.G(ctx).WithError(err).Errorf("ListContainers with filter %+v failed", r.GetFilter())
		} else {
			log.G(ctx).Tracef("ListContainers with filter %+v returns containers %+v",
				r.GetFilter(), res.GetContainers())
		}
	}()
	res, err = c.ListContainers(ctx, r)
	return res, errdefs.ToGRPC(err)
}

func (m *criServiceManager) ContainerStatus(ctx context.Context, r *runtime.ContainerStatusRequest) (res *runtime.ContainerStatusResponse, _ error) {
	ctx, c, err := m.checkInitialized(ctx)
	if err != nil {
		return nil, err
	}
	log.G(ctx).Tracef("ContainerStatus for %q", r.GetContainerId())
	defer func() {
		if err != nil {
			log.G(ctx).WithError(err).Errorf("ContainerStatus for %q failed", r.GetContainerId())
		} else {
			log.G(ctx).Tracef("ContainerStatus for %q returns status %+v", r.GetContainerId(), res.GetStatus())
		}
	}()
	res, err = c.ContainerStatus(ctx, r)
	return res, errdefs.ToGRPC(err)
}

func (m *criServiceManager) StopContainer(ctx context.Context, r *runtime.StopContainerRequest) (res *runtime.StopContainerResponse, _ error) {
	ctx, c, err := m.checkInitialized(ctx)
	if err != nil {
		return nil, err
	}
	log.G(ctx).Infof("StopContainer for %q with timeout %d (s)", r.GetContainerId(), r.GetTimeout())
	defer func() {
		if err != nil {
			log.G(ctx).WithError(err).Errorf("StopContainer for %q failed", r.GetContainerId())
		} else {
			log.G(ctx).Infof("StopContainer for %q returns successfully", r.GetContainerId())
		}
	}()
	res, err = c.StopContainer(ctx, r)
	return res, errdefs.ToGRPC(err)
}

func (m *criServiceManager) RemoveContainer(ctx context.Context, r *runtime.RemoveContainerRequest) (res *runtime.RemoveContainerResponse, _ error) {
	ctx, c, err := m.checkInitialized(ctx)
	if err != nil {
		return nil, err
	}
	log.G(ctx).Infof("RemoveContainer for %q", r.GetContainerId())
	defer func() {
		if err != nil {
			log.G(ctx).WithError(err).Errorf("RemoveContainer for %q failed", r.GetContainerId())
		} else {
			log.G(ctx).Infof("RemoveContainer for %q returns successfully", r.GetContainerId())
		}
	}()
	res, err = c.RemoveContainer(ctx, r)
	return res, errdefs.ToGRPC(err)
}

func (m *criServiceManager) ExecSync(ctx context.Context, r *runtime.ExecSyncRequest) (res *runtime.ExecSyncResponse, _ error) {
	ctx, c, err := m.checkInitialized(ctx)
	if err != nil {
		return nil, err
	}
	log.G(ctx).Infof("ExecSync for %q with command %+v and timeout %d (s)", r.GetContainerId(), r.GetCmd(), r.GetTimeout())
	defer func() {
		if err != nil {
			log.G(ctx).WithError(err).Errorf("ExecSync for %q failed", r.GetContainerId())
		} else {
			log.G(ctx).Infof("ExecSync for %q returns with exit code %d", r.GetContainerId(), res.GetExitCode())
			log.G(ctx).Debugf("ExecSync for %q outputs - stdout: %q, stderr: %q", r.GetContainerId(),
				res.GetStdout(), res.GetStderr())
		}
	}()
	res, err = c.ExecSync(ctx, r)
	return res, errdefs.ToGRPC(err)
}

func (m *criServiceManager) Exec(ctx context.Context, r *runtime.ExecRequest) (res *runtime.ExecResponse, _ error) {
	ctx, c, err := m.checkInitialized(ctx)
	if err != nil {
		return nil, err
	}
	log.G(ctx).Infof("Exec for %q with command %+v, tty %v and stdin %v",
		r.GetContainerId(), r.GetCmd(), r.GetTty(), r.GetStdin())
	defer func() {
		if err != nil {
			log.G(ctx).WithError(err).Errorf("Exec for %q failed", r.GetContainerId())
		} else {
			log.G(ctx).Infof("Exec for %q returns URL %q", r.GetContainerId(), res.GetUrl())
		}
	}()
	res, err = c.Exec(ctx, r)
	return res, errdefs.ToGRPC(err)
}

func (m *criServiceManager) Attach(ctx context.Context, r *runtime.AttachRequest) (res *runtime.AttachResponse, _ error) {
	ctx, c, err := m.checkInitialized(ctx)
	if err != nil {
		return nil, err
	}
	log.G(ctx).Infof("Attach for %q with tty %v and stdin %v", r.GetContainerId(), r.GetTty(), r.GetStdin())
	defer func() {
		if err != nil {
			log.G(ctx).WithError(err).Errorf("Attach for %q failed", r.GetContainerId())
		} else {
			log.G(ctx).Infof("Attach for %q returns URL %q", r.GetContainerId(), res.Url)
		}
	}()
	res, err = c.Attach(ctx, r)
	return res, errdefs.ToGRPC(err)
}

func (m *criServiceManager) UpdateContainerResources(ctx context.Context, r *runtime.UpdateContainerResourcesRequest) (res *runtime.UpdateContainerResourcesResponse, _ error) {
	ctx, c, err := m.checkInitialized(ctx)
	if err != nil {
		return nil, err
	}
	log.G(ctx).Infof("UpdateContainerResources for %q with %+v", r.GetContainerId(), r.GetLinux())
	defer func() {
		if err != nil {
			log.G(ctx).WithError(err).Errorf("UpdateContainerResources for %q failed", r.GetContainerId())
		} else {
			log.G(ctx).Infof("UpdateContainerResources for %q returns successfully", r.GetContainerId())
		}
	}()
	res, err = c.UpdateContainerResources(ctx, r)
	return res, errdefs.ToGRPC(err)
}

func (m *criServiceManager) PullImage(ctx context.Context, r *runtime.PullImageRequest) (res *runtime.PullImageResponse, _ error) {
	ctx, c, err := m.checkInitialized(ctx)
	if err != nil {
		return nil, err
	}
	log.G(ctx).Infof("PullImage %q", r.GetImage().GetImage())
	defer func() {
		if err != nil {
			log.G(ctx).WithError(err).Errorf("PullImage %q failed", r.GetImage().GetImage())
		} else {
			log.G(ctx).Infof("PullImage %q returns image reference %q",
				r.GetImage().GetImage(), res.GetImageRef())
		}
	}()
	res, err = c.PullImage(ctx, r)
	return res, errdefs.ToGRPC(err)
}

func (m *criServiceManager) ListImages(ctx context.Context, r *runtime.ListImagesRequest) (res *runtime.ListImagesResponse, _ error) {
	ctx, c, err := m.checkInitialized(ctx)
	if err != nil {
		return nil, err
	}
	log.G(ctx).Tracef("ListImages with filter %+v", r.GetFilter())
	defer func() {
		if err != nil {
			log.G(ctx).WithError(err).Errorf("ListImages with filter %+v failed", r.GetFilter())
		} else {
			log.G(ctx).Tracef("ListImages with filter %+v returns image list %+v",
				r.GetFilter(), res.GetImages())
		}
	}()
	res, err = c.ListImages(ctx, r)
	return res, errdefs.ToGRPC(err)
}

func (m *criServiceManager) ImageStatus(ctx context.Context, r *runtime.ImageStatusRequest) (res *runtime.ImageStatusResponse, _ error) {
	ctx, c, err := m.checkInitialized(ctx)
	if err != nil {
		return nil, err
	}
	log.G(ctx).Tracef("ImageStatus for %q", r.GetImage().GetImage())
	defer func() {
		if err != nil {
			log.G(ctx).WithError(err).Errorf("ImageStatus for %q failed", r.GetImage().GetImage())
		} else {
			log.G(ctx).Tracef("ImageStatus for %q returns image status %+v",
				r.GetImage().GetImage(), res.GetImage())
		}
	}()
	res, err = c.ImageStatus(ctx, r)
	return res, errdefs.ToGRPC(err)
}

func (m *criServiceManager) RemoveImage(ctx context.Context, r *runtime.RemoveImageRequest) (_ *runtime.RemoveImageResponse, _ error) {
	ctx, c, err := m.checkInitialized(ctx)
	if err != nil {
		return nil, err
	}
	log.G(ctx).Infof("RemoveImage %q", r.GetImage().GetImage())
	defer func() {
		if err != nil {
			log.G(ctx).WithError(err).Errorf("RemoveImage %q failed", r.GetImage().GetImage())
		} else {
			log.G(ctx).Infof("RemoveImage %q returns successfully", r.GetImage().GetImage())
		}
	}()
	res, err := c.RemoveImage(ctx, r)
	return res, errdefs.ToGRPC(err)
}

func (m *criServiceManager) ImageFsInfo(ctx context.Context, r *runtime.ImageFsInfoRequest) (res *runtime.ImageFsInfoResponse, _ error) {
	ctx, c, err := m.checkInitialized(ctx)
	if err != nil {
		return nil, err
	}
	log.G(ctx).Debugf("ImageFsInfo")
	defer func() {
		if err != nil {
			log.G(ctx).WithError(err).Error("ImageFsInfo failed")
		} else {
			log.G(ctx).Debugf("ImageFsInfo returns filesystem info %+v", res.ImageFilesystems)
		}
	}()
	res, err = c.ImageFsInfo(ctx, r)
	return res, errdefs.ToGRPC(err)
}

func (m *criServiceManager) ContainerStats(ctx context.Context, r *runtime.ContainerStatsRequest) (res *runtime.ContainerStatsResponse, _ error) {
	ctx, c, err := m.checkInitialized(ctx)
	if err != nil {
		return nil, err
	}
	log.G(ctx).Debugf("ContainerStats for %q", r.GetContainerId())
	defer func() {
		if err != nil {
			log.G(ctx).WithError(err).Errorf("ContainerStats for %q failed", r.GetContainerId())
		} else {
			log.G(ctx).Debugf("ContainerStats for %q returns stats %+v", r.GetContainerId(), res.GetStats())
		}
	}()
	res, err = c.ContainerStats(ctx, r)
	return res, errdefs.ToGRPC(err)
}

func (m *criServiceManager) ListContainerStats(ctx context.Context, r *runtime.ListContainerStatsRequest) (res *runtime.ListContainerStatsResponse, _ error) {
	ctx, c, err := m.checkInitialized(ctx)
	if err != nil {
		return nil, err
	}
	log.G(ctx).Tracef("ListContainerStats with filter %+v", r.GetFilter())
	defer func() {
		if err != nil {
			log.G(ctx).WithError(err).Error("ListContainerStats failed")
		} else {
			log.G(ctx).Tracef("ListContainerStats returns stats %+v", res.GetStats())
		}
	}()
	res, err = c.ListContainerStats(ctx, r)
	return res, errdefs.ToGRPC(err)
}

func (m *criServiceManager) Status(ctx context.Context, r *runtime.StatusRequest) (res *runtime.StatusResponse, _ error) {
	ctx, c, err := m.checkInitialized(ctx)
	if err != nil {
		return nil, err
	}
	log.G(ctx).Tracef("Status")
	defer func() {
		if err != nil {
			log.G(ctx).WithError(err).Error("Status failed")
		} else {
			log.G(ctx).Tracef("Status returns status %+v", res.GetStatus())
		}
	}()
	res, err = c.Status(ctx, r)
	return res, errdefs.ToGRPC(err)
}

func (m *criServiceManager) Version(ctx context.Context, r *runtime.VersionRequest) (res *runtime.VersionResponse, _ error) {
	ctx, c, err := m.checkInitialized(ctx)
	if err != nil {
		return nil, err
	}
	log.G(ctx).Tracef("Version with client side version %q", r.GetVersion())
	defer func() {
		if err != nil {
			log.G(ctx).WithError(err).Error("Version failed")
		} else {
			log.G(ctx).Tracef("Version returns %+v", res)
		}
	}()
	res, err = c.Version(ctx, r)
	return res, errdefs.ToGRPC(err)
}

func (m *criServiceManager) UpdateRuntimeConfig(ctx context.Context, r *runtime.UpdateRuntimeConfigRequest) (res *runtime.UpdateRuntimeConfigResponse, _ error) {
	ctx, c, err := m.checkInitialized(ctx)
	if err != nil {
		return nil, err
	}
	log.G(ctx).Debugf("UpdateRuntimeConfig with config %+v", r.GetRuntimeConfig())
	defer func() {
		if err != nil {
			log.G(ctx).WithError(err).Error("UpdateRuntimeConfig failed")
		} else {
			log.G(ctx).Debug("UpdateRuntimeConfig returns returns successfully")
		}
	}()
	res, err = c.UpdateRuntimeConfig(ctx, r)
	return res, errdefs.ToGRPC(err)
}

func (m *criServiceManager) ReopenContainerLog(ctx context.Context, r *runtime.ReopenContainerLogRequest) (res *runtime.ReopenContainerLogResponse, _ error) {
	ctx, c, err := m.checkInitialized(ctx)
	if err != nil {
		return nil, err
	}
	log.G(ctx).Debugf("ReopenContainerLog for %q", r.GetContainerId())
	defer func() {
		if err != nil {
			log.G(ctx).WithError(err).Errorf("ReopenContainerLog for %q failed", r.GetContainerId())
		} else {
			log.G(ctx).Debugf("ReopenContainerLog for %q returns successfully", r.GetContainerId())
		}
	}()
	res, err = c.ReopenContainerLog(ctx, r)
	return res, errdefs.ToGRPC(err)
}

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
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"time"

	"github.com/containerd/cri/pkg/constants"

	"github.com/pkg/errors"
	"google.golang.org/grpc"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	kubeletutil "k8s.io/kubernetes/pkg/kubelet/util"

	criconfig "github.com/containerd/cri/pkg/config"
	ctrdutil "github.com/containerd/cri/pkg/containerd/util"
)

func NamespaceContextAndRawClients(ns string) (context.Context, runtime.RuntimeServiceClient, runtime.ImageServiceClient, error) {
	addr, dialer, err := kubeletutil.GetAddressAndDialer(*criEndpoint)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to get dialer")
	}
	ctx, cancel := context.WithTimeout(ctrdutil.NamespacedContext(ns), timeout)
	defer cancel()
	conn, err := grpc.DialContext(ctx, addr, grpc.WithInsecure(), grpc.WithContextDialer(dialer))
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to connect cri endpoint")
	}
	return ctrdutil.NamespacedContext(ns), runtime.NewRuntimeServiceClient(conn), runtime.NewImageServiceClient(conn), nil
}

func NamespacedConfig(ns string, crt runtime.RuntimeServiceClient) (*criconfig.PluginConfig, error) {
	resp, err := crt.Status(ctrdutil.NamespacedContext(ns), &runtime.StatusRequest{Verbose: true})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get status")
	}
	config := &criconfig.PluginConfig{}
	if err := json.Unmarshal([]byte(resp.Info["config"]), config); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal config")
	}
	return config, nil
}

func StartManagedNamespace(ns string, crt runtime.RuntimeServiceClient) error {
	ctx := ctrdutil.NamespacedContext(ns)
	err := containerdClient.NamespaceService().SetLabel(ctx, ns, "io.cri-containerd", "managed")
	if err != nil {
		return err
	}
	defaultConfig, err := NamespacedConfig(constants.K8sContainerdNamespace, crt)
	if err != nil {
		return err
	}
	time.Sleep(5 * time.Second)
	namespaceConfig, err := NamespacedConfig(ns, crt)
	if err != nil {
		return err
	}
	info, err := ioutil.ReadDir(defaultConfig.NetworkPluginConfDir)
	if err != nil {
		return err
	}
	for i := range info {
		inf := info[i]
		if !inf.IsDir() {
			b, err := ioutil.ReadFile(filepath.Join(defaultConfig.NetworkPluginConfDir, inf.Name()))
			if err != nil {
				return err
			}
			err = ioutil.WriteFile(filepath.Join(namespaceConfig.NetworkPluginConfDir, inf.Name()), b, inf.Mode())
			if err != nil {
				return err
			}
		}
	}
	return nil
}

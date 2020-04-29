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
	"bytes"
	"context"
	"path/filepath"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/containerd/containerd"
	eventtypes "github.com/containerd/containerd/api/events"
	"github.com/containerd/containerd/events/exchange"
	"github.com/containerd/containerd/log"
	"github.com/containerd/containerd/namespaces"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/containerd/cri/pkg/atomic"
	criconfig "github.com/containerd/cri/pkg/config"
	"github.com/containerd/cri/pkg/constants"
	osinterface "github.com/containerd/cri/pkg/os"
	"github.com/containerd/cri/pkg/registrar"
	containerstore "github.com/containerd/cri/pkg/store/container"
	imagestore "github.com/containerd/cri/pkg/store/image"
	sandboxstore "github.com/containerd/cri/pkg/store/sandbox"
	snapshotstore "github.com/containerd/cri/pkg/store/snapshot"
)

var _ CRIService = &criServiceManager{}

// criServiceManager implements CRIService.
type criServiceManager struct {
	// config contains all configurations.
	config criconfig.Config
	// client is an instance of the containerd client
	client *containerd.Client
	// os is an interface for all required os operations.
	os osinterface.OS
	// ex is an event exchange for container runtime events
	ex *exchange.Exchange
	// ns is a map of criServices keyed by namespace, protected by a mutex
	ns sync.Map
}

func (c *criServiceManager) namespaceRequired(ctx context.Context) (*criService, error) {
	nsName, err := namespaces.NamespaceRequired(ctx)
	if err != nil {
		return nil, err
	}
	nsService, ok := c.ns.Load(nsName)
	if !ok {
		return nil, errors.Errorf("unmanaged namespace: %s", nsName)
	}
	return nsService.(*criService), nil
}

// Run the CRI service.
func (c *criServiceManager) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	log.G(ctx).Info("Start subscribing containerd events")
	// filter on container events to handle across all namespaces
	envCh, errCh := c.client.Subscribe(ctx, `topic=="/tasks/oom"`, `topic~="/images/"`, `topic~="/namespaces/"`)

	// setup managed namespaces
	namespaceNames, err := c.client.NamespaceService().List(ctx)
	if err != nil {
		return errors.Wrap(err, "Failed to list namespaces")
	}
	if len(namespaceNames) == 0 {
		log.G(ctx).Infof("Namespaces list from containerd is empty, establishing default %q", constants.K8sContainerdNamespace)
		namespaceNames = []string{constants.K8sContainerdNamespace}
	}
	for _, nsn := range namespaceNames {
		labels, err := c.client.NamespaceService().Labels(ctx, nsn)
		if err != nil {
			log.G(ctx).WithError(err).Errorf("Failed to list labels for namespace %q, assuming managed", nsn)
			labels = map[string]string{
				criContainerdPrefix: managedLabelValue,
			}
		}
		managed, ok := labels[criContainerdPrefix]
		switch {
		case !ok && nsn == constants.K8sContainerdNamespace:
			// special case for the default namespace: if it isn't marked as managed, mark it as such
			fallthrough
		case managed == managedLabelValue:
			nsx := namespaces.WithNamespace(log.WithLogger(ctx, log.L.WithField("ns", nsn)), nsn)
			if err := c.client.NamespaceService().SetLabel(nsx, nsn, criContainerdPrefix, managedLabelValue); err != nil {
				return errors.Wrapf(err, "Failed to label namespace %q", nsn)
			}
		}
	}

	// start the event exchange
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			return err
		case env, ok := <-envCh:
			if ok {
				name, evt, err := convertEvent(env.Event)
				if err != nil {
					log.G(ctx).Error(err)
				}
				switch e := evt.(type) {
				case *eventtypes.NamespaceCreate:
					if err := c.handleNamespaceUpdate(ctx, name, e.Labels); err != nil {
						log.G(ctx).Error(err)
					}
				case *eventtypes.NamespaceUpdate:
					if err := c.handleNamespaceUpdate(ctx, name, e.Labels); err != nil {
						log.G(ctx).Error(err)
					}
				case *eventtypes.NamespaceDelete:
					if err := c.handleNamespaceUpdate(ctx, name, map[string]string{}); err != nil {
						log.G(ctx).Error(err)
					}
				}
				if err := c.ex.Forward(ctx, env); err != nil {
					return err
				}
			}
		}
	}
}

// Close stops the CRI service.
func (c *criServiceManager) Close() error {
	logrus.Info("Stop CRI service")
	c.ns.Range(func(nsn interface{}, nsv interface{}) bool {
		defer c.ns.Delete(nsn)
		ns := nsv.(*criService)
		if err := ns.Close(); err != nil {
			logrus.WithField("ns", ns.name).WithError(err).Warn("Failed to stop CRI namespace")
		}
		return true
	})
	return nil
}

func (c *criServiceManager) newNamespacedService(ctx context.Context, name string) (_ context.Context, svc *criService, err error) {
	ctx = namespaces.WithNamespace(log.WithLogger(ctx, log.L.WithField("ns", name)), name)
	svc = &criService{
		name:               name,
		os:                 c.os,
		client:             c.client,
		imageStore:         imagestore.NewStore(c.client),
		sandboxStore:       sandboxstore.NewStore(),
		sandboxNameIndex:   registrar.NewRegistrar(),
		containerStore:     containerstore.NewStore(),
		containerNameIndex: registrar.NewRegistrar(),
		snapshotStore:      snapshotstore.NewStore(),
		initialized:        atomic.NewBool(false),
	}

	return ctx, svc, func() error {
		if svc.name == constants.K8sContainerdNamespace {
			svc.config.RootDir = c.config.RootDir
			svc.config.StateDir = c.config.StateDir
			svc.config.PluginConfig = c.config.PluginConfig
		} else {
			svc.config.RootDir = c.getNamespacesRootDir(svc.name)
			svc.config.StateDir = c.getNamespacesStateDir(svc.name)
			if err := c.loadNamespacedServiceConfig(svc.name, &svc.config); err != nil {
				log.G(ctx).WithError(err).Warnf("Using default config for namespace")
				svc.config.PluginConfig = criconfig.DefaultServiceConfig(svc.name)
			}
		}

		if svc.config.Snapshotter == "" {
			svc.config.Snapshotter = c.config.Snapshotter
		}
		if svc.config.Snapshotter == "" {
			svc.config.Snapshotter = containerd.DefaultSnapshotter
		}
		if snapshotter := svc.client.SnapshotService(svc.config.Snapshotter); snapshotter == nil {
			return errors.Errorf("failed to find snapshotter %q", svc.config.Snapshotter)
		}
		svc.imageFSPath = imageFSPath(c.config.ContainerdRootDir, svc.config.Snapshotter)

		return nil
	}()
}

func (c *criServiceManager) startNamespacedService(ctx context.Context, svc *criService) error {
	err := svc.initNetworking()
	if err != nil {
		return errors.Wrap(err, "failed to initialize networking")
	}

	svc.cniNetConfMonitor, err = newCNINetConfSyncer(svc.config.NetworkPluginConfDir, svc.netPlugin, svc.cniLoadOptions())
	if err != nil {
		return errors.Wrap(err, "failed to create cni conf monitor")
	}

	// prepare streaming server
	svc.streamServer, err = newStreamServer(svc)
	if err != nil {
		return errors.Wrap(err, "failed to create stream server")
	}

	svc.eventMonitor = newEventMonitor(ctx, svc)

	go svc.Run(ctx, c.ex)

	return nil
}

func (c *criServiceManager) handleNamespaceUpdate(ctx context.Context, ns string, labels map[string]string) error {
	managed, ok := labels[criContainerdPrefix]
	if ok && managed == managedLabelValue {
		ctx, svc, err := c.newNamespacedService(ctx, ns)
		if err != nil {
			return errors.Wrapf(err, "Failed to handle namespace update %q", ns)
		}
		if _, loaded := c.ns.LoadOrStore(ns, svc); !loaded {
			if err := c.saveNamespacedServiceConfig(ns, &svc.config); err != nil {
				log.G(ctx).WithError(err).Warn("Unable to persist namespace config")
			}
			if err := c.startNamespacedService(ctx, svc); err != nil {
				log.G(ctx).WithError(err).Error("Unable to start namespace service")
				c.ns.Delete(ns)
			}
		}
		return nil
	} else if v, ok := c.ns.Load(ns); ok {
		defer c.ns.Delete(ns)
		return v.(*criService).Close()
	}
	return nil
}

// getNamespacesRootDir returns the root directory for managing namespace persistent state and configuration.
func (m *criServiceManager) getNamespacesRootDir(id string) string {
	return filepath.Join(m.config.RootDir, namespacesDir, id)
}

// getNamespacesStateDir returns the root directory for managing namespace ephemeral state.
func (m *criServiceManager) getNamespacesStateDir(id string) string {
	return filepath.Join(m.config.StateDir, namespacesDir, id)
}

// loadNamespacedServiceConfig for a namespace
func (m *criServiceManager) loadNamespacedServiceConfig(ns string, config *serviceConfig) error {
	path := filepath.Join(m.getNamespacesRootDir(ns), "config.toml")
	_, err := toml.DecodeFile(path, &config.PluginConfig)
	if err != nil {
		return errors.Wrapf(err, "Failed to load service config for namespace %q", ns)
	}
	return nil
}

// saveNamespacedServiceConfig for a namespace
func (m *criServiceManager) saveNamespacedServiceConfig(ns string, config *serviceConfig) error {
	path := filepath.Join(m.getNamespacesRootDir(ns), "config.toml")
	if err := m.os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return errors.Wrapf(err, "Failed to save service config for namespace %q", ns)
	}
	buffer := &bytes.Buffer{}
	if err := toml.NewEncoder(buffer).Encode(&config.PluginConfig); err != nil {
		return errors.Wrapf(err, "Failed to save service config for namespace %q", ns)
	}
	if err := m.os.WriteFile(path, buffer.Bytes(), 0600); err != nil {
		return errors.Wrapf(err, "Failed to save service config for namespace %q", ns)
	}
	return nil
}

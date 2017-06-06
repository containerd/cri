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

package metadata

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/containernetworking/cni/pkg/ns"
	"github.com/golang/glog"

	"github.com/kubernetes-incubator/cri-containerd/pkg/metadata/store"

	runtime "k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1"
)

// TODO(random-liu): Handle versioning around all marshal/unmarshal.
// version and versionedSandboxMetadata is not used now, but should be used
// in the future:
// 1) Only versioned metadata should be written into the store;
// 2) Only unversioned metadata should be returned to the user;
// 3) A conversion function is needed to convert any supported versioned
// metadata into unversioned metadata.

// sandboxMetadataVersion is current version of sandbox metadata.
const (
	sandboxMetadataVersion = "v1" // nolint
	nsRunDir               = "/var/run/netns"
)

// versionedSandboxMetadata is the internal struct representing the versioned
// sandbox metadata
// nolint
type versionedSandboxMetadata struct {
	// Version indicates the version of the versioned sandbox metadata.
	Version string
	SandboxMetadata
}

// SandboxMetadata is the unversioned sandbox metadata.
type SandboxMetadata struct {
	// ID is the sandbox id.
	ID string
	// Name is the sandbox name.
	Name string
	// Config is the CRI sandbox config.
	Config *runtime.PodSandboxConfig
	// CreatedAt is the created timestamp.
	CreatedAt int64
	// NetNS is the network namespace used by the sandbox.
	NetNS *SandboxNetNS
	// SandboxPID is the pid of sandbox container.
	SandboxPID uint32
}

// SandboxUpdateFunc is the function used to update SandboxMetadata.
type SandboxUpdateFunc func(SandboxMetadata) (SandboxMetadata, error)

// sandboxToStoreUpdateFunc generates a metadata store UpdateFunc from SandboxUpdateFunc.
func sandboxToStoreUpdateFunc(u SandboxUpdateFunc) store.UpdateFunc {
	return func(data []byte) ([]byte, error) {
		meta := &SandboxMetadata{}
		if err := json.Unmarshal(data, meta); err != nil {
			return nil, err
		}
		newMeta, err := u(*meta)
		if err != nil {
			return nil, err
		}
		return json.Marshal(newMeta)
	}
}

// SandboxStore is the store for metadata of all sandboxes.
type SandboxStore interface {
	// Create creates a sandbox from SandboxMetadata in the store.
	Create(SandboxMetadata) error
	// Get gets the specified sandbox.
	Get(string) (*SandboxMetadata, error)
	// Update updates a specified sandbox.
	Update(string, SandboxUpdateFunc) error
	// List lists all sandboxes.
	List() ([]*SandboxMetadata, error)
	// Delete deletes the sandbox from the store.
	Delete(string) error
}

// sandboxStore is an implmentation of SandboxStore.
type sandboxStore struct {
	store store.MetadataStore
}

// NewSandboxStore creates a SandboxStore from a basic MetadataStore.
func NewSandboxStore(store store.MetadataStore) SandboxStore {
	return &sandboxStore{store: store}
}

// Create creates a sandbox from SandboxMetadata in the store.
func (s *sandboxStore) Create(metadata SandboxMetadata) error {
	data, err := json.Marshal(&metadata)
	if err != nil {
		return err
	}
	return s.store.Create(metadata.ID, data)
}

// Get gets the specified sandbox.
func (s *sandboxStore) Get(sandboxID string) (*SandboxMetadata, error) {
	data, err := s.store.Get(sandboxID)
	if err != nil {
		return nil, err
	}
	sandbox := &SandboxMetadata{}
	if err := json.Unmarshal(data, sandbox); err != nil {
		return nil, err
	}
	return sandbox, nil
}

// Update updates a specified sandbox. The function is running in a
// transaction. Update will not be applied when the update function
// returns error.
func (s *sandboxStore) Update(sandboxID string, u SandboxUpdateFunc) error {
	return s.store.Update(sandboxID, sandboxToStoreUpdateFunc(u))
}

// List lists all sandboxes.
func (s *sandboxStore) List() ([]*SandboxMetadata, error) {
	allData, err := s.store.List()
	if err != nil {
		return nil, err
	}
	var sandboxes []*SandboxMetadata
	for _, data := range allData {
		sandbox := &SandboxMetadata{}
		if err := json.Unmarshal(data, sandbox); err != nil {
			return nil, err
		}
		sandboxes = append(sandboxes, sandbox)
	}
	return sandboxes, nil
}

// Delete deletes the sandbox from the store.
func (s *sandboxStore) Delete(sandboxID string) error {
	return s.store.Delete(sandboxID)
}

type SandboxNetNS struct {
	sync.Mutex
	NS     ns.NetNS
	Closed bool
}

// NetNsPath returns the path to the network namespace of the container.
func (s *SandboxMetadata) NetNSPath() string {
	return s.NetNS.NS.Path()
}

// NetNsCreate creates netns and symlink it with a file.
func (s *SandboxMetadata) NetNSCreate() error {
	if s.NetNS != nil {
		return fmt.Errorf("net NS already created")
	}
	netNS, err := ns.NewNS()
	if err != nil {
		return fmt.Errorf("failed to create new net NS: %v", err)
	}
	s.NetNS = &SandboxNetNS{
		NS:     netNS,
		Closed: false,
	}
	return nil
}

// NetNsRemove removes the netns store in sandbox meta data.
func (s *SandboxMetadata) NetNSRemove() error {
	if s.NetNS == nil {
		glog.Error("no networking namespace to be removed.")
		return nil
	}
	s.NetNS.Lock()
	defer s.NetNS.Unlock()
	if s.NetNS.Closed {
		// netNsRemove() can be called multiple times without returning an error.
		return nil
	}
	if err := s.NetNS.NS.Close(); err != nil {
		return fmt.Errorf("failed to close net NS: %v", err)
	}
	// TODO: Handle restore logic with netNsGet.
	// TODO: Get rid of the forcibly cleanup on restore after containernetworking/cni#342 is fixed.
	s.NetNS.Closed = true
	return nil
}

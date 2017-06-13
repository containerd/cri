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

package testing

import (
	"fmt"
	"sync"

	"github.com/containerd/containerd/api/services/snapshot"
	"github.com/containerd/containerd/api/types/mount"
	google_protobuf1 "github.com/golang/protobuf/ptypes/empty"
	"github.com/opencontainers/go-digest"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

// FakeSnapshotClient is a simple fake snapshot client, so that cri-containerd
// can be run for testing without requiring a real containerd setup.
type FakeSnapshotClient struct {
	sync.Mutex
	called      []CalledDetail
	errors      map[string]error
	ChainIDList map[digest.Digest]struct{}
	MountList   map[string][]*mount.Mount
}

var _ snapshot.SnapshotClient = &FakeSnapshotClient{}

// NewFakeSnapshotClient creates a FakeSnapshotClient
func NewFakeSnapshotClient() *FakeSnapshotClient {
	return &FakeSnapshotClient{
		errors:      make(map[string]error),
		ChainIDList: make(map[digest.Digest]struct{}),
		MountList:   make(map[string][]*mount.Mount),
	}
}

func (f *FakeSnapshotClient) getError(op string) error {
	err, ok := f.errors[op]
	if ok {
		delete(f.errors, op)
		return err
	}
	return nil
}

// InjectError inject error for call
func (f *FakeSnapshotClient) InjectError(fn string, err error) {
	f.Lock()
	defer f.Unlock()
	f.errors[fn] = err
}

// InjectErrors inject errors for calls
func (f *FakeSnapshotClient) InjectErrors(errs map[string]error) {
	f.Lock()
	defer f.Unlock()
	for fn, err := range errs {
		f.errors[fn] = err
	}
}

// ClearErrors clear errors for call
func (f *FakeSnapshotClient) ClearErrors() {
	f.Lock()
	defer f.Unlock()
	f.errors = make(map[string]error)
}

//For uncompressed layers, diffID and digest will be the same. For compressed
//layers, we can look up the diffID from the digest if we've already unpacked it.
//In the FakeSnapshotClient, We just use layer digest as diffID.
/* TODO(mikebrow) no longer used see if still needed
func generateChainID(layers []*descriptor.Descriptor) digest.Digest {
	var digests []digest.Digest
	for _, layer := range layers {
		digests = append(digests, layer.Digest)
	}
	parent := identity.ChainID(digests)
	return parent
}
*/

func (f *FakeSnapshotClient) appendCalled(name string, argument interface{}) {
	call := CalledDetail{Name: name, Argument: argument}
	f.called = append(f.called, call)
}

// GetCalledNames get names of call
func (f *FakeSnapshotClient) GetCalledNames() []string {
	f.Lock()
	defer f.Unlock()
	names := []string{}
	for _, detail := range f.called {
		names = append(names, detail.Name)
	}
	return names
}

// GetCalledDetails get detail of each call.
func (f *FakeSnapshotClient) GetCalledDetails() []CalledDetail {
	f.Lock()
	defer f.Unlock()
	// Copy the list and return.
	return append([]CalledDetail{}, f.called...)
}

// SetFakeChainIDs injects fake chainIDs.
func (f *FakeSnapshotClient) SetFakeChainIDs(chainIDs []digest.Digest) {
	f.Lock()
	defer f.Unlock()
	for _, c := range chainIDs {
		f.ChainIDList[c] = struct{}{}
	}
}

// SetFakeMounts injects fake mounts.
func (f *FakeSnapshotClient) SetFakeMounts(name string, mounts []*mount.Mount) {
	f.Lock()
	defer f.Unlock()
	f.MountList[name] = mounts
}

// Prepare is a test implementation of snapshot.Prepare
func (f *FakeSnapshotClient) Prepare(ctx context.Context, prepareOpts *snapshot.PrepareRequest, opts ...grpc.CallOption) (*snapshot.MountsResponse, error) {
	f.Lock()
	defer f.Unlock()
	f.appendCalled("prepare", prepareOpts)
	if err := f.getError("prepare"); err != nil {
		return nil, err
	}
	_, ok := f.MountList[prepareOpts.Key]
	if ok {
		return nil, fmt.Errorf("mounts already exist")
	}
	f.MountList[prepareOpts.Key] = []*mount.Mount{{
		Type:   "bind",
		Source: prepareOpts.Key,
		// TODO(random-liu): Fake options based on Readonly option.
	}}
	return &snapshot.MountsResponse{
		Mounts: f.MountList[prepareOpts.Key],
	}, nil
}

// Mounts is a test implementation of snapshot.Mounts
func (f *FakeSnapshotClient) Mounts(ctx context.Context, mountsOpts *snapshot.MountsRequest, opts ...grpc.CallOption) (*snapshot.MountsResponse, error) {
	f.Lock()
	defer f.Unlock()
	f.appendCalled("mounts", mountsOpts)
	if err := f.getError("mounts"); err != nil {
		return nil, err
	}
	mounts, ok := f.MountList[mountsOpts.Key]
	if !ok {
		return nil, fmt.Errorf("mounts not exist")
	}
	return &snapshot.MountsResponse{
		Mounts: mounts,
	}, nil
}

// Commit is a test implementation of snapshot.Commit
func (f *FakeSnapshotClient) Commit(ctx context.Context, in *snapshot.CommitRequest, opts ...grpc.CallOption) (*google_protobuf1.Empty, error) {
	return nil, nil
}

// View is a test implementation of snapshot.View
func (f *FakeSnapshotClient) View(ctx context.Context, in *snapshot.PrepareRequest, opts ...grpc.CallOption) (*snapshot.MountsResponse, error) {
	return nil, nil
}

// Remove is a test implementation of snapshot.Remove
func (f *FakeSnapshotClient) Remove(ctx context.Context, in *snapshot.RemoveRequest, opts ...grpc.CallOption) (*google_protobuf1.Empty, error) {
	return nil, nil
}

// Stat is a test implementation of snapshot.Stat
func (f *FakeSnapshotClient) Stat(ctx context.Context, in *snapshot.StatRequest, opts ...grpc.CallOption) (*snapshot.StatResponse, error) {
	return nil, nil
}

// List is a test implementation of snapshot.List
func (f *FakeSnapshotClient) List(ctx context.Context, in *snapshot.ListRequest, opts ...grpc.CallOption) (snapshot.Snapshot_ListClient, error) {
	return nil, nil
}

// Usage is a test implementation of snapshot.Usage
func (f *FakeSnapshotClient) Usage(ctx context.Context, in *snapshot.UsageRequest, opts ...grpc.CallOption) (*snapshot.UsageResponse, error) {
	return nil, nil
}

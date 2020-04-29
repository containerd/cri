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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"

	ctrdutil "github.com/containerd/cri/pkg/containerd/util"
)

const (
	created   = "created.test.dev"
	updated   = "updated.test.dev"
	unmanaged = "unmanaged.test.dev"
)

func TestManagedNamespaceLifecycle(t *testing.T) {
	ctx, crt, _, err := NamespaceContextAndRawClients(unmanaged)
	assert.NoError(t, err)

	t.Run(unmanaged, func(t *testing.T) {
		t.Log("status from unmanaged namespace should fail")
		_, err := crt.Status(ctx, &runtime.StatusRequest{Verbose: true})
		assert.Error(t, err)
	})

	t.Run(created, testCreateManagedNamespace(created, crt, func(ctx context.Context, ns string) error {
		return containerdClient.NamespaceService().Create(ctx, ns, map[string]string{"io.cri-containerd": "managed"})
	}))

	t.Run(updated, testCreateManagedNamespace(updated, crt, func(ctx context.Context, ns string) error {
		return containerdClient.NamespaceService().SetLabel(ctx, ns, "io.cri-containerd", "managed")
	}))
}

type createManagedNamespaceFn func(context.Context, string) error

func testCreateManagedNamespace(ns string, runtimeServiceClient runtime.RuntimeServiceClient, create createManagedNamespaceFn) func(*testing.T) {
	return func(t *testing.T) {
		ctx := ctrdutil.NamespacedContext(ns)
		defer containerdClient.NamespaceService().Delete(ctx, ns)

		t.Logf("manage namespace %q", ns)
		require.NoError(t, create(ctx, ns))

		// give it a chance to start
		time.Sleep(5 * time.Second)

		t.Log("status from managed namespace should succeed")
		res, err := runtimeServiceClient.Status(ctx, &runtime.StatusRequest{Verbose: true})
		assert.NoError(t, err)
		assert.NotEmpty(t, res)

		t.Log("delete managed namespace")
		assert.NoError(t,
			containerdClient.NamespaceService().Delete(ctx, ns),
		)

		// give it a chance to stop
		time.Sleep(5 * time.Second)

		t.Log("status from deleted namespace should fail")
		res, err = runtimeServiceClient.Status(ctx, &runtime.StatusRequest{})
		assert.Error(t, err)
	}
}

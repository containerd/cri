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

package util

import (
	"time"

	"github.com/containerd/containerd/log"
	"github.com/containerd/containerd/namespaces"
	"golang.org/x/net/context"

	"github.com/containerd/cri/pkg/constants"
)

// deferCleanupTimeout is the default timeout for containerd cleanup operations
// in defer.
const deferCleanupTimeout = 1 * time.Minute

// DeferContext returns a context for containerd cleanup operations in defer.
// A default timeout is applied to avoid cleanup operation pending forever.
func DeferContext(ns string) (context.Context, context.CancelFunc) {
	return context.WithTimeout(NamespacedContext(ns), deferCleanupTimeout)
}

// NamespacedContext returns a context with kubernetes namespace set.
func NamespacedContext(ns string) context.Context {
	return namespaces.WithNamespace(
		log.WithLogger(context.Background(), log.L.WithField("ns", ns)), ns,
	)
}

// WithNamespace adds kubernetes namespace to the context.
func WithNamespace(ctx context.Context) context.Context {
	_, ok := namespaces.Namespace(ctx)
	if !ok {
		ns := constants.K8sContainerdNamespace
		ctx = namespaces.WithNamespace(
			log.WithLogger(ctx, log.L.WithField("ns", ns)), ns,
		)
	}
	return ctx
}

/*
Copyright 2018 The containerd Authors.

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

package image

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLeases(t *testing.T) {
	l := newLeases()

	t.Logf("Leases can be leased for multiple times")
	assert.NoError(t, l.Lease())
	assert.NoError(t, l.Lease())
	assert.NoError(t, l.Lease())

	t.Logf("Leases with non-zero lease count can't be set nonleasable")
	assert.Error(t, l.SetNonleasable(true))

	t.Logf("Leases with non-zero lease count can be unleased")
	assert.NoError(t, l.Unlease())
	assert.NoError(t, l.Unlease())
	assert.NoError(t, l.Unlease())

	t.Logf("Leases with zero lease count can't be unleased")
	assert.Error(t, l.Unlease())

	t.Logf("Leases with zero lease count can be set nonleasable")
	assert.NoError(t, l.SetNonleasable(true))

	t.Logf("Nonleasable leases can't be leased any more")
	assert.Error(t, l.Lease())

	t.Logf("Nonleasable leases can be set back to leasable")
	assert.NoError(t, l.SetNonleasable(false))

	t.Logf("Leases can be leased after being set back to leasable")
	assert.NoError(t, l.Lease())
}

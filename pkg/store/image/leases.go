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

package image

import (
	"sync"

	"github.com/pkg/errors"
)

// leases manages all leases of an image.
// * Leases can be used for many times;
// * Leases with non-zero lease count can't be set nonleasable.
// * Nonleasable leases can't be used.
type leases struct {
	sync.Mutex
	count       int
	nonleasable bool
}

func newLeases() *leases {
	return &leases{}
}

// Lease leases the image.
func (l *leases) Lease() error {
	l.Lock()
	defer l.Unlock()
	if l.nonleasable {
		return errors.New("not leasable")
	}
	l.count++
	return nil
}

// Unlease unleases the image.
func (l *leases) Unlease() error {
	l.Lock()
	defer l.Unlock()
	if l.count <= 0 {
		return errors.New("not leased")
	}
	l.count--
	return nil
}

// SetNonleasable sets the image nonleasable.
func (l *leases) SetNonleasable(nonleasable bool) error {
	l.Lock()
	defer l.Unlock()
	if nonleasable {
		if l.count > 0 {
			return errors.New("leased")
		}
	}
	l.nonleasable = nonleasable
	return nil
}

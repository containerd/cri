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

package io

import (
	"context"
	"io"
	"sync"

	"github.com/containerd/containerd/cio"
	"github.com/sirupsen/logrus"

	cioutil "github.com/containerd/cri-containerd/pkg/ioutil"
)

// ExecIO holds the exec io.
type ExecIO struct {
	cio.DirectIO
	id    string
	fifos *cio.FIFOSet
	wg    *sync.WaitGroup
}

var _ cio.IO = &ExecIO{}

// NewExecIO creates exec io.
func NewExecIO(id, root string, tty, stdin bool) (*ExecIO, error) {
	fifos, err := newFifos(root, id, tty, stdin)
	if err != nil {
		return nil, err
	}
	dio, err := cio.NewDirectIO(context.Background(), fifos)
	if err != nil {
		return nil, err
	}
	return &ExecIO{
		id:       id,
		fifos:    fifos,
		DirectIO: *dio,
	}, nil
}

// Attach attaches exec stdio. The logic is similar with container io attach.
func (e *ExecIO) Attach(opts AttachOptions) {
	var wg sync.WaitGroup
	var stdinStreamRC io.ReadCloser
	if e.Stdin != nil && opts.Stdin != nil {
		stdinStreamRC = cioutil.NewWrapReadCloser(opts.Stdin)
		wg.Add(1)
		go func() {
			if _, err := io.Copy(e.Stdin, stdinStreamRC); err != nil {
				logrus.WithError(err).Errorf("Failed to redirect stdin for container exec %q", e.id)
			}
			logrus.Infof("Container exec %q stdin closed", e.id)
			if opts.StdinOnce && !opts.Tty {
				e.Stdin.Close()
				if err := opts.CloseStdin(); err != nil {
					logrus.WithError(err).Errorf("Failed to close stdin for container exec %q", e.id)
				}
			} else {
				if e.Stdout != nil {
					e.Stdout.Close()
				}
				if e.Stderr != nil {
					e.Stderr.Close()
				}
			}
			wg.Done()
		}()
	}

	attachOutput := func(t StreamType, stream io.WriteCloser, out io.ReadCloser) {
		if _, err := io.Copy(stream, out); err != nil {
			logrus.WithError(err).Errorf("Failed to pipe %q for container exec %q", t, e.id)
		}
		out.Close()
		stream.Close()
		if stdinStreamRC != nil {
			stdinStreamRC.Close()
		}
		e.wg.Done()
		wg.Done()
		logrus.Infof("Finish piping %q of container exec %q", t, e.id)
	}

	if opts.Stdout != nil {
		wg.Add(1)
		// Closer should wait for this routine to be over.
		e.wg.Add(1)
		go attachOutput(Stdout, opts.Stdout, e.Stdout)
	}

	if !opts.Tty && opts.Stderr != nil {
		wg.Add(1)
		// Closer should wait for this routine to be over.
		e.wg.Add(1)
		go attachOutput(Stderr, opts.Stderr, e.Stderr)
	}
}

// Wait for IO streams to end
func (e *ExecIO) Wait() {
	if e.wg != nil {
		e.wg.Wait()
	}
}

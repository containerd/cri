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
	"errors"
	"io"
	"strings"
	"sync"

	"github.com/containerd/containerd/cio"
	"github.com/sirupsen/logrus"

	cioutil "github.com/containerd/cri-containerd/pkg/ioutil"
	"github.com/containerd/cri-containerd/pkg/util"
)

// streamKey generates a key for the stream.
func streamKey(id, name string, stream StreamType) string {
	return strings.Join([]string{id, name, string(stream)}, "-")
}

// ContainerIO holds the container io.
type ContainerIO struct {
	id string

	fifos *cio.FIFOSet
	cio.DirectIO

	stdoutGroup *cioutil.WriterGroup
	stderrGroup *cioutil.WriterGroup

	wg *sync.WaitGroup
}

var _ cio.IO = &ContainerIO{}

// ContainerIOOpts sets specific information to newly created ContainerIO.
type ContainerIOOpts func(*ContainerIO) error

// WithOutput adds output stream to the container io.
func WithOutput(name string, stdout, stderr io.WriteCloser) ContainerIOOpts {
	return func(c *ContainerIO) error {
		if stdout != nil {
			if err := c.stdoutGroup.Add(streamKey(c.id, name, Stdout), stdout); err != nil {
				return err
			}
		}
		if stderr != nil {
			if err := c.stderrGroup.Add(streamKey(c.id, name, Stderr), stderr); err != nil {
				return err
			}
		}
		return nil
	}
}

// WithFIFOs specifies existing fifos for the container io.
func WithFIFOs(fifos *cio.FIFOSet) ContainerIOOpts {
	return func(c *ContainerIO) error {
		c.fifos = fifos
		return nil
	}
}

// WithNewFIFOs creates new fifos for the container io.
func WithNewFIFOs(root string, tty, stdin bool) ContainerIOOpts {
	return func(c *ContainerIO) error {
		fifos, err := newFifos(root, c.id, tty, stdin)
		if err != nil {
			return err
		}
		return WithFIFOs(fifos)(c)
	}
}

// NewContainerIO creates container io.
func NewContainerIO(id string, opts ...ContainerIOOpts) (_ *ContainerIO, err error) {
	c := &ContainerIO{
		id:          id,
		stdoutGroup: cioutil.NewWriterGroup(),
		stderrGroup: cioutil.NewWriterGroup(),
	}
	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, err
		}
	}
	if c.fifos == nil {
		return nil, errors.New("fifos are not set")
	}
	dio, err := cio.NewDirectIO(context.Background(), c.fifos)
	if err != nil {
		return nil, err
	}
	c.DirectIO = *dio
	return c, nil
}

// Pipe creates container fifos and pipe container output
// to output stream.
func (c *ContainerIO) Pipe() {
	c.wg = &sync.WaitGroup{}
	c.wg.Add(1)
	go func() {
		if _, err := io.Copy(c.stdoutGroup, c.Stdout); err != nil {
			logrus.WithError(err).Errorf("Failed to pipe stdout of container %q", c.id)
		}
		c.Stdout.Close()
		c.stdoutGroup.Close()
		c.wg.Done()
		logrus.Infof("Finish piping stdout of container %q", c.id)
	}()

	if !c.fifos.Terminal {
		c.wg.Add(1)
		go func() {
			if _, err := io.Copy(c.stderrGroup, c.Stderr); err != nil {
				logrus.WithError(err).Errorf("Failed to pipe stderr of container %q", c.id)
			}
			c.Stderr.Close()
			c.stderrGroup.Close()
			c.wg.Done()
			logrus.Infof("Finish piping stderr of container %q", c.id)
		}()
	}
}

// Attach attaches container stdio.
// TODO(random-liu): Use pools.Copy in docker to reduce memory usage?
func (c *ContainerIO) Attach(opts AttachOptions) error {
	var wg sync.WaitGroup
	key := util.GenerateID()
	stdinKey := streamKey(c.id, "attach-"+key, Stdin)
	stdoutKey := streamKey(c.id, "attach-"+key, Stdout)
	stderrKey := streamKey(c.id, "attach-"+key, Stderr)

	var stdinStreamRC io.ReadCloser
	if c.Stdin != nil && opts.Stdin != nil {
		// Create a wrapper of stdin which could be closed. Note that the
		// wrapper doesn't close the actual stdin, it only stops io.Copy.
		// The actual stdin will be closed by stream server.
		stdinStreamRC = cioutil.NewWrapReadCloser(opts.Stdin)
		wg.Add(1)
		go func() {
			if _, err := io.Copy(c.Stdin, stdinStreamRC); err != nil {
				logrus.WithError(err).Errorf("Failed to pipe stdin for container attach %q", c.id)
			}
			logrus.Infof("Attach stream %q closed", stdinKey)
			if opts.StdinOnce && !opts.Tty {
				// Due to kubectl requirements and current docker behavior, when (opts.StdinOnce &&
				// opts.Tty) we have to close container stdin and keep stdout and stderr open until
				// container stops.
				c.Stdin.Close()
				// Also closes the containerd side.
				if err := opts.CloseStdin(); err != nil {
					logrus.WithError(err).Errorf("Failed to close stdin for container %q", c.id)
				}
			} else {
				if opts.Stdout != nil {
					c.stdoutGroup.Remove(stdoutKey)
				}
				if opts.Stderr != nil {
					c.stderrGroup.Remove(stderrKey)
				}
			}
			wg.Done()
		}()
	}

	attachStream := func(key string, close <-chan struct{}) {
		<-close
		logrus.Infof("Attach stream %q closed", key)
		// Make sure stdin gets closed.
		if stdinStreamRC != nil {
			stdinStreamRC.Close()
		}
		wg.Done()
	}

	if opts.Stdout != nil {
		wg.Add(1)
		wc, close := cioutil.NewWriteCloseInformer(opts.Stdout)
		if err := c.stdoutGroup.Add(stdoutKey, wc); err != nil {
			return err
		}
		go attachStream(stdoutKey, close)
	}
	if !opts.Tty && opts.Stderr != nil {
		wg.Add(1)
		wc, close := cioutil.NewWriteCloseInformer(opts.Stderr)
		if err := c.stderrGroup.Add(stderrKey, wc); err != nil {
			return err
		}
		go attachStream(stderrKey, close)
	}
	wg.Wait()
	return nil
}

// Wait for IO streams to end
func (c *ContainerIO) Wait() {
	if c.wg != nil {
		c.wg.Wait()
	}
}

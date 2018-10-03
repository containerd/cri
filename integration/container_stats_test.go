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

package integration

import (
	"fmt"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	runtime "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
)

// Test to verify for a container ID
func TestContainerStats(t *testing.T) {
	t.Logf("Create a pod config and run sandbox container")
	sbConfig := PodSandboxConfig("sandbox1", "stats")
	sb, err := runtimeService.RunPodSandbox(sbConfig)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, runtimeService.StopPodSandbox(sb))
		assert.NoError(t, runtimeService.RemovePodSandbox(sb))
	}()
	t.Logf("Create a container config and run container in a pod")
	containerConfig := ContainerConfig(
		"container1",
		pauseImage,
		WithTestLabels(),
		WithTestAnnotations(),
	)
	cn, err := runtimeService.CreateContainer(sb, containerConfig, sbConfig)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, runtimeService.RemoveContainer(cn))
	}()
	require.NoError(t, runtimeService.StartContainer(cn))
	defer func() {
		assert.NoError(t, runtimeService.StopContainer(cn, 10))
	}()

	t.Logf("Fetch stats for container")
	var s *runtime.ContainerStats
	require.NoError(t, Eventually(func() (bool, error) {
		s, err = runtimeService.ContainerStats(cn)
		if err != nil {
			return false, err
		}
		if s.GetWritableLayer().GetUsedBytes().GetValue() != 0 &&
			s.GetWritableLayer().GetInodesUsed().GetValue() != 0 {
			return true, nil
		}
		return false, nil
	}, time.Second, 30*time.Second))

	t.Logf("Verify stats received for container %q", cn)
	testStats(t, s, containerConfig)
}

// Test to verify filtering without any filter
func TestContainerListStats(t *testing.T) {
	t.Logf("Create a pod config and run sandbox container")
	sbConfig := PodSandboxConfig("running-pod", "statsls")
	sb, err := runtimeService.RunPodSandbox(sbConfig)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, runtimeService.StopPodSandbox(sb))
		assert.NoError(t, runtimeService.RemovePodSandbox(sb))
	}()
	t.Logf("Create a container config and run containers in a pod")
	containerConfigMap := make(map[string]*runtime.ContainerConfig)
	for i := 0; i < 3; i++ {
		cName := fmt.Sprintf("container%d", i)
		containerConfig := ContainerConfig(
			cName,
			pauseImage,
			WithTestLabels(),
			WithTestAnnotations(),
		)
		cn, err := runtimeService.CreateContainer(sb, containerConfig, sbConfig)
		require.NoError(t, err)
		containerConfigMap[cn] = containerConfig
		defer func() {
			assert.NoError(t, runtimeService.RemoveContainer(cn))
		}()
		require.NoError(t, runtimeService.StartContainer(cn))
		defer func() {
			assert.NoError(t, runtimeService.StopContainer(cn, 10))
		}()
	}

	t.Logf("Fetch all container stats")
	var stats []*runtime.ContainerStats
	require.NoError(t, Eventually(func() (bool, error) {
		stats, err = runtimeService.ListContainerStats(&runtime.ContainerStatsFilter{})
		if err != nil {
			return false, err
		}
		var count int
		for _, s := range stats {
			if s.GetWritableLayer().GetUsedBytes().GetValue() == 0 &&
				s.GetWritableLayer().GetInodesUsed().GetValue() == 0 {
				return false, nil
			}
			if containerConfigMap[s.GetAttributes().GetId()] != nil {
				count++
			}
		}
		if count != 3 {
			return false, errors.New("did not get stats for all 3 containers")
		}
		return true, nil
	}, time.Second, 30*time.Second))

	t.Logf("Verify all container stats")

	for _, s := range stats {
		if containerConfigMap[s.GetAttributes().GetId()] != nil {
			testStats(t, s, containerConfigMap[s.GetAttributes().GetId()])
		}
	}
}

// Test to verify filtering given a specific container ID
// TODO Convert the filter tests into table driven tests and unit tests
func TestContainerListStatsWithIdFilter(t *testing.T) {
	t.Logf("Create a pod config and run sandbox container")
	sbConfig := PodSandboxConfig("running-pod", "statsls")
	sb, err := runtimeService.RunPodSandbox(sbConfig)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, runtimeService.StopPodSandbox(sb))
		assert.NoError(t, runtimeService.RemovePodSandbox(sb))
	}()
	t.Logf("Create a container config and run containers in a pod")
	containerConfigMap := make(map[string]*runtime.ContainerConfig)
	for i := 0; i < 3; i++ {
		cName := fmt.Sprintf("container%d", i)
		containerConfig := ContainerConfig(
			cName,
			pauseImage,
			WithTestLabels(),
			WithTestAnnotations(),
		)
		cn, err := runtimeService.CreateContainer(sb, containerConfig, sbConfig)
		containerConfigMap[cn] = containerConfig
		require.NoError(t, err)
		defer func() {
			assert.NoError(t, runtimeService.RemoveContainer(cn))
		}()
		require.NoError(t, runtimeService.StartContainer(cn))
		defer func() {
			assert.NoError(t, runtimeService.StopContainer(cn, 10))
		}()
	}

	t.Logf("Fetch container stats for each container with Filter")
	var stats []*runtime.ContainerStats
	for id := range containerConfigMap {
		require.NoError(t, Eventually(func() (bool, error) {
			stats, err = runtimeService.ListContainerStats(
				&runtime.ContainerStatsFilter{Id: id})
			if err != nil {
				return false, err
			}
			if len(stats) != 1 {
				return false, errors.New("unexpected stats length")
			}
			if stats[0].GetWritableLayer().GetUsedBytes().GetValue() != 0 &&
				stats[0].GetWritableLayer().GetInodesUsed().GetValue() != 0 {
				return true, nil
			}
			return false, nil
		}, time.Second, 30*time.Second))

		t.Logf("Verify container stats for %s", id)
		for _, s := range stats {
			require.Equal(t, s.GetAttributes().GetId(), id)
			testStats(t, s, containerConfigMap[id])
		}
	}
}

// Test to verify filtering given a specific Sandbox ID. Stats for
// all the containers in a pod should be returned
func TestContainerListStatsWithSandboxIdFilter(t *testing.T) {
	t.Logf("Create a pod config and run sandbox container")
	sbConfig := PodSandboxConfig("running-pod", "statsls")
	sb, err := runtimeService.RunPodSandbox(sbConfig)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, runtimeService.StopPodSandbox(sb))
		assert.NoError(t, runtimeService.RemovePodSandbox(sb))
	}()
	t.Logf("Create a container config and run containers in a pod")
	containerConfigMap := make(map[string]*runtime.ContainerConfig)
	for i := 0; i < 3; i++ {
		cName := fmt.Sprintf("container%d", i)
		containerConfig := ContainerConfig(
			cName,
			pauseImage,
			WithTestLabels(),
			WithTestAnnotations(),
		)
		cn, err := runtimeService.CreateContainer(sb, containerConfig, sbConfig)
		containerConfigMap[cn] = containerConfig
		require.NoError(t, err)
		defer func() {
			assert.NoError(t, runtimeService.RemoveContainer(cn))
		}()
		require.NoError(t, runtimeService.StartContainer(cn))
		defer func() {
			assert.NoError(t, runtimeService.StopContainer(cn, 10))
		}()
	}

	t.Logf("Fetch container stats for each container with Filter")
	var stats []*runtime.ContainerStats
	require.NoError(t, Eventually(func() (bool, error) {
		stats, err = runtimeService.ListContainerStats(
			&runtime.ContainerStatsFilter{PodSandboxId: sb})
		if err != nil {
			return false, err
		}
		if len(stats) != 3 {
			return false, errors.New("unexpected stats length")
		}
		if stats[0].GetWritableLayer().GetUsedBytes().GetValue() != 0 &&
			stats[0].GetWritableLayer().GetInodesUsed().GetValue() != 0 {
			return true, nil
		}
		return false, nil
	}, time.Second, 30*time.Second))
	t.Logf("Verify container stats for sandbox %q", sb)
	for _, s := range stats {
		testStats(t, s, containerConfigMap[s.GetAttributes().GetId()])
	}
}

// Test to verify filtering given a specific container ID and
// sandbox ID
func TestContainerListStatsWithIdSandboxIdFilter(t *testing.T) {
	t.Logf("Create a pod config and run sandbox container")
	sbConfig := PodSandboxConfig("running-pod", "statsls")
	sb, err := runtimeService.RunPodSandbox(sbConfig)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, runtimeService.StopPodSandbox(sb))
		assert.NoError(t, runtimeService.RemovePodSandbox(sb))
	}()
	t.Logf("Create container config and run containers in a pod")
	containerConfigMap := make(map[string]*runtime.ContainerConfig)
	for i := 0; i < 3; i++ {
		cName := fmt.Sprintf("container%d", i)
		containerConfig := ContainerConfig(
			cName,
			pauseImage,
			WithTestLabels(),
			WithTestAnnotations(),
		)
		cn, err := runtimeService.CreateContainer(sb, containerConfig, sbConfig)
		containerConfigMap[cn] = containerConfig
		require.NoError(t, err)
		defer func() {
			assert.NoError(t, runtimeService.RemoveContainer(cn))
		}()
		require.NoError(t, runtimeService.StartContainer(cn))
		defer func() {
			assert.NoError(t, runtimeService.StopContainer(cn, 10))
		}()
	}
	t.Logf("Fetch container stats for sandbox ID and container ID filter")
	var stats []*runtime.ContainerStats
	for id, config := range containerConfigMap {
		require.NoError(t, Eventually(func() (bool, error) {
			stats, err = runtimeService.ListContainerStats(
				&runtime.ContainerStatsFilter{Id: id, PodSandboxId: sb})
			if err != nil {
				return false, err
			}
			if len(stats) != 1 {
				return false, errors.New("unexpected stats length")
			}
			if stats[0].GetWritableLayer().GetUsedBytes().GetValue() != 0 &&
				stats[0].GetWritableLayer().GetInodesUsed().GetValue() != 0 {
				return true, nil
			}
			return false, nil
		}, time.Second, 30*time.Second))
		t.Logf("Verify container stats for sandbox %q and container %q filter", sb, id)
		for _, s := range stats {
			testStats(t, s, config)
		}
	}

	t.Logf("Fetch container stats for sandbox truncID and container truncID filter ")
	for id, config := range containerConfigMap {
		require.NoError(t, Eventually(func() (bool, error) {
			stats, err = runtimeService.ListContainerStats(
				&runtime.ContainerStatsFilter{Id: id[:3], PodSandboxId: sb[:3]})
			if err != nil {
				return false, err
			}
			if len(stats) != 1 {
				return false, errors.New("unexpected stats length")
			}
			if stats[0].GetWritableLayer().GetUsedBytes().GetValue() != 0 &&
				stats[0].GetWritableLayer().GetInodesUsed().GetValue() != 0 {
				return true, nil
			}
			return false, nil
		}, time.Second, 30*time.Second))
		t.Logf("Verify container stats for sandbox %q and container %q filter", sb, id)
		for _, s := range stats {
			testStats(t, s, config)
		}
	}
}

// TODO make this as options to use for dead container tests
func testStats(t *testing.T,
	s *runtime.ContainerStats,
	config *runtime.ContainerConfig,
) {
	require.NotEmpty(t, s.GetAttributes().GetId())
	require.NotEmpty(t, s.GetAttributes().GetMetadata())
	require.NotEmpty(t, s.GetAttributes().GetAnnotations())
	require.Equal(t, s.GetAttributes().GetLabels(), config.Labels)
	require.Equal(t, s.GetAttributes().GetAnnotations(), config.Annotations)
	require.Equal(t, s.GetAttributes().GetMetadata().Name, config.Metadata.Name)
	require.NotEmpty(t, s.GetAttributes().GetLabels())
	require.NotEmpty(t, s.GetCpu().GetTimestamp())
	require.NotEmpty(t, s.GetCpu().GetUsageCoreNanoSeconds().GetValue())
	require.NotEmpty(t, s.GetMemory().GetTimestamp())
	require.NotEmpty(t, s.GetMemory().GetWorkingSetBytes().GetValue())
	require.NotEmpty(t, s.GetWritableLayer().GetTimestamp())
	require.NotEmpty(t, s.GetWritableLayer().GetFsId().GetMountpoint())
	require.NotEmpty(t, s.GetWritableLayer().GetUsedBytes().GetValue())
	require.NotEmpty(t, s.GetWritableLayer().GetInodesUsed().GetValue())
}

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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1/runtime"
)

// Test to verify for a container ID
func TestContainerStats(t *testing.T) {
	sbConfig, sb := runPod(t, "sandbox1", "stats")
	defer cleanPod(t, sb)

	t.Logf("Create a container config and run container in a pod")
	cn, containerConfig := runContainerInPod(t, "container1", sb, sbConfig)
	defer cleanContainer(t, cn)

	t.Logf("Fetch stats for container")
	var s *runtime.ContainerStats
	require.NoError(t, Eventually(func() (bool, error) {
		var err error
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
	sbConfig, sb := runPod(t, "running-pod", "statsls")
	defer cleanPod(t, sb)

	t.Logf("Create a container config and run containers in a pod")
	containerConfigMap := make(map[string]*runtime.ContainerConfig)
	for i := 0; i < 3; i++ {
		cName := fmt.Sprintf("container%d", i)
		cn, containerConfig := runContainerInPod(t, cName, sb, sbConfig)
		containerConfigMap[cn] = containerConfig
		defer cleanContainer(t, cn)
	}

	t.Logf("Fetch all container stats")
	stats := listContainerStats(t, &runtime.ContainerStatsFilter{}, 3)
	t.Logf("Verify all container stats")
	for _, s := range stats {
		testStats(t, s, containerConfigMap[s.GetAttributes().GetId()])
	}
}

// Test to verify filtering given a specific container ID
// TODO Convert the filter tests into table driven tests and unit tests
func TestContainerListStatsWithIdFilter(t *testing.T) {
	sbConfig, sb := runPod(t, "running-pod", "statsls")
	defer cleanPod(t, sb)

	t.Logf("Create a container config and run containers in a pod")
	containerConfigMap := make(map[string]*runtime.ContainerConfig)
	for i := 0; i < 3; i++ {
		cName := fmt.Sprintf("container%d", i)
		cn, containerConfig := runContainerInPod(t, cName, sb, sbConfig)
		containerConfigMap[cn] = containerConfig
		defer cleanContainer(t, cn)
	}

	t.Logf("Fetch container stats for each container with Filter")
	for id := range containerConfigMap {
		stats := listContainerStats(t, &runtime.ContainerStatsFilter{Id: id}, 1)
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
	sbConfig, sb := runPod(t, "running-pod", "statsls")
	defer cleanPod(t, sb)

	t.Logf("Create a container config and run containers in a pod")
	containerConfigMap := make(map[string]*runtime.ContainerConfig)
	for i := 0; i < 3; i++ {
		cName := fmt.Sprintf("container%d", i)
		cn, containerConfig := runContainerInPod(t, cName, sb, sbConfig)
		containerConfigMap[cn] = containerConfig
		defer cleanContainer(t, cn)
	}

	t.Logf("Fetch container stats for each container with Filter")
	stats := listContainerStats(t, &runtime.ContainerStatsFilter{PodSandboxId: sb}, 3)
	t.Logf("Verify container stats for sandbox %q", sb)
	for _, s := range stats {
		testStats(t, s, containerConfigMap[s.GetAttributes().GetId()])
	}
}

// Test to verify filtering given a specific container ID and
// sandbox ID
func TestContainerListStatsWithIdSandboxIdFilter(t *testing.T) {
	sbConfig, sb := runPod(t, "running-pod", "statsls")
	defer cleanPod(t, sb)

	t.Logf("Create container config and run containers in a pod")
	containerConfigMap := make(map[string]*runtime.ContainerConfig)
	for i := 0; i < 3; i++ {
		cName := fmt.Sprintf("container%d", i)
		cn, containerConfig := runContainerInPod(t, cName, sb, sbConfig)
		containerConfigMap[cn] = containerConfig
		defer cleanContainer(t, cn)
	}

	t.Logf("Fetch container stats for sandbox ID and container ID filter")
	for id, config := range containerConfigMap {
		stats := listContainerStats(t, &runtime.ContainerStatsFilter{Id: id, PodSandboxId: sb}, 1)
		t.Logf("Verify container stats for sandbox %q and container %q filter", sb, id)
		for _, s := range stats {
			testStats(t, s, config)
		}
	}

	t.Logf("Fetch container stats for sandbox truncID and container truncID filter ")
	for id, config := range containerConfigMap {
		stats := listContainerStats(t, &runtime.ContainerStatsFilter{Id: id[:3], PodSandboxId: sb[:3]}, 1)
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
	require.NotEmpty(t, s.GetWritableLayer().GetStorageId().GetUuid())
	require.NotEmpty(t, s.GetWritableLayer().GetUsedBytes().GetValue())
	require.NotEmpty(t, s.GetWritableLayer().GetInodesUsed().GetValue())

}

func runPod(t *testing.T, name, namespace string) (*runtime.PodSandboxConfig, string) {
	t.Logf("Create a pod config and run sandbox container")
	sbConfig := PodSandboxConfig(name, namespace)
	sb, err := runtimeService.RunPodSandbox(sbConfig)
	require.NoError(t, err)
	return sbConfig, sb
}

func cleanPod(t *testing.T, sId string) {
	assert.NoError(t, runtimeService.StopPodSandbox(sId))
	assert.NoError(t, runtimeService.RemovePodSandbox(sId))
}

func runContainerInPod(t *testing.T, cName, sb string,
	sbConfig *runtime.PodSandboxConfig) (string, *runtime.ContainerConfig) {
	containerConfig := ContainerConfig(
		cName,
		pauseImage,
		WithTestLabels(),
		WithTestAnnotations(),
	)
	cId, err := runtimeService.CreateContainer(sb, containerConfig, sbConfig)
	require.NoError(t, err)
	require.NoError(t, runtimeService.StartContainer(cId))
	return cId, containerConfig
}

func cleanContainer(t *testing.T, cId string) {
	assert.NoError(t, runtimeService.StopContainer(cId, 10))
	assert.NoError(t, runtimeService.RemoveContainer(cId))
}

func listContainerStats(t *testing.T, filter *runtime.ContainerStatsFilter, statsLen int) []*runtime.ContainerStats {
	var stats []*runtime.ContainerStats
	require.NoError(t, Eventually(func() (bool, error) {
		stats, err := runtimeService.ListContainerStats(filter)
		if err != nil {
			return false, err
		}
		if len(stats) != statsLen {
			return false, fmt.Errorf("unexpected stats length")
		}

		for _, s := range stats {
			if s.GetWritableLayer().GetUsedBytes().GetValue() == 0 ||
				s.GetWritableLayer().GetInodesUsed().GetValue() == 0 {
				return false, nil
			}
		}
		return true, nil
	}, time.Second, 30*time.Second))

	return stats
}

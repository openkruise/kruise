/*
Copyright 2021 The Kruise Authors.

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

package ephemeraljob

import (
	"testing"
	"time"

	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

func TestPastActiveDeadline(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// Test case 1: No ActiveDeadlineSeconds set
	job := &appsv1alpha1.EphemeralJob{
		Spec: appsv1alpha1.EphemeralJobSpec{},
	}
	g.Expect(pastActiveDeadline(job)).To(gomega.BeFalse())

	// Test case 2: No StartTime set
	deadline := int64(3600)
	job = &appsv1alpha1.EphemeralJob{
		Spec: appsv1alpha1.EphemeralJobSpec{
			ActiveDeadlineSeconds: &deadline,
		},
	}
	g.Expect(pastActiveDeadline(job)).To(gomega.BeFalse())

	// Test case 3: Deadline not exceeded
	startTime := metav1.NewTime(time.Now().Add(-30 * time.Minute))
	job = &appsv1alpha1.EphemeralJob{
		Spec: appsv1alpha1.EphemeralJobSpec{
			ActiveDeadlineSeconds: &deadline,
		},
		Status: appsv1alpha1.EphemeralJobStatus{
			StartTime: &startTime,
		},
	}
	g.Expect(pastActiveDeadline(job)).To(gomega.BeFalse())

	// Test case 4: Deadline exceeded
	startTime = metav1.NewTime(time.Now().Add(-2 * time.Hour))
	job = &appsv1alpha1.EphemeralJob{
		Spec: appsv1alpha1.EphemeralJobSpec{
			ActiveDeadlineSeconds: &deadline,
		},
		Status: appsv1alpha1.EphemeralJobStatus{
			StartTime: &startTime,
		},
	}
	g.Expect(pastActiveDeadline(job)).To(gomega.BeTrue())
}

func TestPodMatchedEphemeralJob(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// Test case 1: Different namespace
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "namespace1",
		},
	}
	ejob := &appsv1alpha1.EphemeralJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ejob",
			Namespace: "namespace2",
		},
	}
	matched, err := podMatchedEphemeralJob(pod, ejob)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(matched).To(gomega.BeFalse())

	// Test case 2: Same namespace, matching selector
	pod = &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels: map[string]string{
				"app": "test",
			},
		},
	}
	ejob = &appsv1alpha1.EphemeralJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ejob",
			Namespace: "default",
		},
		Spec: appsv1alpha1.EphemeralJobSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test",
				},
			},
		},
	}
	matched, err = podMatchedEphemeralJob(pod, ejob)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(matched).To(gomega.BeTrue())

	// Test case 3: Same namespace, non-matching selector
	pod = &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels: map[string]string{
				"app": "other",
			},
		},
	}
	matched, err = podMatchedEphemeralJob(pod, ejob)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(matched).To(gomega.BeFalse())

	// Test case 4: Empty selector
	ejob = &appsv1alpha1.EphemeralJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ejob",
			Namespace: "default",
		},
		Spec: appsv1alpha1.EphemeralJobSpec{},
	}
	matched, err = podMatchedEphemeralJob(pod, ejob)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(matched).To(gomega.BeFalse())
}

func TestAddConditions(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// Test case 1: Add condition to empty list
	conditions := []appsv1alpha1.EphemeralJobCondition{}
	updated := addConditions(conditions, appsv1alpha1.EJobSucceeded, "JobComplete", "Job completed successfully")
	g.Expect(len(updated)).To(gomega.Equal(1))
	g.Expect(updated[0].Type).To(gomega.Equal(appsv1alpha1.EJobSucceeded))
	g.Expect(updated[0].Reason).To(gomega.Equal("JobComplete"))
	g.Expect(updated[0].Message).To(gomega.Equal("Job completed successfully"))

	// Test case 2: Update existing condition
	updated = addConditions(updated, appsv1alpha1.EJobSucceeded, "JobUpdated", "Job updated")
	g.Expect(len(updated)).To(gomega.Equal(1))
	g.Expect(updated[0].Reason).To(gomega.Equal("JobUpdated"))
	g.Expect(updated[0].Message).To(gomega.Equal("Job updated"))

	// Test case 3: Add new condition type
	updated = addConditions(updated, appsv1alpha1.EJobFailed, "JobFailed", "Job failed")
	g.Expect(len(updated)).To(gomega.Equal(2))
	g.Expect(updated[1].Type).To(gomega.Equal(appsv1alpha1.EJobFailed))
	g.Expect(updated[1].Reason).To(gomega.Equal("JobFailed"))
	g.Expect(updated[1].Message).To(gomega.Equal("Job failed"))
}

func TestNewCondition(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	condition := newCondition(appsv1alpha1.EJobSucceeded, "JobComplete", "Job completed successfully")
	g.Expect(condition.Type).To(gomega.Equal(appsv1alpha1.EJobSucceeded))
	g.Expect(condition.Status).To(gomega.Equal(v1.ConditionTrue))
	g.Expect(condition.Reason).To(gomega.Equal("JobComplete"))
	g.Expect(condition.Message).To(gomega.Equal("Job completed successfully"))
	g.Expect(condition.LastProbeTime).NotTo(gomega.BeNil())
	g.Expect(condition.LastTransitionTime).NotTo(gomega.BeNil())
}

func TestGetEphemeralContainersMaps(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// Test case 1: Empty containers
	containers := []v1.EphemeralContainer{}
	containerMap, empty := getEphemeralContainersMaps(containers)
	g.Expect(empty).To(gomega.BeTrue())
	g.Expect(len(containerMap)).To(gomega.Equal(0))

	// Test case 2: Non-empty containers
	containers = []v1.EphemeralContainer{
		{
			EphemeralContainerCommon: v1.EphemeralContainerCommon{
				Name:  "container1",
				Image: "busybox",
			},
		},
		{
			EphemeralContainerCommon: v1.EphemeralContainerCommon{
				Name:  "container2",
				Image: "alpine",
			},
		},
	}
	containerMap, empty = getEphemeralContainersMaps(containers)
	g.Expect(empty).To(gomega.BeFalse())
	g.Expect(len(containerMap)).To(gomega.Equal(2))
	g.Expect(containerMap["container1"].Name).To(gomega.Equal("container1"))
	g.Expect(containerMap["container2"].Name).To(gomega.Equal("container2"))
}

func TestGetPodEphemeralContainers(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
		},
	}

	ejob := &appsv1alpha1.EphemeralJob{
		Spec: appsv1alpha1.EphemeralJobSpec{
			Template: appsv1alpha1.EphemeralContainerTemplateSpec{
				EphemeralContainers: []v1.EphemeralContainer{
					{
						EphemeralContainerCommon: v1.EphemeralContainerCommon{
							Name:  "container1",
							Image: "busybox",
						},
					},
					{
						EphemeralContainerCommon: v1.EphemeralContainerCommon{
							Name:  "container2",
							Image: "alpine",
						},
					},
				},
			},
		},
	}

	names := getPodEphemeralContainers(pod, ejob)
	g.Expect(len(names)).To(gomega.Equal(2))
	g.Expect(names[0]).To(gomega.Equal("test-pod-container1"))
	g.Expect(names[1]).To(gomega.Equal("test-pod-container2"))
}

func TestExistDuplicatedEphemeralContainer(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	job := &appsv1alpha1.EphemeralJob{
		ObjectMeta: metav1.ObjectMeta{
			UID: types.UID("job-uid"),
		},
		Spec: appsv1alpha1.EphemeralJobSpec{
			Template: appsv1alpha1.EphemeralContainerTemplateSpec{
				EphemeralContainers: []v1.EphemeralContainer{
					{
						EphemeralContainerCommon: v1.EphemeralContainerCommon{
							Name:  "test-container",
							Image: "busybox",
						},
					},
				},
			},
		},
	}

	// Test case 1: No duplicated containers
	targetPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
		},
		Spec: v1.PodSpec{
			EphemeralContainers: []v1.EphemeralContainer{
				{
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name:  "test-container",
						Image: "busybox",
						Env: []v1.EnvVar{
							{
								Name:  appsv1alpha1.EphemeralContainerEnvKey,
								Value: "job-uid",
							},
						},
					},
				},
			},
		},
	}
	g.Expect(existDuplicatedEphemeralContainer(job, targetPod)).To(gomega.BeFalse())

	// Test case 2: Duplicated container not created by this job
	targetPod = &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
		},
		Spec: v1.PodSpec{
			EphemeralContainers: []v1.EphemeralContainer{
				{
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name:  "test-container",
						Image: "busybox",
						Env: []v1.EnvVar{
							{
								Name:  appsv1alpha1.EphemeralContainerEnvKey,
								Value: "other-job-uid",
							},
						},
					},
				},
			},
		},
	}
	g.Expect(existDuplicatedEphemeralContainer(job, targetPod)).To(gomega.BeTrue())
}

func TestIsCreatedByEJob(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// Test case 1: Container created by job
	container := v1.EphemeralContainer{
		EphemeralContainerCommon: v1.EphemeralContainerCommon{
			Name:  "test-container",
			Image: "busybox",
			Env: []v1.EnvVar{
				{
					Name:  appsv1alpha1.EphemeralContainerEnvKey,
					Value: "job-uid",
				},
			},
		},
	}
	g.Expect(isCreatedByEJob("job-uid", container)).To(gomega.BeTrue())

	// Test case 2: Container not created by job
	container = v1.EphemeralContainer{
		EphemeralContainerCommon: v1.EphemeralContainerCommon{
			Name:  "test-container",
			Image: "busybox",
			Env: []v1.EnvVar{
				{
					Name:  appsv1alpha1.EphemeralContainerEnvKey,
					Value: "other-job-uid",
				},
			},
		},
	}
	g.Expect(isCreatedByEJob("job-uid", container)).To(gomega.BeFalse())

	// Test case 3: No environment variable
	container = v1.EphemeralContainer{
		EphemeralContainerCommon: v1.EphemeralContainerCommon{
			Name:  "test-container",
			Image: "busybox",
		},
	}
	g.Expect(isCreatedByEJob("job-uid", container)).To(gomega.BeFalse())
}

func TestGetSyncPods(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	job := &appsv1alpha1.EphemeralJob{
		ObjectMeta: metav1.ObjectMeta{
			UID: types.UID("job-uid"),
		},
		Spec: appsv1alpha1.EphemeralJobSpec{
			Template: appsv1alpha1.EphemeralContainerTemplateSpec{
				EphemeralContainers: []v1.EphemeralContainer{
					{
						EphemeralContainerCommon: v1.EphemeralContainerCommon{
							Name:  "test-container",
							Image: "busybox",
						},
					},
				},
			},
		},
	}

	// Pod without ephemeral containers
	pod1 := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod1",
		},
	}

	// Pod with ephemeral containers from this job
	pod2 := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod2",
		},
		Spec: v1.PodSpec{
			EphemeralContainers: []v1.EphemeralContainer{
				{
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name:  "test-container",
						Image: "busybox",
						Env: []v1.EnvVar{
							{
								Name:  appsv1alpha1.EphemeralContainerEnvKey,
								Value: "job-uid",
							},
						},
					},
				},
			},
		},
	}

	// Pod with ephemeral containers from different job
	pod3 := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod3",
		},
		Spec: v1.PodSpec{
			EphemeralContainers: []v1.EphemeralContainer{
				{
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name:  "test-container",
						Image: "busybox",
						Env: []v1.EnvVar{
							{
								Name:  appsv1alpha1.EphemeralContainerEnvKey,
								Value: "other-job-uid",
							},
						},
					},
				},
			},
		},
	}

	pods := []*v1.Pod{pod1, pod2, pod3}
	toCreate, toUpdate, toDelete := getSyncPods(job, pods)

	g.Expect(len(toCreate)).To(gomega.Equal(2)) // pod1 and pod3
	g.Expect(len(toUpdate)).To(gomega.Equal(0))
	g.Expect(len(toDelete)).To(gomega.Equal(0))
	g.Expect(toCreate[0].Name).To(gomega.Equal("pod1"))
	g.Expect(toCreate[1].Name).To(gomega.Equal("pod3"))
}

func TestParseEphemeralContainerStatus(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// Test case 1: Terminated with exit code 0
	status := &v1.ContainerStatus{
		Name: "test-container",
		State: v1.ContainerState{
			Terminated: &v1.ContainerStateTerminated{
				ExitCode:   0,
				FinishedAt: metav1.Now(),
			},
		},
	}
	g.Expect(parseEphemeralContainerStatus(status)).To(gomega.Equal(SucceededStatus))

	// Test case 2: Terminated with non-zero exit code
	status = &v1.ContainerStatus{
		Name: "test-container",
		State: v1.ContainerState{
			Terminated: &v1.ContainerStateTerminated{
				ExitCode: 1,
			},
		},
	}
	g.Expect(parseEphemeralContainerStatus(status)).To(gomega.Equal(FailedStatus))

	// Test case 3: Running state
	status = &v1.ContainerStatus{
		Name: "test-container",
		State: v1.ContainerState{
			Running: &v1.ContainerStateRunning{
				StartedAt: metav1.Now(),
			},
		},
	}
	g.Expect(parseEphemeralContainerStatus(status)).To(gomega.Equal(RunningStatus))

	// Test case 4: Waiting state
	status = &v1.ContainerStatus{
		Name: "test-container",
		State: v1.ContainerState{
			Waiting: &v1.ContainerStateWaiting{
				Reason: "ImagePullBackOff",
			},
		},
	}
	g.Expect(parseEphemeralContainerStatus(status)).To(gomega.Equal(WaitingStatus))

	// Test case 5: Waiting with RunContainerError
	status = &v1.ContainerStatus{
		Name: "test-container",
		State: v1.ContainerState{
			Waiting: &v1.ContainerStateWaiting{
				Reason: "RunContainerError",
			},
		},
	}
	g.Expect(parseEphemeralContainerStatus(status)).To(gomega.Equal(FailedStatus))
}

func TestHasEphemeralContainerFinalizer(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// Test case 1: Empty finalizers
	finalizers := []string{}
	g.Expect(hasEphemeralContainerFinalizer(finalizers)).To(gomega.BeFalse())

	// Test case 2: Finalizer not present
	finalizers = []string{"other-finalizer"}
	g.Expect(hasEphemeralContainerFinalizer(finalizers)).To(gomega.BeFalse())

	// Test case 3: Finalizer present
	finalizers = []string{"other-finalizer", EphemeralContainerFinalizer}
	g.Expect(hasEphemeralContainerFinalizer(finalizers)).To(gomega.BeTrue())
}

func TestDeleteEphemeralContainerFinalizer(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// Test case 1: Empty finalizers
	finalizers := []string{}
	result := deleteEphemeralContainerFinalizer(finalizers, EphemeralContainerFinalizer)
	g.Expect(len(result)).To(gomega.Equal(0))

	// Test case 2: Finalizer not present
	finalizers = []string{"other-finalizer"}
	result = deleteEphemeralContainerFinalizer(finalizers, EphemeralContainerFinalizer)
	g.Expect(len(result)).To(gomega.Equal(1))
	g.Expect(result[0]).To(gomega.Equal("other-finalizer"))

	// Test case 3: Finalizer present
	finalizers = []string{"other-finalizer", EphemeralContainerFinalizer, "another-finalizer"}
	result = deleteEphemeralContainerFinalizer(finalizers, EphemeralContainerFinalizer)
	g.Expect(len(result)).To(gomega.Equal(2))
	g.Expect(result).To(gomega.ContainElement("other-finalizer"))
	g.Expect(result).To(gomega.ContainElement("another-finalizer"))
	g.Expect(result).NotTo(gomega.ContainElement(EphemeralContainerFinalizer))
}

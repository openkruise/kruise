/*
Copyright 2025 The Kruise Authors.

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

package v1alpha1

import (
	"context"
	"encoding/json"
	"time"

	"github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientset "k8s.io/client-go/kubernetes"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/test/e2e/framework/common"
)

type AdvancedCronJobTester struct {
	c  clientset.Interface
	kc kruiseclientset.Interface
	ns string
}

func NewAdvancedCronJobTester(c clientset.Interface, kc kruiseclientset.Interface, ns string) *AdvancedCronJobTester {
	return &AdvancedCronJobTester{
		c:  c,
		kc: kc,
		ns: ns,
	}
}

func (t *AdvancedCronJobTester) CreateAdvancedCronJob(acj *appsv1alpha1.AdvancedCronJob) (*appsv1alpha1.AdvancedCronJob, error) {
	acj.Namespace = t.ns
	common.Logf("Creating AdvancedCronJob %s/%s", acj.Namespace, acj.Name)
	created, err := t.kc.AppsV1alpha1().AdvancedCronJobs(t.ns).Create(context.TODO(), acj, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	t.WaitForAdvancedCronJobCreated(created)
	return created, nil
}

func (t *AdvancedCronJobTester) GetAdvancedCronJob(name string) (*appsv1alpha1.AdvancedCronJob, error) {
	return t.kc.AppsV1alpha1().AdvancedCronJobs(t.ns).Get(context.TODO(), name, metav1.GetOptions{})
}

func (t *AdvancedCronJobTester) UpdateAdvancedCronJob(acj *appsv1alpha1.AdvancedCronJob) (*appsv1alpha1.AdvancedCronJob, error) {
	return t.kc.AppsV1alpha1().AdvancedCronJobs(t.ns).Update(context.TODO(), acj, metav1.UpdateOptions{})
}

func (t *AdvancedCronJobTester) PatchAdvancedCronJobPaused(name string, paused bool) (*appsv1alpha1.AdvancedCronJob, error) {
	patchData := map[string]interface{}{
		"spec": map[string]interface{}{
			"paused": paused,
		},
	}

	patchBytes, err := json.Marshal(patchData)
	if err != nil {
		return nil, err
	}

	return t.kc.AppsV1alpha1().AdvancedCronJobs(t.ns).Patch(context.TODO(), name, types.MergePatchType, patchBytes, metav1.PatchOptions{})
}

func (t *AdvancedCronJobTester) DeleteAdvancedCronJob(name string) error {
	common.Logf("Deleting AdvancedCronJob %s/%s", t.ns, name)
	err := t.kc.AppsV1alpha1().AdvancedCronJobs(t.ns).Delete(context.TODO(), name, metav1.DeleteOptions{})
	if err != nil {
		return err
	}
	t.WaitForAdvancedCronJobDeleted(name)
	return nil
}

func (t *AdvancedCronJobTester) WaitForAdvancedCronJobCreated(acj *appsv1alpha1.AdvancedCronJob) {
	gomega.Eventually(func() bool {
		_, err := t.GetAdvancedCronJob(acj.Name)
		return err == nil
	}, 30*time.Second, time.Second).Should(gomega.BeTrue())
}

func (t *AdvancedCronJobTester) WaitForAdvancedCronJobDeleted(name string) {
	gomega.Eventually(func() bool {
		_, err := t.GetAdvancedCronJob(name)
		return err != nil
	}, 30*time.Second, time.Second).Should(gomega.BeTrue())
}

func (t *AdvancedCronJobTester) WaitForAdvancedCronJobToHaveActiveJobs(acj *appsv1alpha1.AdvancedCronJob, expectedCount int) {
	gomega.Eventually(func() int {
		updated, err := t.GetAdvancedCronJob(acj.Name)
		if err != nil {
			return 0
		}
		return len(updated.Status.Active)
	}, 60*time.Second, 2*time.Second).Should(gomega.Equal(expectedCount))
}

func (t *AdvancedCronJobTester) WaitForAdvancedCronJobToHaveLastScheduleTime(acj *appsv1alpha1.AdvancedCronJob) {
	gomega.Eventually(func() bool {
		updated, err := t.GetAdvancedCronJob(acj.Name)
		if err != nil {
			return false
		}
		return updated.Status.LastScheduleTime != nil
	}, 180*time.Second, 2*time.Second).Should(gomega.BeTrue())
}

func (t *AdvancedCronJobTester) GetJobsCreatedByAdvancedCronJob(acj *appsv1alpha1.AdvancedCronJob) ([]batchv1.Job, error) {
	// List all Jobs in the namespace
	jobs, err := t.c.BatchV1().Jobs(t.ns).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	// Filter by owner reference
	var ownedJobs []batchv1.Job
	for _, job := range jobs.Items {
		controllerRef := metav1.GetControllerOf(&job)
		if controllerRef != nil && controllerRef.UID == acj.UID {
			ownedJobs = append(ownedJobs, job)
		}
	}
	return ownedJobs, nil
}

func (t *AdvancedCronJobTester) GetBroadcastJobsCreatedByAdvancedCronJob(acj *appsv1alpha1.AdvancedCronJob) ([]appsv1alpha1.BroadcastJob, error) {
	// List all BroadcastJobs in the namespace
	broadcastJobs, err := t.kc.AppsV1alpha1().BroadcastJobs(t.ns).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	// Filter by owner reference
	var ownedBroadcastJobs []appsv1alpha1.BroadcastJob
	for _, job := range broadcastJobs.Items {
		controllerRef := metav1.GetControllerOf(&job)
		if controllerRef != nil && controllerRef.UID == acj.UID {
			ownedBroadcastJobs = append(ownedBroadcastJobs, job)
		}
	}
	return ownedBroadcastJobs, nil
}

func (t *AdvancedCronJobTester) CreateTestAdvancedCronJobWithJobTemplate(name, schedule string) *appsv1alpha1.AdvancedCronJob {
	acj := &appsv1alpha1.AdvancedCronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: t.ns,
		},
		Spec: appsv1alpha1.AdvancedCronJobSpec{
			Schedule:                   schedule,
			ConcurrencyPolicy:          appsv1alpha1.AllowConcurrent,
			SuccessfulJobsHistoryLimit: int32Ptr(3),
			FailedJobsHistoryLimit:     int32Ptr(1),
			Template: appsv1alpha1.CronJobTemplate{
				JobTemplate: &batchv1.JobTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"cronjob": name,
						},
					},
					Spec: batchv1.JobSpec{
						Template: v1.PodTemplateSpec{
							Spec: v1.PodSpec{
								Containers: []v1.Container{
									{
										Name:    "test-container",
										Image:   common.BusyboxImage,
										Command: []string{"/bin/sh", "-c", "echo 'Hello from AdvancedCronJob' && sleep 5"},
									},
								},
								RestartPolicy: v1.RestartPolicyNever,
							},
						},
					},
				},
			},
		},
	}

	created, err := t.CreateAdvancedCronJob(acj)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return created
}

func (t *AdvancedCronJobTester) CreateTestAdvancedCronJobWithBroadcastJobTemplate(name, schedule string) *appsv1alpha1.AdvancedCronJob {
	acj := &appsv1alpha1.AdvancedCronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: t.ns,
		},
		Spec: appsv1alpha1.AdvancedCronJobSpec{
			Schedule:                   schedule,
			ConcurrencyPolicy:          appsv1alpha1.AllowConcurrent,
			SuccessfulJobsHistoryLimit: int32Ptr(3),
			FailedJobsHistoryLimit:     int32Ptr(1),
			Template: appsv1alpha1.CronJobTemplate{
				BroadcastJobTemplate: &appsv1alpha1.BroadcastJobTemplateSpec{
					Spec: appsv1alpha1.BroadcastJobSpec{
						Parallelism: intstrPtr("100%"),
						Template: v1.PodTemplateSpec{
							Spec: v1.PodSpec{
								Containers: []v1.Container{
									{
										Name:    "test-container",
										Image:   common.BusyboxImage,
										Command: []string{"/bin/sh", "-c", "echo 'Hello from AdvancedCronJob BroadcastJob' && sleep 5"},
									},
								},
								RestartPolicy: v1.RestartPolicyNever,
							},
						},
						CompletionPolicy: appsv1alpha1.CompletionPolicy{
							Type: appsv1alpha1.Always,
						},
					},
				},
			},
		},
	}

	created, err := t.CreateAdvancedCronJob(acj)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return created
}

func (t *AdvancedCronJobTester) CreateTestAdvancedCronJobWithConcurrencyPolicy(name, schedule string, policy appsv1alpha1.ConcurrencyPolicy) *appsv1alpha1.AdvancedCronJob {
	acj := &appsv1alpha1.AdvancedCronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: t.ns,
		},
		Spec: appsv1alpha1.AdvancedCronJobSpec{
			Schedule:                   schedule,
			ConcurrencyPolicy:          policy,
			SuccessfulJobsHistoryLimit: int32Ptr(3),
			FailedJobsHistoryLimit:     int32Ptr(1),
			Template: appsv1alpha1.CronJobTemplate{
				JobTemplate: &batchv1.JobTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"cronjob": name,
						},
					},
					Spec: batchv1.JobSpec{
						Template: v1.PodTemplateSpec{
							Spec: v1.PodSpec{
								Containers: []v1.Container{
									{
										Name:    "test-container",
										Image:   common.BusyboxImage,
										Command: []string{"/bin/sh", "-c", "echo 'Hello from AdvancedCronJob' && sleep 30"},
									},
								},
								RestartPolicy: v1.RestartPolicyNever,
							},
						},
					},
				},
			},
		},
	}

	created, err := t.CreateAdvancedCronJob(acj)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return created
}

func (t *AdvancedCronJobTester) WaitForJobToComplete(jobName string) {
	gomega.Eventually(func() bool {
		job, err := t.c.BatchV1().Jobs(t.ns).Get(context.TODO(), jobName, metav1.GetOptions{})
		if err != nil {
			return false
		}
		return job.Status.Succeeded > 0 || job.Status.Failed > 0
	}, 60*time.Second, 2*time.Second).Should(gomega.BeTrue())
}

func (t *AdvancedCronJobTester) WaitForBroadcastJobToComplete(broadcastJobName string) {
	gomega.Eventually(func() bool {
		broadcastJob, err := t.kc.AppsV1alpha1().BroadcastJobs(t.ns).Get(context.TODO(), broadcastJobName, metav1.GetOptions{})
		if err != nil {
			return false
		}
		return broadcastJob.Status.Succeeded > 0 || broadcastJob.Status.Failed > 0
	}, 60*time.Second, 2*time.Second).Should(gomega.BeTrue())
}

func (t *AdvancedCronJobTester) DeleteAllAdvancedCronJobs() {
	acjList, err := t.kc.AppsV1alpha1().AdvancedCronJobs(t.ns).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		common.Logf("Failed to list AdvancedCronJobs: %v", err)
		return
	}

	for _, acj := range acjList.Items {
		t.DeleteAdvancedCronJob(acj.Name)
	}
}

// Helper functions
func int32Ptr(i int32) *int32 { return &i }
func int64Ptr(i int64) *int64 { return &i }
func boolPtr(b bool) *bool    { return &b }
func intstrPtr(s string) *intstr.IntOrString {
	val := intstr.FromString(s)
	return &val
}

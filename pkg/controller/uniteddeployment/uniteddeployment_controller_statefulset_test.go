/*
Copyright 2019 The Kruise Authors.

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

package uniteddeployment

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	kruisectlutil "github.com/openkruise/kruise/pkg/controller/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
)

var c client.Client

const timeout = time.Second * 2

var (
	one int32 = 1
	two int32 = 2
	ten int32 = 10
)

func TestStsReconcile(t *testing.T) {
	g, requests, cancel, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		cancel()
		mgrStopped.Wait()
	}()

	caseName := "sts-reconcile"
	instance := &appsv1alpha1.UnitedDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      caseName,
			Namespace: "default",
		},
		Spec: appsv1alpha1.UnitedDeploymentSpec{
			Replicas: &one,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": caseName,
				},
			},
			Template: appsv1alpha1.SubsetTemplate{
				StatefulSetTemplate: &appsv1alpha1.StatefulSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1.StatefulSetSpec{
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"name": caseName,
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "container-a",
										Image: "nginx:1.0",
									},
								},
							},
						},
					},
				},
			},
			Topology: appsv1alpha1.Topology{
				Subsets: []appsv1alpha1.Subset{
					{
						Name: "subset-a",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"node-a"},
								},
							},
						},
					},
				},
			},
			RevisionHistoryLimit: &ten,
		},
	}

	// Create the UnitedDeployment object and expect the Reconcile and Deployment to be created
	err := c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	waitReconcilerProcessFinished(g, requests, 3)
	expectedStsCount(g, instance, 1)
}

func TestStsSubsetProvision(t *testing.T) {
	g, requests, cancel, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		cancel()
		mgrStopped.Wait()
	}()

	caseName := "test-sts-subset-provision"
	instance := &appsv1alpha1.UnitedDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      caseName,
			Namespace: "default",
		},
		Spec: appsv1alpha1.UnitedDeploymentSpec{
			Replicas: &one,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": caseName,
				},
			},
			Template: appsv1alpha1.SubsetTemplate{
				StatefulSetTemplate: &appsv1alpha1.StatefulSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1.StatefulSetSpec{
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"name": caseName,
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "container-a",
										Image: "nginx:1.0",
									},
								},
							},
						},
					},
				},
			},
			Topology: appsv1alpha1.Topology{
				Subsets: []appsv1alpha1.Subset{
					{
						Name: "subset-a",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"node-a"},
								},
							},
						},
					},
				},
			},
			RevisionHistoryLimit: &ten,
		},
	}

	// Create the UnitedDeployment object and expect the Reconcile and Deployment to be created
	err := c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	waitReconcilerProcessFinished(g, requests, 3)

	stsList := expectedStsCount(g, instance, 1)
	sts := &stsList.Items[0]
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution).ShouldNot(gomega.BeNil())
	g.Expect(len(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms)).Should(gomega.BeEquivalentTo(1))
	g.Expect(len(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions)).Should(gomega.BeEquivalentTo(1))
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Key).Should(gomega.BeEquivalentTo("node-name"))
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpIn))
	g.Expect(len(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values)).Should(gomega.BeEquivalentTo(1))
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values[0]).Should(gomega.BeEquivalentTo("node-a"))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	instance.Spec.Topology.Subsets = append(instance.Spec.Topology.Subsets, appsv1alpha1.Subset{
		Name: "subset-b",
		NodeSelectorTerm: corev1.NodeSelectorTerm{
			MatchExpressions: []corev1.NodeSelectorRequirement{
				{
					Key:      "node-name",
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{"node-b"},
				},
			},
		},
	})
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	stsList = expectedStsCount(g, instance, 2)
	sts = getSubsetByName(stsList, "subset-a")
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution).ShouldNot(gomega.BeNil())
	g.Expect(len(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms)).Should(gomega.BeEquivalentTo(1))
	g.Expect(len(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions)).Should(gomega.BeEquivalentTo(1))
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Key).Should(gomega.BeEquivalentTo("node-name"))
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpIn))
	g.Expect(len(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values)).Should(gomega.BeEquivalentTo(1))
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values[0]).Should(gomega.BeEquivalentTo("node-a"))

	sts = getSubsetByName(stsList, "subset-b")
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution).ShouldNot(gomega.BeNil())
	g.Expect(len(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms)).Should(gomega.BeEquivalentTo(1))
	g.Expect(len(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions)).Should(gomega.BeEquivalentTo(1))
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Key).Should(gomega.BeEquivalentTo("node-name"))
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpIn))
	g.Expect(len(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values)).Should(gomega.BeEquivalentTo(1))
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values[0]).Should(gomega.BeEquivalentTo("node-b"))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	instance.Spec.Topology.Subsets = instance.Spec.Topology.Subsets[1:]
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	stsList = expectedStsCount(g, instance, 1)
	sts = &stsList.Items[0]
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution).ShouldNot(gomega.BeNil())
	g.Expect(len(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms)).Should(gomega.BeEquivalentTo(1))
	g.Expect(len(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions)).Should(gomega.BeEquivalentTo(1))
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Key).Should(gomega.BeEquivalentTo("node-name"))
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpIn))
	g.Expect(len(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values)).Should(gomega.BeEquivalentTo(1))
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values[0]).Should(gomega.BeEquivalentTo("node-b"))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	instance.Spec.Template.StatefulSetTemplate.Spec.Template.Spec.Affinity = &corev1.Affinity{}
	instance.Spec.Template.StatefulSetTemplate.Spec.Template.Spec.Affinity.NodeAffinity = &corev1.NodeAffinity{}

	nodeSelector := &corev1.NodeSelector{}
	nodeSelector.NodeSelectorTerms = append(nodeSelector.NodeSelectorTerms, corev1.NodeSelectorTerm{
		MatchExpressions: []corev1.NodeSelectorRequirement{
			{
				Key:      "test",
				Operator: corev1.NodeSelectorOpExists,
			},
			{
				Key:      "region",
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{caseName},
			},
		},
	})
	nodeSelector.NodeSelectorTerms = append(nodeSelector.NodeSelectorTerms, corev1.NodeSelectorTerm{
		MatchExpressions: []corev1.NodeSelectorRequirement{
			{
				Key:      "test",
				Operator: corev1.NodeSelectorOpDoesNotExist,
			},
		},
	})
	instance.Spec.Template.StatefulSetTemplate.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = nodeSelector

	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	stsList = expectedStsCount(g, instance, 1)
	sts = &stsList.Items[0]
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution).ShouldNot(gomega.BeNil())
	g.Expect(len(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms)).Should(gomega.BeEquivalentTo(2))

	g.Expect(len(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions)).Should(gomega.BeEquivalentTo(3))
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Key).Should(gomega.BeEquivalentTo("test"))
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpExists))
	g.Expect(len(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values)).Should(gomega.BeEquivalentTo(0))
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[1].Key).Should(gomega.BeEquivalentTo("region"))
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[1].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpIn))
	g.Expect(len(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[1].Values)).Should(gomega.BeEquivalentTo(1))
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[1].Values[0]).Should(gomega.BeEquivalentTo(caseName))
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[2].Key).Should(gomega.BeEquivalentTo("node-name"))
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[2].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpIn))
	g.Expect(len(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[2].Values)).Should(gomega.BeEquivalentTo(1))
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[2].Values[0]).Should(gomega.BeEquivalentTo("node-b"))

	g.Expect(len(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[1].MatchExpressions)).Should(gomega.BeEquivalentTo(2))
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[1].MatchExpressions[0].Key).Should(gomega.BeEquivalentTo("test"))
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[1].MatchExpressions[0].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpDoesNotExist))
	g.Expect(len(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[1].MatchExpressions[0].Values)).Should(gomega.BeEquivalentTo(0))
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[1].MatchExpressions[1].Key).Should(gomega.BeEquivalentTo("node-name"))
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[1].MatchExpressions[1].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpIn))
	g.Expect(len(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[1].MatchExpressions[1].Values)).Should(gomega.BeEquivalentTo(1))
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[1].MatchExpressions[1].Values[0]).Should(gomega.BeEquivalentTo("node-b"))
}

func TestStsSubsetPatch(t *testing.T) {
	g, requests, cancel, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		cancel()
		mgrStopped.Wait()
	}()

	caseName := "test-sts-subset-patch"

	imagePatch := map[string]interface{}{
		"spec": map[string]interface{}{
			"containers": []map[string]interface{}{
				{
					"name":  "container-a",
					"image": "nginx:2.0",
				},
			},
		},
	}
	labelPatch := map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": map[string]string{
				"zone": "a",
			},
		},
	}
	resourcePatch := map[string]interface{}{
		"spec": map[string]interface{}{
			"containers": []map[string]interface{}{
				{
					"name": "container-a",
					"resources": map[string]interface{}{
						"limits": map[string]interface{}{
							"cpu":    "2",
							"memory": "800Mi",
						},
					},
				},
			},
		},
	}
	envPatch := map[string]interface{}{
		"spec": map[string]interface{}{
			"containers": []map[string]interface{}{
				{
					"name": "container-a",
					"env": []map[string]string{
						{
							"name":  "K8S_CONTAINER_NAME",
							"value": "main",
						},
					},
				},
			},
		},
	}
	labelPatchBytes, _ := json.Marshal(labelPatch)
	imagePatchBytes, _ := json.Marshal(imagePatch)
	resourcePatchBytes, _ := json.Marshal(resourcePatch)
	envPatchBytes, _ := json.Marshal(envPatch)
	instance := &appsv1alpha1.UnitedDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      caseName,
			Namespace: "default",
		},
		Spec: appsv1alpha1.UnitedDeploymentSpec{
			Replicas: &one,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": caseName,
				},
			},
			Template: appsv1alpha1.SubsetTemplate{
				StatefulSetTemplate: &appsv1alpha1.StatefulSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1.StatefulSetSpec{
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"name": caseName,
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "container-a",
										Image: "nginx:1.0",
									},
								},
							},
						},
					},
				},
			},
			Topology: appsv1alpha1.Topology{
				Subsets: []appsv1alpha1.Subset{
					{
						Name: "subset-a",
						Patch: runtime.RawExtension{
							Raw: imagePatchBytes,
						},
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"node-a"},
								},
							},
						},
					},
					{
						Name: "subset-b",
						Patch: runtime.RawExtension{
							Raw: labelPatchBytes,
						},
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"node-b"},
								},
							},
						},
					},
					{
						Name: "subset-c",
						Patch: runtime.RawExtension{
							Raw: resourcePatchBytes,
						},
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"node-c"},
								},
							},
						},
					},
					{
						Name: "subset-d",
						Patch: runtime.RawExtension{
							Raw: envPatchBytes,
						},
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"node-d"},
								},
							},
						},
					},
				},
			},
			RevisionHistoryLimit: &ten,
		},
	}

	// Create the UnitedDeployment object and expect the Reconcile and Deployment to be created
	err := c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	waitReconcilerProcessFinished(g, requests, 3)

	stsList := expectedStsCount(g, instance, 4)
	sts := getSubsetByName(stsList, "subset-a")
	g.Expect(sts.Spec).ShouldNot(gomega.BeNil())
	g.Expect(sts.Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))

	sts = getSubsetByName(stsList, "subset-b")
	g.Expect(sts.Spec).ShouldNot(gomega.BeNil())
	g.Expect(sts.Spec.Template.Labels).Should(gomega.HaveKeyWithValue("zone", "a"))

	sts = getSubsetByName(stsList, "subset-c")
	g.Expect(sts.Spec).ShouldNot(gomega.BeNil())
	g.Expect(sts.Spec.Template.Spec.Containers[0].Resources).ShouldNot(gomega.BeNil())
	g.Expect(sts.Spec.Template.Spec.Containers[0].Resources.Limits).ShouldNot(gomega.BeNil())
	g.Expect(sts.Spec.Template.Spec.Containers[0].Resources.Limits.Cpu()).ShouldNot(gomega.BeNil())
	g.Expect(sts.Spec.Template.Spec.Containers[0].Resources.Limits.Cpu().Value()).Should(gomega.BeEquivalentTo(2))
	g.Expect(sts.Spec.Template.Spec.Containers[0].Resources.Limits.Memory()).ShouldNot(gomega.BeNil())
	g.Expect(sts.Spec.Template.Spec.Containers[0].Resources.Limits.Memory().String()).Should(gomega.BeEquivalentTo("800Mi"))

	sts = getSubsetByName(stsList, "subset-d")
	g.Expect(sts.Spec).ShouldNot(gomega.BeNil())
	g.Expect(sts.Spec.Template.Spec.Containers[0].Env).ShouldNot(gomega.BeNil())
	g.Expect(sts.Spec.Template.Spec.Containers[0].Env[0].Name).Should(gomega.BeEquivalentTo("K8S_CONTAINER_NAME"))
	g.Expect(sts.Spec.Template.Spec.Containers[0].Env[0].Value).Should(gomega.BeEquivalentTo("main"))
}

func TestStsSubsetProvisionWithToleration(t *testing.T) {
	g, requests, cancel, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		cancel()
		mgrStopped.Wait()
	}()

	caseName := "test-sts-subset-provision-with-toleration"
	instance := &appsv1alpha1.UnitedDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      caseName,
			Namespace: "default",
		},
		Spec: appsv1alpha1.UnitedDeploymentSpec{
			Replicas: &one,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": caseName,
				},
			},
			Template: appsv1alpha1.SubsetTemplate{
				StatefulSetTemplate: &appsv1alpha1.StatefulSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1.StatefulSetSpec{
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"name": caseName,
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "container-a",
										Image: "nginx:1.0",
									},
								},
							},
						},
					},
				},
			},
			Topology: appsv1alpha1.Topology{
				Subsets: []appsv1alpha1.Subset{
					{
						Name: "subset-a",
						Tolerations: []corev1.Toleration{
							{
								Key:      "taint-a",
								Operator: corev1.TolerationOpExists,
								Effect:   corev1.TaintEffectNoSchedule,
							},
							{
								Key:      "taint-b",
								Operator: corev1.TolerationOpEqual,
								Value:    "taint-b-value",
							},
						},
					},
				},
			},
			RevisionHistoryLimit: &ten,
		},
	}

	// Create the UnitedDeployment object and expect the Reconcile and Deployment to be created
	err := c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	waitReconcilerProcessFinished(g, requests, 3)

	stsList := expectedStsCount(g, instance, 1)
	sts := &stsList.Items[0]
	g.Expect(sts.Spec.Template.Spec.Tolerations).ShouldNot(gomega.BeNil())
	g.Expect(len(sts.Spec.Template.Spec.Tolerations)).Should(gomega.BeEquivalentTo(2))
	g.Expect(reflect.DeepEqual(sts.Spec.Template.Spec.Tolerations[0], instance.Spec.Topology.Subsets[0].Tolerations[0])).Should(gomega.BeTrue())
	g.Expect(reflect.DeepEqual(sts.Spec.Template.Spec.Tolerations[1], instance.Spec.Topology.Subsets[0].Tolerations[1])).Should(gomega.BeTrue())

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	instance.Spec.Template.StatefulSetTemplate.Spec.Template.Spec.Tolerations = append(instance.Spec.Template.StatefulSetTemplate.Spec.Template.Spec.Tolerations, corev1.Toleration{
		Key:      "taint-0",
		Operator: corev1.TolerationOpExists,
	})

	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	stsList = expectedStsCount(g, instance, 1)
	sts = &stsList.Items[0]
	g.Expect(sts.Spec.Template.Spec.Tolerations).ShouldNot(gomega.BeNil())
	g.Expect(len(sts.Spec.Template.Spec.Tolerations)).Should(gomega.BeEquivalentTo(3))
	g.Expect(reflect.DeepEqual(sts.Spec.Template.Spec.Tolerations[1], instance.Spec.Topology.Subsets[0].Tolerations[0])).Should(gomega.BeTrue())
	g.Expect(reflect.DeepEqual(sts.Spec.Template.Spec.Tolerations[2], instance.Spec.Topology.Subsets[0].Tolerations[1])).Should(gomega.BeTrue())
}

func TestStsDupSubset(t *testing.T) {
	g, requests, cancel, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		cancel()
		mgrStopped.Wait()
	}()

	caseName := "test-sts-dup-subset"
	instance := &appsv1alpha1.UnitedDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      caseName,
			Namespace: "default",
		},
		Spec: appsv1alpha1.UnitedDeploymentSpec{
			Replicas: &one,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": caseName,
				},
			},
			Template: appsv1alpha1.SubsetTemplate{
				StatefulSetTemplate: &appsv1alpha1.StatefulSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1.StatefulSetSpec{
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"name": caseName,
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "container-a",
										Image: "nginx:1.0",
									},
								},
							},
						},
					},
				},
			},
			Topology: appsv1alpha1.Topology{
				Subsets: []appsv1alpha1.Subset{
					{
						Name: "subset-a",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"node-a"},
								},
							},
						},
					},
				},
			},
			RevisionHistoryLimit: &ten,
		},
	}

	// Create the UnitedDeployment object and expect the Reconcile and Deployment to be created
	err := c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	waitReconcilerProcessFinished(g, requests, 3)

	stsList := expectedStsCount(g, instance, 1)

	subsetA := stsList.Items[0]
	dupSts := subsetA.DeepCopy()
	dupSts.Name = "dup-subset-a"
	dupSts.ResourceVersion = ""
	g.Expect(c.Create(context.TODO(), dupSts)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 3)
	expectedStsCount(g, instance, 1)
}

func TestStsScale(t *testing.T) {
	g, requests, cancel, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		cancel()
		mgrStopped.Wait()
	}()

	caseName := "test-sts-scale"
	instance := &appsv1alpha1.UnitedDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      caseName,
			Namespace: "default",
		},
		Spec: appsv1alpha1.UnitedDeploymentSpec{
			Replicas: &one,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": caseName,
				},
			},
			Template: appsv1alpha1.SubsetTemplate{
				StatefulSetTemplate: &appsv1alpha1.StatefulSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1.StatefulSetSpec{
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"name": caseName,
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "container-a",
										Image: "nginx:1.0",
									},
								},
							},
						},
					},
				},
			},
			Topology: appsv1alpha1.Topology{
				Subsets: []appsv1alpha1.Subset{
					{
						Name: "subset-a",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"nodeA"},
								},
							},
						},
					},
					{
						Name: "subset-b",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"nodeB"},
								},
							},
						},
					},
				},
			},
			RevisionHistoryLimit: &ten,
		},
	}

	// Create the UnitedDeployment object and expect the Reconcile and Deployment to be created
	err := c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	waitReconcilerProcessFinished(g, requests, 3)

	stsList := expectedStsCount(g, instance, 2)
	g.Expect(*stsList.Items[0].Spec.Replicas + *stsList.Items[1].Spec.Replicas).Should(gomega.BeEquivalentTo(1))

	var two int32 = 2
	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	instance.Spec.Replicas = &two
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	stsList = expectedStsCount(g, instance, 2)
	g.Expect(*stsList.Items[0].Spec.Replicas).Should(gomega.BeEquivalentTo(1))
	g.Expect(*stsList.Items[1].Spec.Replicas).Should(gomega.BeEquivalentTo(1))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	g.Expect(instance.Status.SubsetReplicas).Should(gomega.BeEquivalentTo(map[string]int32{
		"subset-a": 1,
		"subset-b": 1,
	}))

	var five int32 = 6
	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	instance.Spec.Replicas = &five
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	stsList = expectedStsCount(g, instance, 2)
	g.Expect(*stsList.Items[0].Spec.Replicas).Should(gomega.BeEquivalentTo(3))
	g.Expect(*stsList.Items[1].Spec.Replicas).Should(gomega.BeEquivalentTo(3))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	g.Expect(instance.Status.SubsetReplicas).Should(gomega.BeEquivalentTo(map[string]int32{
		"subset-a": 3,
		"subset-b": 3,
	}))

	var four int32 = 4
	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	instance.Spec.Replicas = &four
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	stsList = expectedStsCount(g, instance, 2)
	g.Expect(*stsList.Items[0].Spec.Replicas).Should(gomega.BeEquivalentTo(2))
	g.Expect(*stsList.Items[1].Spec.Replicas).Should(gomega.BeEquivalentTo(2))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	g.Expect(instance.Status.SubsetReplicas).Should(gomega.BeEquivalentTo(map[string]int32{
		"subset-a": 2,
		"subset-b": 2,
	}))
}

func TestStsUpdate(t *testing.T) {
	g, requests, cancel, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		cancel()
		mgrStopped.Wait()
	}()

	caseName := "test-sts-update"
	instance := &appsv1alpha1.UnitedDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      caseName,
			Namespace: "default",
		},
		Spec: appsv1alpha1.UnitedDeploymentSpec{
			Replicas: &two,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": caseName,
				},
			},
			Template: appsv1alpha1.SubsetTemplate{
				StatefulSetTemplate: &appsv1alpha1.StatefulSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1.StatefulSetSpec{
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"name": caseName,
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "container-a",
										Image: "nginx:1.0",
									},
								},
							},
						},
					},
				},
			},
			Topology: appsv1alpha1.Topology{
				Subsets: []appsv1alpha1.Subset{
					{
						Name: "subset-a",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"nodeA"},
								},
							},
						},
					},
					{
						Name: "subset-b",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"nodeB"},
								},
							},
						},
					},
				},
			},
			RevisionHistoryLimit: &ten,
		},
	}

	// Create the UnitedDeployment object and expect the Reconcile and Deployment to be created
	err := c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	waitReconcilerProcessFinished(g, requests, 3)

	stsList := expectedStsCount(g, instance, 2)
	g.Expect(*stsList.Items[0].Spec.Replicas + *stsList.Items[1].Spec.Replicas).Should(gomega.BeEquivalentTo(2))
	revisionList := &appsv1.ControllerRevisionList{}
	g.Expect(c.List(context.TODO(), revisionList))
	g.Expect(len(revisionList.Items)).Should(gomega.BeEquivalentTo(1))
	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	v1 := revisionList.Items[0].Name
	g.Expect(instance.Status.CurrentRevision).Should(gomega.BeEquivalentTo(v1))

	instance.Spec.Template.StatefulSetTemplate.Spec.Template.Spec.Containers[0].Image = "nginx:2.0"
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	stsList = expectedStsCount(g, instance, 2)
	g.Expect(stsList.Items[0].Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))
	g.Expect(stsList.Items[1].Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	revisionList = &appsv1.ControllerRevisionList{}
	g.Expect(c.List(context.TODO(), revisionList))
	g.Expect(len(revisionList.Items)).Should(gomega.BeEquivalentTo(2))
	v2 := revisionList.Items[0].Name
	if v2 == v1 {
		v2 = revisionList.Items[1].Name
	}
	g.Expect(instance.Status.UpdateStatus.UpdatedRevision).Should(gomega.BeEquivalentTo(v2))
}

func TestStsRollingUpdatePartition(t *testing.T) {
	g, requests, cancel, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		cancel()
		mgrStopped.Wait()
	}()

	caseName := "test-sts-rolling-update-partition"
	instance := &appsv1alpha1.UnitedDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      caseName,
			Namespace: "default",
		},
		Spec: appsv1alpha1.UnitedDeploymentSpec{
			Replicas: &ten,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": caseName,
				},
			},
			Template: appsv1alpha1.SubsetTemplate{
				StatefulSetTemplate: &appsv1alpha1.StatefulSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1.StatefulSetSpec{
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"name": caseName,
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "container-a",
										Image: "nginx:1.0",
									},
								},
							},
						},
					},
				},
			},
			UpdateStrategy: appsv1alpha1.UnitedDeploymentUpdateStrategy{
				Type: appsv1alpha1.ManualUpdateStrategyType,
			},
			Topology: appsv1alpha1.Topology{
				Subsets: []appsv1alpha1.Subset{
					{
						Name: "subset-a",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"nodeA"},
								},
							},
						},
					},
					{
						Name: "subset-b",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"nodeB"},
								},
							},
						},
					},
				},
			},
			RevisionHistoryLimit: &ten,
		},
	}

	// Create the UnitedDeployment object and expect the Reconcile and Deployment to be created
	err := c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	waitReconcilerProcessFinished(g, requests, 3)

	stsList := expectedStsCount(g, instance, 2)
	g.Expect(*stsList.Items[0].Spec.Replicas).Should(gomega.BeEquivalentTo(5))
	g.Expect(*stsList.Items[1].Spec.Replicas).Should(gomega.BeEquivalentTo(5))

	// update with partition
	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	instance.Spec.UpdateStrategy.ManualUpdate = &appsv1alpha1.ManualUpdate{
		Partitions: map[string]int32{
			"subset-a": 4,
			"subset-b": 3,
		},
	}
	instance.Spec.Template.StatefulSetTemplate.Spec.Template.Spec.Containers[0].Image = "nginx:2.0"
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	stsList = expectedStsCount(g, instance, 2)
	g.Expect(stsList.Items[0].Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))
	g.Expect(stsList.Items[1].Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))

	stsA := getSubsetByName(stsList, "subset-a")
	g.Expect(stsA).ShouldNot(gomega.BeNil())
	g.Expect(*stsA.Spec.UpdateStrategy.RollingUpdate.Partition).Should(gomega.BeEquivalentTo(4))

	stsB := getSubsetByName(stsList, "subset-b")
	g.Expect(stsB).ShouldNot(gomega.BeNil())
	g.Expect(*stsB.Spec.UpdateStrategy.RollingUpdate.Partition).Should(gomega.BeEquivalentTo(3))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	g.Expect(instance.Status.UpdateStatus.CurrentPartitions).Should(gomega.BeEquivalentTo(map[string]int32{
		"subset-a": 4,
		"subset-b": 3,
	}))

	// move on
	instance.Spec.UpdateStrategy.ManualUpdate = &appsv1alpha1.ManualUpdate{
		Partitions: map[string]int32{
			"subset-a": 0,
			"subset-b": 3,
		},
	}
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 4)

	stsList = expectedStsCount(g, instance, 2)
	g.Expect(stsList.Items[0].Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))
	g.Expect(stsList.Items[1].Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))

	stsA = getSubsetByName(stsList, "subset-a")
	g.Expect(stsA).ShouldNot(gomega.BeNil())
	g.Expect(*stsA.Spec.UpdateStrategy.RollingUpdate.Partition).Should(gomega.BeEquivalentTo(0))

	stsB = getSubsetByName(stsList, "subset-b")
	g.Expect(stsB).ShouldNot(gomega.BeNil())
	g.Expect(*stsB.Spec.UpdateStrategy.RollingUpdate.Partition).Should(gomega.BeEquivalentTo(3))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	g.Expect(instance.Status.UpdateStatus.CurrentPartitions).Should(gomega.BeEquivalentTo(map[string]int32{
		"subset-a": 0,
		"subset-b": 3,
	}))

	// move on
	instance.Spec.UpdateStrategy.ManualUpdate = &appsv1alpha1.ManualUpdate{
		Partitions: map[string]int32{},
	}
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	stsList = expectedStsCount(g, instance, 2)
	g.Expect(stsList.Items[0].Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))
	g.Expect(stsList.Items[1].Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))

	stsA = getSubsetByName(stsList, "subset-a")
	g.Expect(stsA).ShouldNot(gomega.BeNil())
	g.Expect(*stsA.Spec.UpdateStrategy.RollingUpdate.Partition).Should(gomega.BeEquivalentTo(0))

	stsB = getSubsetByName(stsList, "subset-b")
	g.Expect(stsB).ShouldNot(gomega.BeNil())
	g.Expect(*stsB.Spec.UpdateStrategy.RollingUpdate.Partition).Should(gomega.BeEquivalentTo(0))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	g.Expect(instance.Status.UpdateStatus.CurrentPartitions).Should(gomega.BeEquivalentTo(map[string]int32{
		"subset-a": 0,
		"subset-b": 0,
	}))
}

func TestStsRollingUpdateDeleteStuckPod(t *testing.T) {
	g, requests, cancel, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		cancel()
		mgrStopped.Wait()
	}()

	caseName := "test-sts-rolling-update-delete-stuck-pod"
	instance := &appsv1alpha1.UnitedDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      caseName,
			Namespace: "default",
		},
		Spec: appsv1alpha1.UnitedDeploymentSpec{
			Replicas: &ten,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": caseName,
				},
			},
			Template: appsv1alpha1.SubsetTemplate{
				StatefulSetTemplate: &appsv1alpha1.StatefulSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1.StatefulSetSpec{
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"name": caseName,
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "container-a",
										Image: "nginx:1.0",
									},
								},
							},
						},
					},
				},
			},
			UpdateStrategy: appsv1alpha1.UnitedDeploymentUpdateStrategy{
				Type: appsv1alpha1.ManualUpdateStrategyType,
			},
			Topology: appsv1alpha1.Topology{
				Subsets: []appsv1alpha1.Subset{
					{
						Name: "subset-a",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"nodeA"},
								},
							},
						},
					},
					{
						Name: "subset-b",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"nodeB"},
								},
							},
						},
					},
				},
			},
			RevisionHistoryLimit: &ten,
		},
	}

	// Create the UnitedDeployment object and expect the Reconcile and Deployment to be created
	err := c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	waitReconcilerProcessFinished(g, requests, 3)

	stsList := expectedStsCount(g, instance, 2)
	g.Expect(*stsList.Items[0].Spec.Replicas).Should(gomega.BeEquivalentTo(5))
	g.Expect(*stsList.Items[1].Spec.Replicas).Should(gomega.BeEquivalentTo(5))

	g.Expect(provisionStatefulSetMockPod(c, &stsList.Items[0])).Should(gomega.BeNil())
	g.Expect(provisionStatefulSetMockPod(c, &stsList.Items[1])).Should(gomega.BeNil())

	g.Expect(retry(func() error { return collectPodOrdinal(c, &stsList.Items[0], "0,1,2,3,4") })).Should(gomega.BeNil())
	g.Expect(retry(func() error { return collectPodOrdinal(c, &stsList.Items[1], "0,1,2,3,4") })).Should(gomega.BeNil())

	// update with partition
	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	instance.Spec.UpdateStrategy.ManualUpdate = &appsv1alpha1.ManualUpdate{
		Partitions: map[string]int32{
			"subset-a": 4,
			"subset-b": 3,
		},
	}
	instance.Spec.Template.StatefulSetTemplate.Spec.Template.Spec.Containers[0].Image = "nginx:2.0"
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	stsList = expectedStsCount(g, instance, 2)
	g.Expect(stsList.Items[0].Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))
	g.Expect(stsList.Items[1].Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))

	stsA := getSubsetByName(stsList, "subset-a")
	g.Expect(stsA).ShouldNot(gomega.BeNil())
	g.Expect(*stsA.Spec.UpdateStrategy.RollingUpdate.Partition).Should(gomega.BeEquivalentTo(4))
	g.Expect(collectPodOrdinal(c, stsA, "0,1,2,3")).Should(gomega.BeNil())

	stsB := getSubsetByName(stsList, "subset-b")
	g.Expect(stsB).ShouldNot(gomega.BeNil())
	g.Expect(*stsB.Spec.UpdateStrategy.RollingUpdate.Partition).Should(gomega.BeEquivalentTo(3))
	g.Expect(collectPodOrdinal(c, stsB, "0,1,2")).Should(gomega.BeNil())
}

func TestStsOnDelete(t *testing.T) {
	g, requests, cancel, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		cancel()
		mgrStopped.Wait()
	}()

	caseName := "test-sts-on-delete"
	instance := &appsv1alpha1.UnitedDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      caseName,
			Namespace: "default",
		},
		Spec: appsv1alpha1.UnitedDeploymentSpec{
			Replicas: &ten,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": caseName,
				},
			},
			Template: appsv1alpha1.SubsetTemplate{
				StatefulSetTemplate: &appsv1alpha1.StatefulSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1.StatefulSetSpec{
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"name": caseName,
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "container-a",
										Image: "nginx:1.0",
									},
								},
							},
						},
						UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
							Type: appsv1.OnDeleteStatefulSetStrategyType,
						},
					},
				},
			},
			UpdateStrategy: appsv1alpha1.UnitedDeploymentUpdateStrategy{
				Type: appsv1alpha1.ManualUpdateStrategyType,
			},
			Topology: appsv1alpha1.Topology{
				Subsets: []appsv1alpha1.Subset{
					{
						Name: "subset-a",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"nodeA"},
								},
							},
						},
					},
					{
						Name: "subset-b",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"nodeB"},
								},
							},
						},
					},
				},
			},
			RevisionHistoryLimit: &ten,
		},
	}

	// Create the UnitedDeployment object and expect the Reconcile and Deployment to be created
	err := c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	waitReconcilerProcessFinished(g, requests, 3)

	stsList := expectedStsCount(g, instance, 2)
	g.Expect(*stsList.Items[0].Spec.Replicas).Should(gomega.BeEquivalentTo(5))
	g.Expect(*stsList.Items[1].Spec.Replicas).Should(gomega.BeEquivalentTo(5))

	g.Expect(stsList.Items[0].Spec.UpdateStrategy.Type).Should(gomega.BeEquivalentTo(appsv1.OnDeleteStatefulSetStrategyType))
	g.Expect(stsList.Items[1].Spec.UpdateStrategy.Type).Should(gomega.BeEquivalentTo(appsv1.OnDeleteStatefulSetStrategyType))

	// update with partition
	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	instance.Spec.UpdateStrategy.ManualUpdate = &appsv1alpha1.ManualUpdate{
		Partitions: map[string]int32{
			"subset-a": 4,
			"subset-b": 3,
		},
	}
	instance.Spec.Template.StatefulSetTemplate.Spec.Template.Spec.Containers[0].Image = "nginx:2.0"
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	stsList = expectedStsCount(g, instance, 2)
	g.Expect(stsList.Items[0].Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))
	g.Expect(stsList.Items[1].Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))

	stsA := getSubsetByName(stsList, "subset-a")
	g.Expect(stsA).ShouldNot(gomega.BeNil())
	g.Expect(stsA.Spec.UpdateStrategy.Type).Should(gomega.BeEquivalentTo(appsv1.OnDeleteStatefulSetStrategyType))
	g.Expect(stsA.Spec.UpdateStrategy.RollingUpdate).Should(gomega.BeNil())

	stsB := getSubsetByName(stsList, "subset-b")
	g.Expect(stsB).ShouldNot(gomega.BeNil())
	g.Expect(stsB.Spec.UpdateStrategy.Type).Should(gomega.BeEquivalentTo(appsv1.OnDeleteStatefulSetStrategyType))
	g.Expect(stsB.Spec.UpdateStrategy.RollingUpdate).Should(gomega.BeNil())

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	g.Expect(instance.Status.UpdateStatus.CurrentPartitions).Should(gomega.BeEquivalentTo(map[string]int32{
		"subset-a": 4,
		"subset-b": 3,
	}))

	// move on
	instance.Spec.UpdateStrategy.ManualUpdate = &appsv1alpha1.ManualUpdate{
		Partitions: map[string]int32{
			"subset-a": 0,
			"subset-b": 3,
		},
	}
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	stsList = expectedStsCount(g, instance, 2)
	g.Expect(stsList.Items[0].Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))
	g.Expect(stsList.Items[1].Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))
}

func TestStsSubsetCount(t *testing.T) {
	g, requests, cancel, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		cancel()
		mgrStopped.Wait()
	}()

	caseName := "test-sts-subset-count"
	instance := &appsv1alpha1.UnitedDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      caseName,
			Namespace: "default",
		},
		Spec: appsv1alpha1.UnitedDeploymentSpec{
			Replicas: &ten,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": caseName,
				},
			},
			Template: appsv1alpha1.SubsetTemplate{
				StatefulSetTemplate: &appsv1alpha1.StatefulSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1.StatefulSetSpec{
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"name": caseName,
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "container-a",
										Image: "nginx:1.0",
									},
								},
							},
						},
					},
				},
			},
			Topology: appsv1alpha1.Topology{
				Subsets: []appsv1alpha1.Subset{
					{
						Name: "subset-a",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"nodeA"},
								},
							},
						},
					},
					{
						Name: "subset-b",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"nodeB"},
								},
							},
						},
					},
				},
			},
			RevisionHistoryLimit: &ten,
		},
	}

	// Create the UnitedDeployment object and expect the Reconcile and Deployment to be created
	err := c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	waitReconcilerProcessFinished(g, requests, 3)

	stsList := expectedStsCount(g, instance, 2)
	g.Expect(*stsList.Items[0].Spec.Replicas).Should(gomega.BeEquivalentTo(5))
	g.Expect(*stsList.Items[1].Spec.Replicas).Should(gomega.BeEquivalentTo(5))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	nine := intstr.FromInt(9)
	instance.Spec.Topology.Subsets[0].Replicas = &nine
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	stsList = expectedStsCount(g, instance, 2)
	setsubA := getSubsetByName(stsList, "subset-a")
	g.Expect(*setsubA.Spec.Replicas).Should(gomega.BeEquivalentTo(9))
	setsubB := getSubsetByName(stsList, "subset-b")
	g.Expect(*setsubB.Spec.Replicas).Should(gomega.BeEquivalentTo(1))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	percentage := intstr.FromString("40%")
	instance.Spec.Topology.Subsets[0].Replicas = &percentage
	instance.Spec.Template.StatefulSetTemplate.Spec.Template.Spec.Containers[0].Image = "nginx:2.0"
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	stsList = expectedStsCount(g, instance, 2)
	setsubA = getSubsetByName(stsList, "subset-a")
	g.Expect(*setsubA.Spec.Replicas).Should(gomega.BeEquivalentTo(4))
	g.Expect(setsubA.Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))
	setsubB = getSubsetByName(stsList, "subset-b")
	g.Expect(*setsubB.Spec.Replicas).Should(gomega.BeEquivalentTo(6))
	g.Expect(setsubB.Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	percentage = intstr.FromString("30%")
	instance.Spec.Topology.Subsets[0].Replicas = &percentage
	instance.Spec.Template.StatefulSetTemplate.Spec.Template.Spec.Containers[0].Image = "nginx:3.0"
	instance.Spec.UpdateStrategy.Type = appsv1alpha1.ManualUpdateStrategyType
	instance.Spec.UpdateStrategy.ManualUpdate = &appsv1alpha1.ManualUpdate{
		Partitions: map[string]int32{
			"subset-a": 1,
		},
	}
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	stsList = expectedStsCount(g, instance, 2)
	setsubA = getSubsetByName(stsList, "subset-a")
	g.Expect(*setsubA.Spec.Replicas).Should(gomega.BeEquivalentTo(3))
	g.Expect(setsubA.Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:3.0"))
	g.Expect(setsubA.Spec.UpdateStrategy.RollingUpdate).ShouldNot(gomega.BeNil())
	g.Expect(setsubA.Spec.UpdateStrategy.RollingUpdate.Partition).ShouldNot(gomega.BeNil())
	g.Expect(*setsubA.Spec.UpdateStrategy.RollingUpdate.Partition).Should(gomega.BeEquivalentTo(1))
	setsubB = getSubsetByName(stsList, "subset-b")
	g.Expect(*setsubB.Spec.Replicas).Should(gomega.BeEquivalentTo(7))
	g.Expect(setsubB.Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:3.0"))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	percentage = intstr.FromString("20%")
	instance.Spec.Topology.Subsets[0].Replicas = &percentage
	instance.Spec.Template.StatefulSetTemplate.Spec.Template.Spec.Containers[0].Image = "nginx:4.0"
	instance.Spec.UpdateStrategy.ManualUpdate.Partitions = map[string]int32{
		"subset-a": 2,
	}
	instance.Spec.Topology.Subsets = append(instance.Spec.Topology.Subsets, appsv1alpha1.Subset{
		Name: "subset-c",
		NodeSelectorTerm: corev1.NodeSelectorTerm{
			MatchExpressions: []corev1.NodeSelectorRequirement{
				{
					Key:      "node-name",
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{"nodeC"},
				},
			},
		},
	})
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	stsList = expectedStsCount(g, instance, 3)
	setsubA = getSubsetByName(stsList, "subset-a")
	g.Expect(*setsubA.Spec.Replicas).Should(gomega.BeEquivalentTo(2))
	g.Expect(setsubA.Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:4.0"))
	g.Expect(setsubA.Spec.UpdateStrategy.RollingUpdate).ShouldNot(gomega.BeNil())
	g.Expect(setsubA.Spec.UpdateStrategy.RollingUpdate.Partition).ShouldNot(gomega.BeNil())
	g.Expect(*setsubA.Spec.UpdateStrategy.RollingUpdate.Partition).Should(gomega.BeEquivalentTo(2))
	setsubB = getSubsetByName(stsList, "subset-b")
	g.Expect(*setsubB.Spec.Replicas).Should(gomega.BeEquivalentTo(4))
	g.Expect(setsubB.Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:4.0"))
	setsubB = getSubsetByName(stsList, "subset-c")
	g.Expect(*setsubB.Spec.Replicas).Should(gomega.BeEquivalentTo(4))
	g.Expect(setsubB.Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:4.0"))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	percentage = intstr.FromString("10%")
	instance.Spec.Topology.Subsets[0].Replicas = &percentage
	instance.Spec.Template.StatefulSetTemplate.Spec.Template.Spec.Containers[0].Image = "nginx:5.0"
	instance.Spec.UpdateStrategy.ManualUpdate.Partitions = map[string]int32{
		"subset-a": 2,
	}
	instance.Spec.Topology.Subsets = instance.Spec.Topology.Subsets[:2]
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 3)

	stsList = expectedStsCount(g, instance, 2)
	setsubA = getSubsetByName(stsList, "subset-a")
	g.Expect(*setsubA.Spec.Replicas).Should(gomega.BeEquivalentTo(1))
	g.Expect(setsubA.Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:5.0"))
	g.Expect(setsubA.Spec.UpdateStrategy.RollingUpdate).ShouldNot(gomega.BeNil())
	g.Expect(setsubA.Spec.UpdateStrategy.RollingUpdate.Partition).ShouldNot(gomega.BeNil())
	g.Expect(*setsubA.Spec.UpdateStrategy.RollingUpdate.Partition).Should(gomega.BeEquivalentTo(1))
	setsubB = getSubsetByName(stsList, "subset-b")
	g.Expect(*setsubB.Spec.Replicas).Should(gomega.BeEquivalentTo(9))
	g.Expect(setsubB.Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:5.0"))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	g.Expect(instance.Spec.UpdateStrategy.ManualUpdate.Partitions["subset-a"]).Should(gomega.BeEquivalentTo(2))
	percentage = intstr.FromString("40%")
	instance.Spec.Topology.Subsets[0].Replicas = &percentage
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 3)

	stsList = expectedStsCount(g, instance, 2)
	setsubA = getSubsetByName(stsList, "subset-a")
	g.Expect(*setsubA.Spec.Replicas).Should(gomega.BeEquivalentTo(4))
	g.Expect(setsubA.Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:5.0"))
	g.Expect(setsubA.Spec.UpdateStrategy.RollingUpdate).ShouldNot(gomega.BeNil())
	g.Expect(setsubA.Spec.UpdateStrategy.RollingUpdate.Partition).ShouldNot(gomega.BeNil())
	g.Expect(*setsubA.Spec.UpdateStrategy.RollingUpdate.Partition).Should(gomega.BeEquivalentTo(2))
	setsubB = getSubsetByName(stsList, "subset-b")
	g.Expect(*setsubB.Spec.Replicas).Should(gomega.BeEquivalentTo(6))
	g.Expect(setsubB.Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:5.0"))
}

func retry(assert func() error) error {
	var err error
	for i := 0; i < 3; i++ {
		err = assert()
		if err == nil {
			return nil
		}
		time.Sleep(1 * time.Second)
	}

	return err
}

func collectPodOrdinal(c client.Client, sts *appsv1.StatefulSet, expected string) error {
	selector, err := metav1.LabelSelectorAsSelector(sts.Spec.Selector)
	if err != nil {
		return err
	}

	podList := &corev1.PodList{}
	if err := c.List(context.TODO(), podList, &client.ListOptions{LabelSelector: selector}); err != nil {
		return err
	}

	marks := make([]bool, len(podList.Items))
	for _, pod := range podList.Items {
		ordinal := int(kruisectlutil.GetOrdinal(&pod))
		if ordinal >= len(marks) || ordinal < 0 {
			continue
		}

		marks[ordinal] = true
	}

	got := ""
	for idx, mark := range marks {
		if mark {
			got = fmt.Sprintf("%s,%d", got, idx)
		}
	}

	if len(got) > 0 {
		got = got[1:]
	}

	if got != expected {
		return fmt.Errorf("expected %s, got %s", expected, got)
	}

	return nil
}

func provisionStatefulSetMockPod(c client.Client, sts *appsv1.StatefulSet) error {
	if sts.Spec.Replicas == nil {
		return nil
	}

	replicas := *sts.Spec.Replicas
	for {
		replicas--
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: sts.Namespace,
				Name:      fmt.Sprintf("%s-%d", sts.Name, replicas),
				Labels:    sts.Spec.Template.Labels,
			},
			Spec: sts.Spec.Template.Spec,
		}

		if err := c.Create(context.TODO(), pod); err != nil {
			if !apierrors.IsAlreadyExists(err) {
				return err
			}
		}

		if replicas == 0 {
			return nil
		}
	}
}

func waitReconcilerProcessFinished(g *gomega.GomegaWithT, requests chan reconcile.Request, minCount int) {
	timeoutChan := time.After(timeout)
	maxTimeoutChan := time.After(timeout * 2)
	for {
		minCount--
		select {
		case <-requests:
			continue
		case <-timeoutChan:
			if minCount <= 0 {
				return
			}
		case <-maxTimeoutChan:
			return
		}
	}
}

func getSubsetByName(stsList *appsv1.StatefulSetList, name string) *appsv1.StatefulSet {
	for _, sts := range stsList.Items {
		if sts.Labels[appsv1alpha1.SubSetNameLabelKey] == name {
			return &sts
		}
	}

	return nil
}

func expectedStsCount(g *gomega.GomegaWithT, ud *appsv1alpha1.UnitedDeployment, count int) *appsv1.StatefulSetList {
	stsList := &appsv1.StatefulSetList{}

	selector, err := metav1.LabelSelectorAsSelector(ud.Spec.Selector)
	g.Expect(err).Should(gomega.BeNil())

	g.Eventually(func() error {
		if err := c.List(context.TODO(), stsList, &client.ListOptions{LabelSelector: selector}); err != nil {
			return err
		}

		if len(stsList.Items) != count {
			return fmt.Errorf("expected %d sts, got %d", count, len(stsList.Items))
		}

		return nil
	}, timeout).Should(gomega.Succeed())

	return stsList
}

func setUp(t *testing.T) (*gomega.GomegaWithT, chan reconcile.Request, context.CancelFunc, *sync.WaitGroup) {
	g := gomega.NewGomegaWithT(t)
	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{Metrics: metricsserver.Options{BindAddress: "0"}})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	c = utilclient.NewClientFromManager(mgr, "test-uniteddeployment-controller")
	recFn, requests := SetupTestReconcile(newReconciler(mgr))
	g.Expect(add(mgr, recFn)).NotTo(gomega.HaveOccurred())
	ctx, cancel := context.WithCancel(context.Background())
	mgrStopped := StartTestManager(ctx, mgr, g)

	return g, requests, cancel, mgrStopped
}

// clean can be shared amongst all subset workload tests (i.e. statefulsets, deployments, advancedStatefulsets, etc.).
func clean(g *gomega.GomegaWithT, c client.Client) {
	udList := &appsv1alpha1.UnitedDeploymentList{}
	if err := c.List(context.TODO(), udList); err == nil {
		for _, ud := range udList.Items {
			c.Delete(context.TODO(), &ud)
		}
	}
	g.Eventually(func() error {
		if err := c.List(context.TODO(), udList); err != nil {
			return err
		}

		if len(udList.Items) != 0 {
			return fmt.Errorf("expected %d subset objects, got %d", 0, len(udList.Items))
		}

		return nil
	}, timeout, time.Second).Should(gomega.Succeed())

	rList := &appsv1.ControllerRevisionList{}
	if err := c.List(context.TODO(), rList); err == nil {
		for _, ud := range rList.Items {
			c.Delete(context.TODO(), &ud)
		}
	}
	g.Eventually(func() error {
		if err := c.List(context.TODO(), rList); err != nil {
			return err
		}

		if len(rList.Items) != 0 {
			return fmt.Errorf("expected %d subset objects, got %d", 0, len(rList.Items))
		}

		return nil
	}, timeout, time.Second).Should(gomega.Succeed())

	stsList := &appsv1.StatefulSetList{}
	if err := c.List(context.TODO(), stsList); err == nil {
		for _, sts := range stsList.Items {
			c.Delete(context.TODO(), &sts)
		}
	}
	g.Eventually(func() error {
		if err := c.List(context.TODO(), stsList); err != nil {
			return err
		}

		if len(stsList.Items) != 0 {
			return fmt.Errorf("expected %d sts, got %d", 0, len(stsList.Items))
		}

		return nil
	}, timeout, time.Second).Should(gomega.Succeed())

	astsList := &appsv1beta1.StatefulSetList{}
	if err := c.List(context.TODO(), astsList); err == nil {
		for _, asts := range astsList.Items {
			c.Delete(context.TODO(), &asts)
		}
	}
	g.Eventually(func() error {
		if err := c.List(context.TODO(), astsList); err != nil {
			return err
		}

		if len(astsList.Items) != 0 {
			return fmt.Errorf("expected %d asts, got %d", 0, len(astsList.Items))
		}

		return nil
	}, timeout, time.Second).Should(gomega.Succeed())

	deploymentList := &appsv1.DeploymentList{}
	if err := c.List(context.TODO(), deploymentList); err == nil {
		for _, asts := range deploymentList.Items {
			c.Delete(context.TODO(), &asts)
		}
	}
	g.Eventually(func() error {
		if err := c.List(context.TODO(), deploymentList); err != nil {
			return err
		}

		if len(deploymentList.Items) != 0 {
			return fmt.Errorf("expected %d deployments, got %d", 0, len(deploymentList.Items))
		}

		return nil
	}, timeout, time.Second).Should(gomega.Succeed())

	podList := &corev1.PodList{}
	if err := c.List(context.TODO(), podList); err == nil {
		for _, pod := range podList.Items {
			c.Delete(context.TODO(), &pod)
		}
	}
	g.Eventually(func() error {
		if err := c.List(context.TODO(), podList); err != nil {
			return err
		}

		if len(podList.Items) != 0 {
			return fmt.Errorf("expected %d pods, got %d", 0, len(podList.Items))
		}

		return nil
	}, timeout, time.Second).Should(gomega.Succeed())
}

type TestCaseFunc func(t *testing.T, g *gomega.GomegaWithT, namespace string, requests chan reconcile.Request)

/*
Copyright 2020 The Kruise Authors.

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
	"testing"

	"github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

func TestDeploymentReconcile(t *testing.T) {
	g, requests, cancel, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		cancel()
		mgrStopped.Wait()
	}()

	caseName := "deployment-reconcile"
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
				DeploymentTemplate: &appsv1alpha1.DeploymentTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1.DeploymentSpec{
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
	expectedDeploymentCount(g, instance, 1)
}

func TestDeploymentSubsetProvision(t *testing.T) {
	g, requests, cancel, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		cancel()
		mgrStopped.Wait()
	}()

	caseName := "test-deployment-subset-provision"
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
				DeploymentTemplate: &appsv1alpha1.DeploymentTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1.DeploymentSpec{
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

	deploymentList := expectedDeploymentCount(g, instance, 1)
	deployment := &deploymentList.Items[0]
	g.Expect(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution).ShouldNot(gomega.BeNil())
	g.Expect(len(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms)).Should(gomega.BeEquivalentTo(1))
	g.Expect(len(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions)).Should(gomega.BeEquivalentTo(1))
	g.Expect(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Key).Should(gomega.BeEquivalentTo("node-name"))
	g.Expect(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpIn))
	g.Expect(len(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values)).Should(gomega.BeEquivalentTo(1))
	g.Expect(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values[0]).Should(gomega.BeEquivalentTo("node-a"))

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

	deploymentList = expectedDeploymentCount(g, instance, 2)
	deployment = getDeploymentSubsetByName(deploymentList, "subset-a")
	g.Expect(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution).ShouldNot(gomega.BeNil())
	g.Expect(len(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms)).Should(gomega.BeEquivalentTo(1))
	g.Expect(len(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions)).Should(gomega.BeEquivalentTo(1))
	g.Expect(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Key).Should(gomega.BeEquivalentTo("node-name"))
	g.Expect(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpIn))
	g.Expect(len(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values)).Should(gomega.BeEquivalentTo(1))
	g.Expect(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values[0]).Should(gomega.BeEquivalentTo("node-a"))

	deployment = getDeploymentSubsetByName(deploymentList, "subset-b")
	g.Expect(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution).ShouldNot(gomega.BeNil())
	g.Expect(len(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms)).Should(gomega.BeEquivalentTo(1))
	g.Expect(len(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions)).Should(gomega.BeEquivalentTo(1))
	g.Expect(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Key).Should(gomega.BeEquivalentTo("node-name"))
	g.Expect(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpIn))
	g.Expect(len(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values)).Should(gomega.BeEquivalentTo(1))
	g.Expect(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values[0]).Should(gomega.BeEquivalentTo("node-b"))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	instance.Spec.Topology.Subsets = instance.Spec.Topology.Subsets[1:]
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	deploymentList = expectedDeploymentCount(g, instance, 1)
	deployment = &deploymentList.Items[0]
	g.Expect(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution).ShouldNot(gomega.BeNil())
	g.Expect(len(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms)).Should(gomega.BeEquivalentTo(1))
	g.Expect(len(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions)).Should(gomega.BeEquivalentTo(1))
	g.Expect(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Key).Should(gomega.BeEquivalentTo("node-name"))
	g.Expect(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpIn))
	g.Expect(len(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values)).Should(gomega.BeEquivalentTo(1))
	g.Expect(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values[0]).Should(gomega.BeEquivalentTo("node-b"))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	instance.Spec.Template.DeploymentTemplate.Spec.Template.Spec.Affinity = &corev1.Affinity{}
	instance.Spec.Template.DeploymentTemplate.Spec.Template.Spec.Affinity.NodeAffinity = &corev1.NodeAffinity{}

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
	instance.Spec.Template.DeploymentTemplate.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = nodeSelector

	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	deploymentList = expectedDeploymentCount(g, instance, 1)
	deployment = &deploymentList.Items[0]
	g.Expect(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution).ShouldNot(gomega.BeNil())
	g.Expect(len(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms)).Should(gomega.BeEquivalentTo(2))

	g.Expect(len(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions)).Should(gomega.BeEquivalentTo(3))
	g.Expect(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Key).Should(gomega.BeEquivalentTo("test"))
	g.Expect(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpExists))
	g.Expect(len(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values)).Should(gomega.BeEquivalentTo(0))
	g.Expect(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[1].Key).Should(gomega.BeEquivalentTo("region"))
	g.Expect(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[1].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpIn))
	g.Expect(len(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[1].Values)).Should(gomega.BeEquivalentTo(1))
	g.Expect(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[1].Values[0]).Should(gomega.BeEquivalentTo(caseName))
	g.Expect(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[2].Key).Should(gomega.BeEquivalentTo("node-name"))
	g.Expect(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[2].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpIn))
	g.Expect(len(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[2].Values)).Should(gomega.BeEquivalentTo(1))
	g.Expect(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[2].Values[0]).Should(gomega.BeEquivalentTo("node-b"))

	g.Expect(len(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[1].MatchExpressions)).Should(gomega.BeEquivalentTo(2))
	g.Expect(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[1].MatchExpressions[0].Key).Should(gomega.BeEquivalentTo("test"))
	g.Expect(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[1].MatchExpressions[0].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpDoesNotExist))
	g.Expect(len(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[1].MatchExpressions[0].Values)).Should(gomega.BeEquivalentTo(0))
	g.Expect(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[1].MatchExpressions[1].Key).Should(gomega.BeEquivalentTo("node-name"))
	g.Expect(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[1].MatchExpressions[1].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpIn))
	g.Expect(len(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[1].MatchExpressions[1].Values)).Should(gomega.BeEquivalentTo(1))
	g.Expect(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[1].MatchExpressions[1].Values[0]).Should(gomega.BeEquivalentTo("node-b"))
}

func TestDeploymentSubsetPatch(t *testing.T) {
	g, requests, cancel, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		cancel()
		mgrStopped.Wait()
	}()

	caseName := "test-deployment-subset-patch"

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
				DeploymentTemplate: &appsv1alpha1.DeploymentTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1.DeploymentSpec{
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

	deploymentList := expectedDeploymentCount(g, instance, 4)
	deployment := getDeploymentSubsetByName(deploymentList, "subset-a")
	g.Expect(deployment.Spec).ShouldNot(gomega.BeNil())
	g.Expect(deployment.Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))

	deployment = getDeploymentSubsetByName(deploymentList, "subset-b")
	g.Expect(deployment.Spec).ShouldNot(gomega.BeNil())
	g.Expect(deployment.Spec.Template.Labels).Should(gomega.HaveKeyWithValue("zone", "a"))

	deployment = getDeploymentSubsetByName(deploymentList, "subset-c")
	g.Expect(deployment.Spec).ShouldNot(gomega.BeNil())
	g.Expect(deployment.Spec.Template.Spec.Containers[0].Resources).ShouldNot(gomega.BeNil())
	g.Expect(deployment.Spec.Template.Spec.Containers[0].Resources.Limits).ShouldNot(gomega.BeNil())
	g.Expect(deployment.Spec.Template.Spec.Containers[0].Resources.Limits.Cpu()).ShouldNot(gomega.BeNil())
	g.Expect(deployment.Spec.Template.Spec.Containers[0].Resources.Limits.Cpu().Value()).Should(gomega.BeEquivalentTo(2))
	g.Expect(deployment.Spec.Template.Spec.Containers[0].Resources.Limits.Memory()).ShouldNot(gomega.BeNil())
	g.Expect(deployment.Spec.Template.Spec.Containers[0].Resources.Limits.Memory().String()).Should(gomega.BeEquivalentTo("800Mi"))

	deployment = getDeploymentSubsetByName(deploymentList, "subset-d")
	g.Expect(deployment.Spec).ShouldNot(gomega.BeNil())
	g.Expect(deployment.Spec.Template.Spec.Containers[0].Env).ShouldNot(gomega.BeNil())
	g.Expect(deployment.Spec.Template.Spec.Containers[0].Env[0].Name).Should(gomega.BeEquivalentTo("K8S_CONTAINER_NAME"))
	g.Expect(deployment.Spec.Template.Spec.Containers[0].Env[0].Value).Should(gomega.BeEquivalentTo("main"))

}

func TestDeploymentSubsetProvisionWithToleration(t *testing.T) {
	g, requests, cancel, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		cancel()
		mgrStopped.Wait()
	}()

	caseName := "test-deployment-subset-provision-with-tolerationssss"
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
				DeploymentTemplate: &appsv1alpha1.DeploymentTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1.DeploymentSpec{
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

	deploymentList := expectedDeploymentCount(g, instance, 1)
	deployment := &deploymentList.Items[0]
	g.Expect(deployment.Spec.Template.Spec.Tolerations).ShouldNot(gomega.BeNil())
	g.Expect(len(deployment.Spec.Template.Spec.Tolerations)).Should(gomega.BeEquivalentTo(2))
	g.Expect(reflect.DeepEqual(deployment.Spec.Template.Spec.Tolerations[0], instance.Spec.Topology.Subsets[0].Tolerations[0])).Should(gomega.BeTrue())
	g.Expect(reflect.DeepEqual(deployment.Spec.Template.Spec.Tolerations[1], instance.Spec.Topology.Subsets[0].Tolerations[1])).Should(gomega.BeTrue())

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	instance.Spec.Template.DeploymentTemplate.Spec.Template.Spec.Tolerations = append(instance.Spec.Template.DeploymentTemplate.Spec.Template.Spec.Tolerations, corev1.Toleration{
		Key:      "taint-0",
		Operator: corev1.TolerationOpExists,
	})

	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	deploymentList = expectedDeploymentCount(g, instance, 1)
	deployment = &deploymentList.Items[0]
	g.Expect(deployment.Spec.Template.Spec.Tolerations).ShouldNot(gomega.BeNil())
	g.Expect(len(deployment.Spec.Template.Spec.Tolerations)).Should(gomega.BeEquivalentTo(3))
	g.Expect(reflect.DeepEqual(deployment.Spec.Template.Spec.Tolerations[1], instance.Spec.Topology.Subsets[0].Tolerations[0])).Should(gomega.BeTrue())
	g.Expect(reflect.DeepEqual(deployment.Spec.Template.Spec.Tolerations[2], instance.Spec.Topology.Subsets[0].Tolerations[1])).Should(gomega.BeTrue())
}

func TestDeploymentDupSubset(t *testing.T) {
	g, requests, cancel, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		cancel()
		mgrStopped.Wait()
	}()

	caseName := "test-deployment-dup-subset"
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
				DeploymentTemplate: &appsv1alpha1.DeploymentTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1.DeploymentSpec{
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

	deploymentList := expectedDeploymentCount(g, instance, 1)

	subsetA := deploymentList.Items[0]
	dupDeployment := subsetA.DeepCopy()
	dupDeployment.Name = "dup-subset-a"
	dupDeployment.ResourceVersion = ""
	g.Expect(c.Create(context.TODO(), dupDeployment)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 3)
	expectedDeploymentCount(g, instance, 1)
}

func TestDeploymentScale(t *testing.T) {
	g, requests, cancel, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		cancel()
		mgrStopped.Wait()
	}()

	caseName := "test-deployment-scale"
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
				DeploymentTemplate: &appsv1alpha1.DeploymentTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1.DeploymentSpec{
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

	deploymentList := expectedDeploymentCount(g, instance, 2)
	g.Expect(*deploymentList.Items[0].Spec.Replicas + *deploymentList.Items[1].Spec.Replicas).Should(gomega.BeEquivalentTo(1))

	var two int32 = 2
	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	instance.Spec.Replicas = &two
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	deploymentList = expectedDeploymentCount(g, instance, 2)
	g.Expect(*deploymentList.Items[0].Spec.Replicas).Should(gomega.BeEquivalentTo(1))
	g.Expect(*deploymentList.Items[1].Spec.Replicas).Should(gomega.BeEquivalentTo(1))

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

	deploymentList = expectedDeploymentCount(g, instance, 2)
	g.Expect(*deploymentList.Items[0].Spec.Replicas).Should(gomega.BeEquivalentTo(3))
	g.Expect(*deploymentList.Items[1].Spec.Replicas).Should(gomega.BeEquivalentTo(3))

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

	deploymentList = expectedDeploymentCount(g, instance, 2)
	g.Expect(*deploymentList.Items[0].Spec.Replicas).Should(gomega.BeEquivalentTo(2))
	g.Expect(*deploymentList.Items[1].Spec.Replicas).Should(gomega.BeEquivalentTo(2))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	g.Expect(instance.Status.SubsetReplicas).Should(gomega.BeEquivalentTo(map[string]int32{
		"subset-a": 2,
		"subset-b": 2,
	}))
}

func TestDeploymentUpdate(t *testing.T) {
	g, requests, cancel, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		cancel()
		mgrStopped.Wait()
	}()

	caseName := "test-deployment-update"
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
				DeploymentTemplate: &appsv1alpha1.DeploymentTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1.DeploymentSpec{
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

	deploymentList := expectedDeploymentCount(g, instance, 2)
	g.Expect(*deploymentList.Items[0].Spec.Replicas + *deploymentList.Items[1].Spec.Replicas).Should(gomega.BeEquivalentTo(2))
	revisionList := &appsv1.ControllerRevisionList{}
	g.Expect(c.List(context.TODO(), revisionList))
	g.Expect(len(revisionList.Items)).Should(gomega.BeEquivalentTo(1))
	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	v1 := revisionList.Items[0].Name
	g.Expect(instance.Status.CurrentRevision).Should(gomega.BeEquivalentTo(v1))

	instance.Spec.Template.DeploymentTemplate.Spec.Template.Spec.Containers[0].Image = "nginx:2.0"
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	deploymentList = expectedDeploymentCount(g, instance, 2)
	g.Expect(deploymentList.Items[0].Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))
	g.Expect(deploymentList.Items[1].Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))

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

func TestDeploymentOnDelete(t *testing.T) {
	g, requests, cancel, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		cancel()
		mgrStopped.Wait()
	}()

	caseName := "test-deployment-on-delete"
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
				DeploymentTemplate: &appsv1alpha1.DeploymentTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1.DeploymentSpec{
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

	deploymentList := expectedDeploymentCount(g, instance, 2)
	g.Expect(*deploymentList.Items[0].Spec.Replicas).Should(gomega.BeEquivalentTo(5))
	g.Expect(*deploymentList.Items[1].Spec.Replicas).Should(gomega.BeEquivalentTo(5))

	// update with partition
	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	instance.Spec.UpdateStrategy.ManualUpdate = &appsv1alpha1.ManualUpdate{
		Partitions: map[string]int32{
			"subset-a": 4,
			"subset-b": 3,
		},
	}
	instance.Spec.Template.DeploymentTemplate.Spec.Template.Spec.Containers[0].Image = "nginx:2.0"
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	deploymentList = expectedDeploymentCount(g, instance, 2)
	g.Expect(deploymentList.Items[0].Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))
	g.Expect(deploymentList.Items[1].Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))

	deploymentA := getDeploymentSubsetByName(deploymentList, "subset-a")
	g.Expect(deploymentA).ShouldNot(gomega.BeNil())

	deploymentB := getDeploymentSubsetByName(deploymentList, "subset-b")
	g.Expect(deploymentB).ShouldNot(gomega.BeNil())

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

	deploymentList = expectedDeploymentCount(g, instance, 2)
	g.Expect(deploymentList.Items[0].Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))
	g.Expect(deploymentList.Items[1].Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))
}

func TestDeploymentSubsetCount(t *testing.T) {
	g, requests, cancel, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		cancel()
		mgrStopped.Wait()
	}()

	caseName := "test-deployment-subset-count"
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
				DeploymentTemplate: &appsv1alpha1.DeploymentTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1.DeploymentSpec{
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

	deploymentList := expectedDeploymentCount(g, instance, 2)
	g.Expect(*deploymentList.Items[0].Spec.Replicas).Should(gomega.BeEquivalentTo(5))
	g.Expect(*deploymentList.Items[1].Spec.Replicas).Should(gomega.BeEquivalentTo(5))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	nine := intstr.FromInt32(9)
	instance.Spec.Topology.Subsets[0].Replicas = &nine
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	deploymentList = expectedDeploymentCount(g, instance, 2)
	setsubA := getDeploymentSubsetByName(deploymentList, "subset-a")
	g.Expect(*setsubA.Spec.Replicas).Should(gomega.BeEquivalentTo(9))
	setsubB := getDeploymentSubsetByName(deploymentList, "subset-b")
	g.Expect(*setsubB.Spec.Replicas).Should(gomega.BeEquivalentTo(1))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	percentage := intstr.FromString("40%")
	instance.Spec.Topology.Subsets[0].Replicas = &percentage
	instance.Spec.Template.DeploymentTemplate.Spec.Template.Spec.Containers[0].Image = "nginx:2.0"
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	deploymentList = expectedDeploymentCount(g, instance, 2)
	setsubA = getDeploymentSubsetByName(deploymentList, "subset-a")
	g.Expect(*setsubA.Spec.Replicas).Should(gomega.BeEquivalentTo(4))
	g.Expect(setsubA.Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))
	setsubB = getDeploymentSubsetByName(deploymentList, "subset-b")
	g.Expect(*setsubB.Spec.Replicas).Should(gomega.BeEquivalentTo(6))
	g.Expect(setsubB.Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	percentage = intstr.FromString("30%")
	instance.Spec.Topology.Subsets[0].Replicas = &percentage
	instance.Spec.Template.DeploymentTemplate.Spec.Template.Spec.Containers[0].Image = "nginx:3.0"
	instance.Spec.UpdateStrategy.Type = appsv1alpha1.ManualUpdateStrategyType
	instance.Spec.UpdateStrategy.ManualUpdate = &appsv1alpha1.ManualUpdate{
		Partitions: map[string]int32{
			"subset-a": 1,
		},
	}
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	deploymentList = expectedDeploymentCount(g, instance, 2)
	setsubA = getDeploymentSubsetByName(deploymentList, "subset-a")
	g.Expect(*setsubA.Spec.Replicas).Should(gomega.BeEquivalentTo(3))
	g.Expect(setsubA.Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:3.0"))
	setsubB = getDeploymentSubsetByName(deploymentList, "subset-b")
	g.Expect(*setsubB.Spec.Replicas).Should(gomega.BeEquivalentTo(7))
	g.Expect(setsubB.Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:3.0"))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	percentage = intstr.FromString("20%")
	instance.Spec.Topology.Subsets[0].Replicas = &percentage
	instance.Spec.Template.DeploymentTemplate.Spec.Template.Spec.Containers[0].Image = "nginx:4.0"
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

	deploymentList = expectedDeploymentCount(g, instance, 3)
	setsubA = getDeploymentSubsetByName(deploymentList, "subset-a")
	g.Expect(*setsubA.Spec.Replicas).Should(gomega.BeEquivalentTo(2))
	g.Expect(setsubA.Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:4.0"))
	setsubB = getDeploymentSubsetByName(deploymentList, "subset-b")
	g.Expect(*setsubB.Spec.Replicas).Should(gomega.BeEquivalentTo(4))
	g.Expect(setsubB.Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:4.0"))
	setsubB = getDeploymentSubsetByName(deploymentList, "subset-c")
	g.Expect(*setsubB.Spec.Replicas).Should(gomega.BeEquivalentTo(4))
	g.Expect(setsubB.Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:4.0"))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	percentage = intstr.FromString("10%")
	instance.Spec.Topology.Subsets[0].Replicas = &percentage
	instance.Spec.Template.DeploymentTemplate.Spec.Template.Spec.Containers[0].Image = "nginx:5.0"
	instance.Spec.UpdateStrategy.ManualUpdate.Partitions = map[string]int32{
		"subset-a": 2,
	}
	instance.Spec.Topology.Subsets = instance.Spec.Topology.Subsets[:2]
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 3)

	deploymentList = expectedDeploymentCount(g, instance, 2)
	setsubA = getDeploymentSubsetByName(deploymentList, "subset-a")
	g.Expect(*setsubA.Spec.Replicas).Should(gomega.BeEquivalentTo(1))
	g.Expect(setsubA.Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:5.0"))
	setsubB = getDeploymentSubsetByName(deploymentList, "subset-b")
	g.Expect(*setsubB.Spec.Replicas).Should(gomega.BeEquivalentTo(9))
	g.Expect(setsubB.Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:5.0"))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	g.Expect(instance.Spec.UpdateStrategy.ManualUpdate.Partitions["subset-a"]).Should(gomega.BeEquivalentTo(2))
	percentage = intstr.FromString("40%")
	instance.Spec.Topology.Subsets[0].Replicas = &percentage
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 3)

	deploymentList = expectedDeploymentCount(g, instance, 2)
	setsubA = getDeploymentSubsetByName(deploymentList, "subset-a")
	g.Expect(*setsubA.Spec.Replicas).Should(gomega.BeEquivalentTo(4))
	g.Expect(setsubA.Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:5.0"))
	setsubB = getDeploymentSubsetByName(deploymentList, "subset-b")
	g.Expect(*setsubB.Spec.Replicas).Should(gomega.BeEquivalentTo(6))
	g.Expect(setsubB.Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:5.0"))
}

func TestDeploymentRollingUpdate(t *testing.T) {
	g, requests, cancel, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		cancel()
		mgrStopped.Wait()
	}()

	caseName := "test-deployment-rolling-update"
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
				DeploymentTemplate: &appsv1alpha1.DeploymentTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1.DeploymentSpec{
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
						Strategy: appsv1.DeploymentStrategy{
							Type: appsv1.RollingUpdateDeploymentStrategyType,
							RollingUpdate: &appsv1.RollingUpdateDeployment{
								MaxSurge: &intstr.IntOrString{
									Type:   intstr.Int,
									IntVal: 1,
								},
								MaxUnavailable: &intstr.IntOrString{
									Type:   intstr.Int,
									IntVal: 4,
								},
							},
						},
						MinReadySeconds: ten,
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
	g.Expect(c.Create(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)
	deploymentList := expectedDeploymentCount(g, instance, 2)
	for _, name := range []string{"subset-a", "subset-b"} {
		subset := getDeploymentSubsetByName(deploymentList, name)
		g.Expect(subset.Spec.Strategy.Type).Should(gomega.BeEquivalentTo(appsv1.RollingUpdateDeploymentStrategyType))
		g.Expect(subset.Spec.Strategy.RollingUpdate.MaxUnavailable.IntVal).Should(gomega.BeEquivalentTo(4))
		g.Expect(subset.Spec.Strategy.RollingUpdate.MaxSurge.IntVal).Should(gomega.BeEquivalentTo(1))
		g.Expect(subset.Spec.MinReadySeconds).Should(gomega.BeEquivalentTo(ten))
	}
}

func getDeploymentSubsetByName(deploymentList *appsv1.DeploymentList, name string) *appsv1.Deployment {
	for _, deployment := range deploymentList.Items {
		if deployment.Labels[appsv1alpha1.SubSetNameLabelKey] == name {
			return &deployment
		}
	}

	return nil
}

func expectedDeploymentCount(g *gomega.GomegaWithT, ud *appsv1alpha1.UnitedDeployment, count int) *appsv1.DeploymentList {
	deploymentList := &appsv1.DeploymentList{}

	selector, err := metav1.LabelSelectorAsSelector(ud.Spec.Selector)
	g.Expect(err).Should(gomega.BeNil())

	g.Eventually(func() error {
		if err := c.List(context.TODO(), deploymentList, &client.ListOptions{LabelSelector: selector}); err != nil {
			return err
		}

		if len(deploymentList.Items) != count {
			return fmt.Errorf("expected %d deployments, got %d", count, len(deploymentList.Items))
		}

		return nil
	}, timeout).Should(gomega.Succeed())

	return deploymentList
}

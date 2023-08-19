/*
Copyright 2022 The Kruise Authors.

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

// +kubebuilder:skip
package framework

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	kruiseappsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/util/configuration"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"

	"github.com/onsi/gomega"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacvapi1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	utilpointer "k8s.io/utils/pointer"
)

type PersistentPodStateTester struct {
	c  clientset.Interface
	d  dynamic.Interface
	a  apiextensionsclientset.Interface
	kc kruiseclientset.Interface
}

func NewPersistentPodStateTester(c clientset.Interface, kc kruiseclientset.Interface, d dynamic.Interface, a apiextensionsclientset.Interface) *PersistentPodStateTester {
	return &PersistentPodStateTester{
		a:  a,
		c:  c,
		d:  d,
		kc: kc,
	}
}

func (s *PersistentPodStateTester) NewBaseStatefulset(namespace string) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "StatefulSet",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        "statefulset",
			Namespace:   namespace,
			Annotations: map[string]string{},
			Labels:      map[string]string{},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: utilpointer.Int32Ptr(5),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "staticip",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "staticip",
					},
					Annotations: map[string]string{},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "main",
							Image:   "busybox:1.34",
							Command: []string{"/bin/sh", "-c", "sleep 10000000"},
						},
					},
					TerminationGracePeriodSeconds: utilpointer.Int64Ptr(5),
				},
			},
		},
	}
}

func (s *PersistentPodStateTester) WaitForStatefulsetRunning(sts *appsv1.StatefulSet) {
	pollErr := wait.PollImmediate(time.Second, time.Minute*5,
		func() (bool, error) {
			inner, err := s.c.AppsV1().StatefulSets(sts.Namespace).Get(context.TODO(), sts.Name, metav1.GetOptions{})
			if err != nil {
				return false, nil
			}
			if inner.Generation != inner.Status.ObservedGeneration {
				return false, nil
			}
			if *inner.Spec.Replicas == inner.Status.ReadyReplicas && *inner.Spec.Replicas == inner.Status.UpdatedReplicas &&
				*inner.Spec.Replicas == inner.Status.Replicas {
				return true, nil
			}
			return false, nil
		})
	if pollErr != nil {
		Failf("Failed waiting for statefulset(%s/%s) to enter running: %v", sts.Namespace, sts.Name, pollErr)
	}
}

func (s *PersistentPodStateTester) CreateStatefulset(sts *appsv1.StatefulSet) {
	Logf("create StatefulSet(%s/%s)", sts.Namespace, sts.Name)
	_, err := s.c.AppsV1().StatefulSets(sts.Namespace).Create(context.TODO(), sts, metav1.CreateOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	s.WaitForStatefulsetRunning(sts)
}

func (s *PersistentPodStateTester) ListPodsInKruiseSts(sts *appsv1.StatefulSet) ([]*corev1.Pod, error) {
	podList, err := s.c.CoreV1().Pods(sts.Namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	pods := make([]*corev1.Pod, 0)
	for i := range podList.Items {
		pod := &podList.Items[i]
		if pod.DeletionTimestamp.IsZero() {
			pods = append(pods, pod)
		}
	}
	return pods, nil
}

func (s *PersistentPodStateTester) UpdateStatefulset(sts *appsv1.StatefulSet) {
	Logf("update statefulset(%s/%s)", sts.Namespace, sts.Name)
	stsClone, _ := s.c.AppsV1().StatefulSets(sts.Namespace).Get(context.TODO(), sts.Name, metav1.GetOptions{})
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		stsClone.Spec = sts.Spec
		stsClone.Annotations = sts.Annotations
		stsClone.Labels = sts.Labels
		_, updateErr := s.c.AppsV1().StatefulSets(stsClone.Namespace).Update(context.TODO(), stsClone, metav1.UpdateOptions{})
		if updateErr == nil {
			return nil
		}
		stsClone, _ = s.c.AppsV1().StatefulSets(stsClone.Namespace).Get(context.TODO(), stsClone.Name, metav1.GetOptions{})
		return updateErr
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	time.Sleep(time.Second)
	s.WaitForStatefulsetRunning(sts)
}

func (s *PersistentPodStateTester) AddStatefulsetLikeClusterRoleConf() {
	cli := s.c.RbacV1().ClusterRoles()
	conf, err := cli.Get(context.TODO(), "kruise-manager-role", metav1.GetOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	conf.Rules = append(conf.Rules, rbacvapi1.PolicyRule{
		Verbs:     []string{"list", "watch", "get"},
		APIGroups: []string{kruiseappsv1beta1.GroupVersion.String(), "apps.kruise.io"},
		Resources: []string{"statefulsetlike"},
	})
	_, err = cli.Update(context.TODO(), conf, metav1.UpdateOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

func (s *PersistentPodStateTester) NewBaseStatefulsetLikeTest(namespace string) *StatefulSetLikeTest {
	return &StatefulSetLikeTest{
		TypeMeta: metav1.TypeMeta{
			Kind:       "StatefulSetLikeTest",
			APIVersion: "apps.kruise.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        "statefulsetlike",
			Namespace:   namespace,
			Annotations: map[string]string{},
			Labels:      map[string]string{},
		},
		Spec: StatefulSetLikeTestSpec{
			Replicas: utilpointer.Int32Ptr(5),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "staticip",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "staticip",
					},
					Annotations: map[string]string{},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "main",
							Image:   "nginx:1.23.1",
							Command: []string{"/bin/sh", "-c", "sleep 10000000"},
						},
					},
					TerminationGracePeriodSeconds: utilpointer.Int64Ptr(5),
				},
			},
		},
	}
}

func (s *PersistentPodStateTester) CheckStatefulsetLikeCRDExist() (bool, error) {
	_, err := s.a.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), "statefulsetliketests.apps.kruise.io", metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *PersistentPodStateTester) CreateStatefulsetLikeCRD(namespace string) {
	Logf("create StatefulSetLikeTest CRD")
	name := fmt.Sprintf("%v.%v", "statefulsetliketests", StatefulSetLikeTestKind.Group)
	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: StatefulSetLikeTestKind.Group,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:   "statefulsetliketests",
				Singular: "statefulsetliketest",
				Kind:     "StatefulSetLikeTest",
			},
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				apiextensionsv1.CustomResourceDefinitionVersion{
					Name:       StatefulSetLikeTestKind.Version,
					Served:     true,
					Storage:    true,
					Deprecated: false,
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
							XPreserveUnknownFields: utilpointer.BoolPtr(true),
							Type:                   "object",
						},
					},
				},
			},
			Scope: apiextensionsv1.NamespaceScoped,
		},
	}
	_, err := s.a.ApiextensionsV1().CustomResourceDefinitions().Create(context.TODO(), crd, metav1.CreateOptions{})
	if !errors.IsAlreadyExists(err) {
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	}
}

func (s *PersistentPodStateTester) CreateStatefulsetLike(sts *StatefulSetLikeTest) *StatefulSetLikeTest {
	Logf("create StatefulSetLikeTest(%s/%s)", sts.Namespace, sts.Name)
	obj := s.convertObjectToUnstructured(sts)
	cli := s.createStatefulSetLikeCli(sts.Namespace)

	resp, err := cli.Create(context.TODO(), obj, metav1.CreateOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	sts.SetResourceVersion(resp.GetResourceVersion())
	sts.SetUID(resp.GetUID())
	return sts
}

func (s *PersistentPodStateTester) createStatefulSetLikeCli(ns string) dynamic.ResourceInterface {
	gvr := schema.GroupVersionResource{
		Group:    "apps.kruise.io",
		Version:  "v1beta1",
		Resource: "statefulsetliketests",
	}
	return s.d.Resource(gvr).Namespace(ns)
}

func (s *PersistentPodStateTester) convertObjectToUnstructured(o runtime.Object) *unstructured.Unstructured {
	data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(o)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		//"apiVersion": "apps.kruise.io/v1beta1",
		//"kind":       "StatefulSetLikeTest",
		"apiVersion": o.GetObjectKind().GroupVersionKind().GroupVersion().String(),
		"kind":       o.GetObjectKind().GroupVersionKind().Kind,
		"metadata":   data["objectMeta"],
		"spec":       data["spec"],
	}}
	if status, ok := data["status"]; ok {
		obj.Object["status"] = status
	}
	return obj
}
func (s *PersistentPodStateTester) CreateDynamicWatchWhiteList(gvks []schema.GroupVersionKind) {
	var (
		name      = "kruise-configuration"
		namespace = "kruise-system"
		exist     = true
	)
	conf, err := s.c.CoreV1().ConfigMaps(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		}
		conf = &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "configmap",
				APIVersion: corev1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      name,
			},
			Data: map[string]string{},
		}
		exist = false
	}

	whiteList := make(map[string][]schema.GroupVersionKind, 0)
	if workloadStr, ok := conf.Data[configuration.PPSWatchCustomWorkloadWhiteList]; ok {
		err := json.Unmarshal([]byte(workloadStr), &whiteList)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	}
	whiteList["workloads"] = append(whiteList["workloads"], gvks...)
	workloadsData, _ := json.Marshal(whiteList)
	conf.Data[configuration.PPSWatchCustomWorkloadWhiteList] = string(workloadsData)

	if exist {
		_, err = s.c.CoreV1().ConfigMaps(conf.Namespace).Update(context.TODO(), conf, metav1.UpdateOptions{})
	} else {
		_, err = s.c.CoreV1().ConfigMaps(conf.Namespace).Create(context.TODO(), conf, metav1.CreateOptions{})
	}
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

func (s *PersistentPodStateTester) CreateStatefulsetLikePods(sts *StatefulSetLikeTest) {
	Logf("create StatefulSetLikeTest (%s/%s) pods", sts.Namespace, sts.Name)
	var (
		objectMeta      = sts.Spec.Template.ObjectMeta.DeepCopy()
		controller      = true
		ownerReferences = metav1.OwnerReference{
			APIVersion: sts.APIVersion,
			Kind:       sts.Kind,
			Name:       sts.Name,
			UID:        sts.UID,
			Controller: &controller,
		}
	)
	{
		objectMeta.SetNamespace(sts.Namespace)
		objectMeta.SetName(sts.Name)
		objectMeta.SetOwnerReferences([]metav1.OwnerReference{ownerReferences})
	}

	for i := 0; i < int(*sts.Spec.Replicas); i++ {
		meta := objectMeta.DeepCopy()
		name := fmt.Sprintf("%v-%v", sts.Name, i)
		meta.SetAnnotations(map[string]string{"name": name})
		meta.SetName(name)
		obj := &corev1.Pod{ObjectMeta: *meta, Spec: sts.Spec.Template.Spec}

		_, err := s.c.CoreV1().Pods(sts.Namespace).Create(context.TODO(), obj, metav1.CreateOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	}
	s.waitStatefulsetLikePodsRunning(sts)
}

func (s *PersistentPodStateTester) waitStatefulsetLikePodsRunning(sts *StatefulSetLikeTest) {
	pollErr := wait.PollImmediate(time.Second*3, time.Minute*5,
		func() (bool, error) {
			options := metav1.ListOptions{LabelSelector: "app=staticip"}
			items, err := s.c.CoreV1().Pods(sts.Namespace).List(context.TODO(), options)
			if err != nil {
				return false, err
			}
			readyCount := 0
			for _, pod := range items.Items {
				if podutil.IsPodReady(&pod) {
					readyCount += 1
				}
			}
			return readyCount == int(*sts.Spec.Replicas), nil
		})
	if pollErr != nil {
		Failf("Failed waiting for statefulsetlike(%s/%s) to enter running: %v", sts.Namespace, sts.Name, pollErr)
	}
}

func (s *PersistentPodStateTester) UpdateStatefulsetLikeStatus(sts *StatefulSetLikeTest) {
	Logf("update StatefulSetLikeTest (%s/%s) status", sts.Namespace, sts.Name)
	options := metav1.ListOptions{LabelSelector: "app=staticip"}
	items, err := s.c.CoreV1().Pods(sts.Namespace).List(context.TODO(), options)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	status := StatefulSetLikeStatusSpec{Replicas: *sts.Spec.Replicas, ReadyReplicas: 0}
	for _, pod := range items.Items {
		if podutil.IsPodReady(&pod) {
			status.ReadyReplicas += 1
		}
	}

	cli := s.createStatefulSetLikeCli(sts.Namespace)
	data, err := cli.Get(context.TODO(), sts.Name, metav1.GetOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	data.Object["status"] = status
	_, err = cli.Update(context.TODO(), data, metav1.UpdateOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

func (s *PersistentPodStateTester) ListPodsInNamespace(ns string) ([]*corev1.Pod, error) {
	podList, err := s.c.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	pods := make([]*corev1.Pod, 0)
	for i := range podList.Items {
		pod := &podList.Items[i]
		if pod.DeletionTimestamp.IsZero() {
			pods = append(pods, pod)
		}
	}
	return pods, nil
}

var StatefulSetLikeTestKind = kruiseappsv1beta1.GroupVersion.WithKind("StatefulSetLikeTest")

type StatefulSetLikeTest struct {
	metav1.TypeMeta
	metav1.ObjectMeta
	Spec   StatefulSetLikeTestSpec   `json:"spec"`
	Status StatefulSetLikeStatusSpec `json:"status"`
}

type StatefulSetLikeTestSpec struct {
	Replicas *int32                 `json:"replicas,omitempty" protobuf:"varint,1,opt,name=replicas"`
	Selector *metav1.LabelSelector  `json:"selector" protobuf:"bytes,2,opt,name=selector"`
	Template corev1.PodTemplateSpec `json:"template" protobuf:"bytes,3,opt,name=template"`
}

type StatefulSetLikeStatusSpec struct {
	Replicas      int32 `json:"replicas" protobuf:"varint,2,opt,name=replicas"`
	ReadyReplicas int32 `json:"readyReplicas,omitempty" protobuf:"varint,3,opt,name=readyReplicas"`
}

func (in *StatefulSetLikeTest) DeepCopyObject() runtime.Object {
	return in.DeepCopy()
}

func (in *StatefulSetLikeTest) DeepCopy() *StatefulSetLikeTest {
	return &StatefulSetLikeTest{
		TypeMeta: metav1.TypeMeta{
			Kind:       in.TypeMeta.Kind,
			APIVersion: in.TypeMeta.APIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      in.ObjectMeta.Name,
			Namespace: in.ObjectMeta.Namespace,
			UID:       in.ObjectMeta.UID,
		},
		Spec: StatefulSetLikeTestSpec{
			Replicas: utilpointer.Int32(*in.Spec.Replicas),
			Template: *in.Spec.Template.DeepCopy(),
		},
	}
}

func (in *StatefulSetLikeTest) DeepCopyInto(out *StatefulSetLikeTest) {
	out.TypeMeta = in.TypeMeta
	out.ObjectMeta = in.ObjectMeta
	out.Spec = StatefulSetLikeTestSpec{
		Replicas: in.Spec.Replicas,
		Selector: in.Spec.Selector,
		Template: in.Spec.Template,
	}
}

type StatefulSetLikeTestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []StatefulSetLikeTest `json:"items"`
}

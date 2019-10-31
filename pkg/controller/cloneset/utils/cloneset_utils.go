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

package utils

import (
	"context"

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ControllerKind is GroupVersionKind for CloneSet.
var ControllerKind = appsv1alpha1.SchemeGroupVersion.WithKind("CloneSet")

// GetActivePods returns all active pods in this namespace.
func GetActivePods(reader client.Reader, namespace string) ([]*v1.Pod, error) {
	podList := &v1.PodList{}
	if err := reader.List(context.TODO(), client.InNamespace(namespace), podList); err != nil {
		return nil, err
	}

	// Ignore inactive pods
	var activePods []*v1.Pod
	for i, pod := range podList.Items {
		// Consider all rebuild pod as active pod, should not recreate
		if kubecontroller.IsPodActive(&pod) {
			activePods = append(activePods, &podList.Items[i])
		}
	}
	return activePods, nil
}

// GetPodRevision returns revision hash of this pod.
func GetPodRevision(pod metav1.Object) string {
	return pod.GetLabels()[apps.ControllerRevisionHashLabelKey]
}

// GetPodsRevisions return revision hash set of these pods.
func GetPodsRevisions(pods []*v1.Pod) sets.String {
	revisions := sets.NewString()
	for _, p := range pods {
		revisions.Insert(GetPodRevision(p))
	}
	return revisions
}

// NextRevision finds the next valid revision number based on revisions. If the length of revisions
// is 0 this is 1. Otherwise, it is 1 greater than the largest revision's Revision. This method
// assumes that revisions has been sorted by Revision.
func NextRevision(revisions []*apps.ControllerRevision) int64 {
	count := len(revisions)
	if count <= 0 {
		return 1
	}
	return revisions[count-1].Revision + 1
}

// IsRunningAndReady returns true if pod is in the PodRunning Phase, if it has a condition of PodReady.
func IsRunningAndReady(pod *v1.Pod) bool {
	return pod.Status.Phase == v1.PodRunning && podutil.IsPodReady(pod)
}

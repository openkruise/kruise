/*
Copyright 2019 The Kruise Authors.
Copyright 2015 The Kubernetes Authors.

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

package framework

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
)

const (
	// isDefaultStorageClassAnnotation represents a StorageClass annotation that
	// marks a class as the default StorageClass
	isDefaultStorageClassAnnotation = "storageclass.kubernetes.io/is-default-class"

	// betaIsDefaultStorageClassAnnotation is the beta version of IsDefaultStorageClassAnnotation.
	// TODO: remove Beta when no longer used
	betaIsDefaultStorageClassAnnotation = "storageclass.beta.kubernetes.io/is-default-class"
)

// create the PV resource. Fails test on error.
func createPV(c clientset.Interface, pv *v1.PersistentVolume) (*v1.PersistentVolume, error) {
	pv, err := c.CoreV1().PersistentVolumes().Create(context.TODO(), pv, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("PV Create API error: %v", err)
	}
	return pv, nil
}

// CreatePV creates the PV resource. Fails test on error.
func CreatePV(c clientset.Interface, pv *v1.PersistentVolume) (*v1.PersistentVolume, error) {
	return createPV(c, pv)
}

// SkipIfNoDefaultStorageClass skips tests if no default SC can be found.
func SkipIfNoDefaultStorageClass(c clientset.Interface) bool {
	_, err := GetDefaultStorageClassName(c)
	if err != nil {
		Logf("error finding default storageClass : %v", err)
		return true
	}
	return false
}

// GetDefaultStorageClassName returns default storageClass or return error
func GetDefaultStorageClassName(c clientset.Interface) (string, error) {
	list, err := c.StorageV1().StorageClasses().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("Error listing storage classes: %v", err)
	}
	var scName string
	for _, sc := range list.Items {
		if isDefaultAnnotation(sc.ObjectMeta) {
			if len(scName) != 0 {
				return "", fmt.Errorf("Multiple default storage classes found: %q and %q", scName, sc.Name)
			}
			scName = sc.Name
		}
	}
	if len(scName) == 0 {
		return "", fmt.Errorf("No default storage class found")
	}
	Logf("Default storage class: %q", scName)
	return scName, nil
}

// isDefaultAnnotation returns a boolean if the default storage class
// annotation is set
// TODO: remove Beta when no longer needed
func isDefaultAnnotation(obj metav1.ObjectMeta) bool {
	if obj.Annotations[isDefaultStorageClassAnnotation] == "true" {
		return true
	}
	if obj.Annotations[betaIsDefaultStorageClassAnnotation] == "true" {
		return true
	}

	return false
}

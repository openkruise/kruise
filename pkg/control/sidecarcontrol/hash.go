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

package sidecarcontrol

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/util/rand"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

// SidecarSetHash returns a hash of the SidecarSet.
// The Containers are taken into account.
func SidecarSetHash(sidecarSet *appsv1alpha1.SidecarSet) (string, error) {
	encoded, err := encodeSidecarSet(sidecarSet)
	if err != nil {
		return "", err
	}
	h := rand.SafeEncodeString(hash(encoded))
	return h, nil
}

// SidecarSetHashWithoutImage calculates sidecars's container hash without its image
// we use this to determine if the sidecar reconcile needs to update a pod image
func SidecarSetHashWithoutImage(sidecarSet *appsv1alpha1.SidecarSet) (string, error) {
	ss := sidecarSet.DeepCopy()
	for i := range ss.Spec.Containers {
		ss.Spec.Containers[i].Image = ""
	}
	for i := range ss.Spec.InitContainers {
		ss.Spec.InitContainers[i].Image = ""
	}
	encoded, err := encodeSidecarSet(ss)
	if err != nil {
		return "", err
	}
	return rand.SafeEncodeString(hash(encoded)), nil
}

func encodeSidecarSet(sidecarSet *appsv1alpha1.SidecarSet) (string, error) {
	// json.Marshal sorts the keys in a stable order in the encoding
	m := map[string]interface{}{"containers": sidecarSet.Spec.Containers}
	// when k8s 1.28, if initContainer restartPolicy = Always, indicates it is sidecar container, so the hash needs to contain it.
	initContainer := make([]appsv1alpha1.SidecarContainer, 0)
	for i := range sidecarSet.Spec.InitContainers {
		container := &sidecarSet.Spec.InitContainers[i]
		if IsSidecarContainer(container.Container) {
			initContainer = append(initContainer, *container)
		}
	}
	if len(initContainer) > 0 {
		m["initContainers"] = sidecarSet.Spec.InitContainers
	}
	data, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// SidecarSetHashV1beta1 returns a hash of the SidecarSet v1beta1.
// The Containers are taken into account.
func SidecarSetHashV1beta1(sidecarSet *appsv1beta1.SidecarSet) (string, error) {
	encoded, err := encodeSidecarSetV1beta1(sidecarSet)
	if err != nil {
		return "", err
	}
	h := rand.SafeEncodeString(hash(encoded))
	return h, nil
}

// SidecarSetHashWithoutImageV1beta1 calculates sidecars's container hash without its image for v1beta1
// we use this to determine if the sidecar reconcile needs to update a pod image
func SidecarSetHashWithoutImageV1beta1(sidecarSet *appsv1beta1.SidecarSet) (string, error) {
	ss := sidecarSet.DeepCopy()
	for i := range ss.Spec.Containers {
		ss.Spec.Containers[i].Image = ""
	}
	for i := range ss.Spec.InitContainers {
		ss.Spec.InitContainers[i].Image = ""
	}
	encoded, err := encodeSidecarSetV1beta1(ss)
	if err != nil {
		return "", err
	}
	return rand.SafeEncodeString(hash(encoded)), nil
}

func encodeSidecarSetV1beta1(sidecarSet *appsv1beta1.SidecarSet) (string, error) {
	// json.Marshal sorts the keys in a stable order in the encoding
	m := map[string]interface{}{"containers": sidecarSet.Spec.Containers}
	// when k8s 1.28, if initContainer restartPolicy = Always, indicates it is sidecar container, so the hash needs to contain it.
	initContainer := make([]appsv1beta1.SidecarContainer, 0)
	for i := range sidecarSet.Spec.InitContainers {
		container := &sidecarSet.Spec.InitContainers[i]
		if IsSidecarContainer(container.Container) {
			initContainer = append(initContainer, *container)
		}
	}
	if len(initContainer) > 0 {
		m["initContainers"] = sidecarSet.Spec.InitContainers
	}
	data, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// hash hashes `data` with sha256 and returns the hex string
func hash(data string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(data)))
}

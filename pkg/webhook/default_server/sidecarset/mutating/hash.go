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

package mutating

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/util/rand"

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
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

// SidecarSetHashWithoutImage calculates hash without sidecars's image
func SidecarSetHashWithoutImage(sidecarSet *appsv1alpha1.SidecarSet) (string, error) {
	ss := sidecarSet.DeepCopy()
	for i := range ss.Spec.Containers {
		ss.Spec.Containers[i].Image = ""
	}
	return SidecarSetHash(ss)
}

func encodeSidecarSet(sidecarSet *appsv1alpha1.SidecarSet) (string, error) {
	// json.Marshal sorts the keys in a stable order in the encoding
	m := map[string]interface{}{"containers": sidecarSet.Spec.Containers}
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

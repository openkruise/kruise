/*
Copyright 2023 The Kruise Authors.

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

package revision

import (
	"strings"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	"github.com/openkruise/kruise/pkg/features"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	"github.com/openkruise/kruise/pkg/util/lifecycle"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
)

// IsPodUpdate return true when:
// - Pod controller-revision-hash equals to updateRevision;
// - Pod at preparing update state if PreparingUpdateAsUpdate feature-gated is enabled.
func IsPodUpdate(pod *v1.Pod, updateRevision string) bool {
	if utilfeature.DefaultFeatureGate.Enabled(features.PreparingUpdateAsUpdate) &&
		lifecycle.GetPodLifecycleState(pod) == appspub.LifecycleStatePreparingUpdate {
		return true
	}
	return equalToRevisionHash("", pod, updateRevision)
}

func equalToRevisionHash(s string, pod *v1.Pod, hash string) bool {
	objHash := pod.GetLabels()[apps.ControllerRevisionHashLabelKey]
	if objHash == hash {
		return true
	}
	return getShortHash(hash) == getShortHash(objHash)
}

func getShortHash(hash string) string {
	// This makes sure the real hash must be the last '-' substring of revision name
	// vendor/k8s.io/kubernetes/pkg/controller/history/controller_history.go#82
	list := strings.Split(hash, "-")
	return list[len(list)-1]
}

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

package features

import (
	"os"
	"strings"

	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/component-base/featuregate"
)

const (
	// ImagePulling enables controllers for NodeImage and ImagePullJob.
	ImagePulling featuregate.Feature = "ImagePulling"

	// PodWebhook enables webhook for Pods creations. This is also related to SidecarSet.
	PodWebhook featuregate.Feature = "PodWebhook"
)

var defaultFeatureGates = map[featuregate.Feature]featuregate.FeatureSpec{
	PodWebhook:   {Default: true, PreRelease: featuregate.Beta},
	ImagePulling: {Default: true, PreRelease: featuregate.Beta},
}

func init() {
	compatibleEnv()
	runtime.Must(utilfeature.DefaultMutableFeatureGate.Add(defaultFeatureGates))
}

// Make it compatible with the old CUSTOM_RESOURCE_ENABLE gate in env.
func compatibleEnv() {
	str := strings.TrimSpace(os.Getenv("CUSTOM_RESOURCE_ENABLE"))
	if len(str) == 0 {
		return
	}
	limits := sets.NewString(strings.Split(str, ",")...)
	if !limits.Has("SidecarSet") {
		defaultFeatureGates[PodWebhook] = featuregate.FeatureSpec{Default: false, PreRelease: featuregate.Beta}
	}
}

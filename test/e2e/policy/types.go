/*
Copyright 2019 The Kruise Authors.
Copyright 2017 The Kubernetes Authors.

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

package policy

import (
	imageutils "k8s.io/kubernetes/test/utils/image"
)

// NOTE(claudiub): These constants should NOT be used as Pod Container Images.
const (
	WebserverImageName = "httpd"
	AgnhostImageName   = "agnhost"
)

var (
	// WebserverImage is the fully qualified URI to the Httpd image
	WebserverImage = imageutils.GetE2EImage(imageutils.Httpd)

	// NewWebserverImage is the fully qualified URI to the HttpdNew image
	NewWebserverImage = imageutils.GetE2EImage(imageutils.HttpdNew)

	// NginxImage is the fully qualified URI to the Nginx image
	NginxImage = imageutils.GetE2EImage(imageutils.Nginx)

	// NewNginxImage is the fully qualified URI to the NginxNew image
	NewNginxImage = imageutils.GetE2EImage(imageutils.NginxNew)

	// InvalidImage is the fully qualified URI to the invalid image
	InvalidImage = imageutils.GetE2EImage(imageutils.InvalidRegistryImage)
)

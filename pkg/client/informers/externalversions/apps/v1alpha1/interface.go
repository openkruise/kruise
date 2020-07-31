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
// Code generated by informer-gen. DO NOT EDIT.

package v1alpha1

import (
	internalinterfaces "github.com/openkruise/kruise/pkg/client/informers/externalversions/internalinterfaces"
)

// Interface provides access to all the informers in this group version.
type Interface interface {
	// BroadcastJobs returns a BroadcastJobInformer.
	BroadcastJobs() BroadcastJobInformer
	// CloneSets returns a CloneSetInformer.
	CloneSets() CloneSetInformer
	// DaemonSets returns a DaemonSetInformer.
	DaemonSets() DaemonSetInformer
	// ImagePullJobs returns a ImagePullJobInformer.
	ImagePullJobs() ImagePullJobInformer
	// NodeImages returns a NodeImageInformer.
	NodeImages() NodeImageInformer
	// SidecarSets returns a SidecarSetInformer.
	SidecarSets() SidecarSetInformer
	// StatefulSets returns a StatefulSetInformer.
	StatefulSets() StatefulSetInformer
	// UnitedDeployments returns a UnitedDeploymentInformer.
	UnitedDeployments() UnitedDeploymentInformer
}

type version struct {
	factory          internalinterfaces.SharedInformerFactory
	namespace        string
	tweakListOptions internalinterfaces.TweakListOptionsFunc
}

// New returns a new Interface.
func New(f internalinterfaces.SharedInformerFactory, namespace string, tweakListOptions internalinterfaces.TweakListOptionsFunc) Interface {
	return &version{factory: f, namespace: namespace, tweakListOptions: tweakListOptions}
}

// BroadcastJobs returns a BroadcastJobInformer.
func (v *version) BroadcastJobs() BroadcastJobInformer {
	return &broadcastJobInformer{factory: v.factory, namespace: v.namespace, tweakListOptions: v.tweakListOptions}
}

// CloneSets returns a CloneSetInformer.
func (v *version) CloneSets() CloneSetInformer {
	return &cloneSetInformer{factory: v.factory, namespace: v.namespace, tweakListOptions: v.tweakListOptions}
}

// DaemonSets returns a DaemonSetInformer.
func (v *version) DaemonSets() DaemonSetInformer {
	return &daemonSetInformer{factory: v.factory, namespace: v.namespace, tweakListOptions: v.tweakListOptions}
}

// ImagePullJobs returns a ImagePullJobInformer.
func (v *version) ImagePullJobs() ImagePullJobInformer {
	return &imagePullJobInformer{factory: v.factory, namespace: v.namespace, tweakListOptions: v.tweakListOptions}
}

// NodeImages returns a NodeImageInformer.
func (v *version) NodeImages() NodeImageInformer {
	return &nodeImageInformer{factory: v.factory, namespace: v.namespace, tweakListOptions: v.tweakListOptions}
}

// SidecarSets returns a SidecarSetInformer.
func (v *version) SidecarSets() SidecarSetInformer {
	return &sidecarSetInformer{factory: v.factory, namespace: v.namespace, tweakListOptions: v.tweakListOptions}
}

// StatefulSets returns a StatefulSetInformer.
func (v *version) StatefulSets() StatefulSetInformer {
	return &statefulSetInformer{factory: v.factory, namespace: v.namespace, tweakListOptions: v.tweakListOptions}
}

// UnitedDeployments returns a UnitedDeploymentInformer.
func (v *version) UnitedDeployments() UnitedDeploymentInformer {
	return &unitedDeploymentInformer{factory: v.factory, namespace: v.namespace, tweakListOptions: v.tweakListOptions}
}

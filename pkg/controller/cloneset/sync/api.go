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

package sync

import (
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	clonesetutils "github.com/openkruise/kruise/pkg/controller/cloneset/utils"
	"github.com/openkruise/kruise/pkg/util/controllerfinder"
	"github.com/openkruise/kruise/pkg/util/inplaceupdate"
	"github.com/openkruise/kruise/pkg/util/lifecycle"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Interface for managing pods scaling and updating.
type Interface interface {
	Scale(
		currentCS, updateCS *appsv1alpha1.CloneSet,
		currentRevision, updateRevision string,
		pods []*v1.Pod, pvcs []*v1.PersistentVolumeClaim,
	) (bool, error)

	Update(cs *appsv1alpha1.CloneSet,
		currentRevision, updateRevision *apps.ControllerRevision, revisions []*apps.ControllerRevision,
		pods []*v1.Pod, pvcs []*v1.PersistentVolumeClaim,
	) error
}

type realControl struct {
	client.Client
	lifecycleControl lifecycle.Interface
	inplaceControl   inplaceupdate.Interface
	recorder         record.EventRecorder
	controllerFinder *controllerfinder.ControllerFinder
}

func New(c client.Client, recorder record.EventRecorder) Interface {
	return &realControl{
		Client:           c,
		inplaceControl:   inplaceupdate.New(c, clonesetutils.RevisionAdapterImpl),
		lifecycleControl: lifecycle.New(c),
		recorder:         recorder,
		controllerFinder: controllerfinder.Finder,
	}
}

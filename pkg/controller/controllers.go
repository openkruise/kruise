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

package controller

import (
	"github.com/openkruise/kruise/pkg/controller/advancedcronjob"
	"github.com/openkruise/kruise/pkg/controller/broadcastjob"
	"github.com/openkruise/kruise/pkg/controller/cloneset"
	containerlauchpriority "github.com/openkruise/kruise/pkg/controller/containerlaunchpriority"
	"github.com/openkruise/kruise/pkg/controller/containerrecreaterequest"
	"github.com/openkruise/kruise/pkg/controller/daemonset"
	"github.com/openkruise/kruise/pkg/controller/imagepulljob"
	"github.com/openkruise/kruise/pkg/controller/nodeimage"
	"github.com/openkruise/kruise/pkg/controller/podreadiness"
	"github.com/openkruise/kruise/pkg/controller/podunavailablebudget"
	"github.com/openkruise/kruise/pkg/controller/resourcedistribution"
	"github.com/openkruise/kruise/pkg/controller/sidecarset"
	"github.com/openkruise/kruise/pkg/controller/statefulset"
	"github.com/openkruise/kruise/pkg/controller/uniteddeployment"
	"github.com/openkruise/kruise/pkg/controller/workloadspread"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var controllerAddFuncs []func(manager.Manager) error

func init() {
	controllerAddFuncs = append(controllerAddFuncs, advancedcronjob.Add)
	controllerAddFuncs = append(controllerAddFuncs, broadcastjob.Add)
	controllerAddFuncs = append(controllerAddFuncs, cloneset.Add)
	controllerAddFuncs = append(controllerAddFuncs, containerrecreaterequest.Add)
	controllerAddFuncs = append(controllerAddFuncs, daemonset.Add)
	controllerAddFuncs = append(controllerAddFuncs, nodeimage.Add)
	controllerAddFuncs = append(controllerAddFuncs, imagepulljob.Add)
	controllerAddFuncs = append(controllerAddFuncs, podreadiness.Add)
	controllerAddFuncs = append(controllerAddFuncs, sidecarset.Add)
	controllerAddFuncs = append(controllerAddFuncs, statefulset.Add)
	controllerAddFuncs = append(controllerAddFuncs, uniteddeployment.Add)
	controllerAddFuncs = append(controllerAddFuncs, podunavailablebudget.Add)
	controllerAddFuncs = append(controllerAddFuncs, workloadspread.Add)
	controllerAddFuncs = append(controllerAddFuncs, resourcedistribution.Add)
	controllerAddFuncs = append(controllerAddFuncs, containerlauchpriority.Add)
}

func SetupWithManager(m manager.Manager) error {
	for _, f := range controllerAddFuncs {
		if err := f(m); err != nil {
			if kindMatchErr, ok := err.(*meta.NoKindMatchError); ok {
				klog.Infof("CRD %v is not installed, its controller will perform noops!", kindMatchErr.GroupKind)
				continue
			}
			return err
		}
	}
	return nil
}

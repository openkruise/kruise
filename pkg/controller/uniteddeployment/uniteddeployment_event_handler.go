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

package uniteddeployment

import (
	"context"

	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

type eventHandler struct {
	handler.TypedEnqueueRequestForObject[*appsv1alpha1.UnitedDeployment]
}

func (e *eventHandler) Delete(ctx context.Context, evt event.TypedDeleteEvent[*appsv1alpha1.UnitedDeployment], q workqueue.RateLimitingInterface) {
	klog.InfoS("cleaning up UnitedDeployment", "unitedDeployment", evt.Object)
	ResourceVersionExpectation.Delete(evt.Object)
	e.TypedEnqueueRequestForObject.Delete(ctx, evt, q)
}

func (e *eventHandler) Update(ctx context.Context, evt event.TypedUpdateEvent[*appsv1alpha1.UnitedDeployment], q workqueue.RateLimitingInterface) {
	// make sure latest version is observed
	ResourceVersionExpectation.Observe(evt.ObjectNew)
	e.TypedEnqueueRequestForObject.Update(ctx, evt, q)
}

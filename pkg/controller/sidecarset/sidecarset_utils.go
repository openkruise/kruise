package sidecarset

import (
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

func isSidecarSetUpdateFinish(status *appsv1alpha1.SidecarSetStatus) bool {
	return status.UpdatedPods >= status.MatchedPods
}

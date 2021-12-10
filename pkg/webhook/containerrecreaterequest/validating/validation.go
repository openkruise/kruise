package validating

import (
	"context"
	"fmt"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/control/pubcontrol"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/util/dryrun"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// podUnavailableBudgetValidating allowed(bool) indicates whether to allow this crr create operation
func (h *ContainerRecreateRequestHandler) podUnavailableBudgetValidating(ctx context.Context, req admission.Request) (allowed bool, reason string, err error) {
	crr := &appsv1alpha1.ContainerRecreateRequest{}
	switch req.AdmissionRequest.Operation {
	case admissionv1.Create:
		err := h.Decoder.Decode(req, crr)
		if err != nil {
			return false, "", err
		}
		pod := &corev1.Pod{}
		if err := h.Client.Get(ctx, types.NamespacedName{Namespace: crr.Namespace, Name: crr.Spec.PodName}, pod); err != nil {
			if errors.IsNotFound(err) {
				return false, fmt.Sprintf("no found Pod named %s", crr.Spec.PodName), err
			}
			return false, fmt.Sprintf("failed to find Pod %s", crr.Spec.PodName), err
		}
		pub, err := pubcontrol.GetPodUnavailableBudgetForPod(h.Client, h.finders, pod)
		if err != nil {
			return false, "", err
		}
		// if there is no matching PodUnavailableBudget, just return true
		if pub == nil {
			return true, "", nil
		}
		control := pubcontrol.NewPubControl(pub, h.finders, h.Client)
		klog.V(3).Infof("validating crr(%s.%s) operation(%s) for pub(%s.%s)", crr.Namespace, crr.Name, req.Operation, pub.Namespace, pub.Name)

		creation := &metav1.CreateOptions{}
		err = h.Decoder.DecodeRaw(req.Options, creation)
		if err != nil {
			return false, "", err
		}
		// if dry run
		dryRun := dryrun.IsDryRun(creation.DryRun)
		// pods that contain annotations[pod.kruise.io/pub-no-protect]="true" will be ignore
		// and will no longer check the pub quota
		if pod.Annotations[pubcontrol.PodPubNoProtectionAnnotation] == "true" {
			klog.V(3).Infof("pod(%s.%s) contains annotations[%s], then don't need check pub", pod.Namespace, pod.Name, pubcontrol.PodPubNoProtectionAnnotation)
			return true, "", nil
		}
		return pubcontrol.PodUnavailableBudgetValidatePod(h.Client, pod, control, pubcontrol.Operation(req.Operation), dryRun)
	}

	return
}

package resourcedistribution

import (
	"github.com/openkruise/kruise/apis/apps/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	ResourceDistributionFailed         = "Failed: create, update, delete failed or not synchronized yet in namespaces"
	ResourceDistributionSucceed        = "Successful: this resource has been distributed to all target namespaces"
	ResourceDistributionListAnnotation = "kruise.io/resourcedistribution-injected-list"
)

func NamespaceMatchedResourceDistribution(namespace *corev1.Namespace, resourceDistribution *v1alpha1.ResourceDistribution) (bool, error) {
	// if selector not matched, then continue
	selector, err := metav1.LabelSelectorAsSelector(&resourceDistribution.Spec.Targets.NamespaceLabelSelector)
	if err != nil {
		return false, err
	}

	if !selector.Empty() && selector.Matches(labels.Set(namespace.Labels)) {
		return true, nil
	}
	return false, nil
}

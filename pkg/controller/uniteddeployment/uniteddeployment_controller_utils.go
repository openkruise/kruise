package uniteddeployment

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
)

const updateRetries = 5

func getSubsetName(obj *metav1.ObjectMeta) string {
	if name, exist := obj.Labels[appsv1alpha1.SubSetNameLabelKey]; exist {
		return name
	}

	return ""
}

// ParseSubsetReplicas parses the subsetReplicas, and returns the replicas number depending on the sum replicas.
func ParseSubsetReplicas(udReplicas int32, subsetReplicas intstr.IntOrString) (int32, error) {
	if subsetReplicas.Type == intstr.Int {
		if subsetReplicas.IntVal < 0 {
			return 0, fmt.Errorf("subset replicas (%d) should not be less than 0", subsetReplicas.IntVal)
		}
		return subsetReplicas.IntVal, nil
	}

	strVal := subsetReplicas.StrVal
	if !strings.HasSuffix(strVal, "%") {
		return 0, fmt.Errorf("subset replicas (%s) only support integer value or percentage value with a suffix '%%'", strVal)
	}

	intPart := strVal[:len(strVal)-1]
	percent64, err := strconv.ParseInt(intPart, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("subset replicas (%s) should be correct percentage integer: %s", strVal, err)
	}

	if percent64 > int64(100) || percent64 < int64(0) {
		return 0, fmt.Errorf("subset replicas (%s) should be in range [0, 100]", strVal)
	}

	return int32(round(float64(udReplicas) * float64(percent64) / 100)), nil
}

func round(x float64) int {
	return int(math.Floor(x + 0.5))
}

func getPodsPrefix(controllerName string) string {
	// use the dash (if the name isn't too long) to make the pod name a bit prettier
	prefix := fmt.Sprintf("%s-", controllerName)
	if len(validation.NameIsDNSSubdomain(prefix, true)) != 0 {
		prefix = controllerName
	}
	return prefix
}

func attachNodeAffinity(podSpec *corev1.PodSpec, subsetConfig *appsv1alpha1.Subset) {
	if podSpec.Affinity == nil {
		podSpec.Affinity = &corev1.Affinity{}
	}

	if podSpec.Affinity.NodeAffinity == nil {
		podSpec.Affinity.NodeAffinity = &corev1.NodeAffinity{}
	}

	if podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{}
	}

	if podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms == nil {
		podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms = []corev1.NodeSelectorTerm{}
	}

	for _, term := range subsetConfig.NodeSelector.NodeSelectorTerms {
		podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms = append(podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms, term)
	}
}

func getSubsetNameFrom(metaObj metav1.Object) (string, error) {
	name, exist := metaObj.GetLabels()[appsv1alpha1.SubSetNameLabelKey]
	if !exist {
		return "", fmt.Errorf("fail to get subSet name from label of subset %s/%s: no label %s found", metaObj.GetNamespace(), metaObj.GetName(), appsv1alpha1.SubSetNameLabelKey)
	}

	if len(name) == 0 {
		return "", fmt.Errorf("fail to get subSet name from label of subset %s/%s: label %s has an empty value", metaObj.GetNamespace(), metaObj.GetName(), appsv1alpha1.SubSetNameLabelKey)
	}

	return name, nil
}

func getRevision(objMeta *metav1.ObjectMeta) string {
	if objMeta.Labels == nil {
		return ""
	}
	return objMeta.Labels[appsv1alpha1.ControllerRevisionHashLabelKey]
}

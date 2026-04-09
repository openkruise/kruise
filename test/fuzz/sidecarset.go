package fuzz

import (
	"encoding/json"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/util/configuration"
)

type SidecarSetGenerateSpecFunc = func(cf *fuzz.ConsumeFuzzer, subset *appsv1beta1.SidecarSet) error

func GenerateSidecarSetSpec(cf *fuzz.ConsumeFuzzer, sidecarSet *appsv1beta1.SidecarSet, fns ...SidecarSetGenerateSpecFunc) error {
	if len(fns) == 0 {
		return nil
	}

	for _, fn := range fns {
		if err := fn(cf, sidecarSet); err != nil {
			return err
		}
	}
	return nil
}

func GenerateSidecarSetSelector(cf *fuzz.ConsumeFuzzer, sidecarSet *appsv1beta1.SidecarSet) error {
	selector := &metav1.LabelSelector{}
	if err := GenerateLabelSelector(cf, selector); err != nil {
		return err
	}
	sidecarSet.Spec.Selector = selector
	return nil
}

func GenerateSidecarSetNamespace(cf *fuzz.ConsumeFuzzer, sidecarSet *appsv1beta1.SidecarSet) error {
	// Namespace field has been removed from v1beta1, use GenerateSidecarSetNamespaceSelector instead
	return GenerateSidecarSetNamespaceSelector(cf, sidecarSet)
}

func GenerateSidecarSetNamespaceSelector(cf *fuzz.ConsumeFuzzer, sidecarSet *appsv1beta1.SidecarSet) error {
	selector := &metav1.LabelSelector{}
	if err := GenerateLabelSelector(cf, selector); err != nil {
		return err
	}
	sidecarSet.Spec.NamespaceSelector = selector
	return nil
}

func GenerateSidecarSetInitContainer(cf *fuzz.ConsumeFuzzer, sidecarSet *appsv1beta1.SidecarSet) error {
	isStructured, err := cf.GetBool()
	if err != nil {
		return err
	}

	if !isStructured {
		initContainers := make([]appsv1beta1.SidecarContainer, 0)
		if err := cf.CreateSlice(&initContainers); err != nil {
			return err
		}
		sidecarSet.Spec.InitContainers = initContainers
		return nil
	}

	initContainers := make([]appsv1beta1.SidecarContainer, r.Intn(collectionMaxElements)+1)
	for i := range initContainers {
		if err := GenerateSidecarContainer(cf, &initContainers[i]); err != nil {
			return err
		}
	}
	sidecarSet.Spec.InitContainers = initContainers
	return nil
}

func GenerateSidecarSetContainer(cf *fuzz.ConsumeFuzzer, sidecarSet *appsv1beta1.SidecarSet) error {
	isStructured, err := cf.GetBool()
	if err != nil {
		return err
	}

	if !isStructured {
		containers := make([]appsv1beta1.SidecarContainer, 0)
		if err := cf.CreateSlice(&containers); err != nil {
			return err
		}
		sidecarSet.Spec.Containers = containers
		return nil
	}

	containers := make([]appsv1beta1.SidecarContainer, r.Intn(collectionMaxElements)+1)
	for i := range containers {
		if err := GenerateSidecarContainer(cf, &containers[i]); err != nil {
			return err
		}
	}
	sidecarSet.Spec.Containers = containers
	return nil
}

func GenerateSidecarContainer(cf *fuzz.ConsumeFuzzer, sidecarContainer *appsv1beta1.SidecarContainer) error {
	isStructured, err := cf.GetBool()
	if err != nil {
		return err
	}

	if err := cf.GenerateStruct(sidecarContainer); err != nil {
		return err
	}

	if !isStructured {
		return nil
	}

	choice, err := cf.GetInt()
	if err != nil {
		return err
	}

	validPodInjectPolicyType := []appsv1beta1.PodInjectPolicyType{
		appsv1beta1.BeforeAppContainerType,
		appsv1beta1.AfterAppContainerType,
	}

	validSidecarContainerUpgradeType := []appsv1beta1.SidecarContainerUpgradeType{
		appsv1beta1.SidecarContainerColdUpgrade,
		appsv1beta1.SidecarContainerHotUpgrade,
	}

	validShareVolumePolicy := []appsv1beta1.ShareVolumePolicyType{
		appsv1beta1.ShareVolumePolicyEnabled,
		appsv1beta1.ShareVolumePolicyDisabled,
	}

	validRestartPolicy := []corev1.ContainerRestartPolicy{
		corev1.ContainerRestartPolicyAlways,
	}

	sidecarContainer.PodInjectPolicy = validPodInjectPolicyType[choice%(len(validPodInjectPolicyType))]
	sidecarContainer.UpgradeStrategy.UpgradeType = validSidecarContainerUpgradeType[choice%(len(validSidecarContainerUpgradeType))]
	sidecarContainer.ShareVolumePolicy.Type = validShareVolumePolicy[choice%(len(validShareVolumePolicy))]
	sidecarContainer.ShareVolumeDevicePolicy.Type = validShareVolumePolicy[choice%(len(validShareVolumePolicy))]

	isValid, err := cf.GetBool()
	if err != nil {
		return err
	}
	if isValid {
		sidecarContainer.Container.RestartPolicy = &validRestartPolicy[choice%(len(validRestartPolicy))]
	}
	return nil
}

func GenerateSidecarSetUpdateStrategy(cf *fuzz.ConsumeFuzzer, sidecarSet *appsv1beta1.SidecarSet) error {
	isStructured, err := cf.GetBool()
	if err != nil {
		return err
	}

	updateStrategy := appsv1beta1.SidecarSetUpdateStrategy{}
	if err := cf.GenerateStruct(&updateStrategy); err != nil {
		return err
	}

	if !isStructured {
		sidecarSet.Spec.UpdateStrategy = updateStrategy
		return nil
	}

	validStrategyTypes := []appsv1beta1.SidecarSetUpdateStrategyType{
		appsv1beta1.NotUpdateSidecarSetStrategyType,
		appsv1beta1.RollingUpdateSidecarSetStrategyType,
	}
	choice, err := cf.GetInt()
	if err != nil {
		return err
	}
	updateStrategy.Type = validStrategyTypes[choice%len(validStrategyTypes)]

	selector := &metav1.LabelSelector{}
	if err := GenerateLabelSelector(cf, selector); err != nil {
		return err
	}
	updateStrategy.Selector = selector

	if partition, err := GenerateIntOrString(cf); err == nil {
		updateStrategy.Partition = &partition
	}
	if maxUnavailable, err := GenerateIntOrString(cf); err == nil {
		updateStrategy.MaxUnavailable = &maxUnavailable
	}
	sidecarSet.Spec.UpdateStrategy = updateStrategy

	return nil
}

func GenerateSidecarSetInjectionStrategy(cf *fuzz.ConsumeFuzzer, sidecarSet *appsv1beta1.SidecarSet) error {
	isStructured, err := cf.GetBool()
	if err != nil {
		return err
	}

	injectionStrategy := appsv1beta1.SidecarSetInjectionStrategy{}
	if err := cf.GenerateStruct(&injectionStrategy); err != nil {
		return err
	}

	if !isStructured {
		sidecarSet.Spec.InjectionStrategy = injectionStrategy
		return nil
	}

	validPolicies := []appsv1beta1.SidecarSetInjectRevisionPolicy{
		appsv1beta1.AlwaysSidecarSetInjectRevisionPolicy,
		appsv1beta1.PartialSidecarSetInjectRevisionPolicy,
	}
	choice, err := cf.GetInt()
	if err != nil {
		return err
	}

	injectRevision := &appsv1beta1.SidecarSetInjectRevision{
		RevisionName:  func() *string { name := GenerateValidValue(); return &name }(),
		CustomVersion: func() *string { version := GenerateValidValue(); return &version }(),
		Policy:        validPolicies[choice%len(validPolicies)],
	}
	injectionStrategy.Revision = injectRevision
	sidecarSet.Spec.InjectionStrategy = injectionStrategy

	return nil
}

func GenerateSidecarSetPatchPodMetadata(cf *fuzz.ConsumeFuzzer, sidecarSet *appsv1beta1.SidecarSet) error {
	sliceLen, err := cf.GetInt()
	if err != nil {
		return err
	}

	sidecarSetPatchSlice := make([]appsv1beta1.SidecarSetPatchPodMetadata, sliceLen%3+1)
	for i := range sidecarSetPatchSlice {
		jsonPatch, err := cf.GetBool()
		if err != nil {
			return err
		}
		patch, err := GeneratePatchPodMetadata(cf, jsonPatch)
		if err != nil || patch == nil {
			return err
		}
		sidecarSetPatchSlice[i] = *patch
	}
	sidecarSet.Spec.PatchPodMetadata = sidecarSetPatchSlice
	return nil
}

func GeneratePatchPodMetadata(cf *fuzz.ConsumeFuzzer, jsonPatch bool) (*appsv1beta1.SidecarSetPatchPodMetadata, error) {
	isStructured, err := cf.GetBool()
	if err != nil {
		return nil, err
	}

	sidecarSetPatch := &appsv1beta1.SidecarSetPatchPodMetadata{}
	if !isStructured {
		if err := cf.GenerateStruct(sidecarSetPatch); err != nil {
			return nil, err
		}
		return sidecarSetPatch, nil
	}

	validPatchPolicies := []appsv1beta1.SidecarSetPatchPolicyType{
		appsv1beta1.SidecarSetMergePatchJsonPatchPolicy,
		appsv1beta1.SidecarSetRetainPatchPolicy,
		appsv1beta1.SidecarSetOverwritePatchPolicy,
	}

	annotations := make(map[string]string)
	if jsonPatch {
		m := make(map[string]string)
		m[GenerateValidKey()] = GenerateValidValue()
		bytes, _ := json.Marshal(m)
		annotations[GenerateValidKey()] = string(bytes)
	} else {
		annotations[GenerateValidKey()] = GenerateValidValue()
	}
	sidecarSetPatch.Annotations = annotations

	choice, err := cf.GetInt()
	if err != nil {
		return sidecarSetPatch, nil
	}
	sidecarSetPatch.PatchPolicy = validPatchPolicies[choice%len(validPatchPolicies)]

	return sidecarSetPatch, nil
}

func GenerateSidecarSetWhiteListRule(cf *fuzz.ConsumeFuzzer, whiteList *configuration.SidecarSetPatchMetadataWhiteList) error {
	sliceLen, err := cf.GetInt()
	if err != nil {
		return err
	}

	whiteListRuleSlice := make([]configuration.SidecarSetPatchMetadataWhiteRule, sliceLen%3+1)
	for i := range whiteListRuleSlice {
		allowedAnnotationKeyExprs := make([]string, sliceLen%3+1)
		for j := range allowedAnnotationKeyExprs {
			expr, err := cf.GetString()
			if err != nil {
				return err
			}
			allowedAnnotationKeyExprs[j] = expr
		}

		selector := &metav1.LabelSelector{}
		if err := GenerateLabelSelector(cf, selector); err != nil {
			return err
		}
		whiteListRuleSlice[i] = configuration.SidecarSetPatchMetadataWhiteRule{
			Selector:                  selector,
			AllowedAnnotationKeyExprs: allowedAnnotationKeyExprs,
		}
	}

	whiteList.Rules = whiteListRuleSlice

	return nil
}

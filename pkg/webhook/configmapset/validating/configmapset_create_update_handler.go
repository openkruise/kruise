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

package validating

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"k8s.io/kubernetes/pkg/controller"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	genericvalidation "k8s.io/apimachinery/pkg/api/validation"
	validationutil "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/openkruise/kruise/pkg/controller/configmapset"
	"github.com/openkruise/kruise/pkg/util"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

const (
	configMapSetNameMaxLen = 63
)

var (
	validateConfigMapSetNameMsg   = "ConfigMapSet name must consist of alphanumeric characters or '-'"
	validateConfigMapSetNameRegex = regexp.MustCompile(validConfigMapSetNameFmt)
	validConfigMapSetNameFmt      = `^[a-zA-Z0-9\-]+$`
)

// ConfigMapSetCreateUpdateHandler handles ConfigMapSet
type ConfigMapSetCreateUpdateHandler struct {
	Client client.Client
	// Decoder decodes objects
	Decoder admission.Decoder
}

func (h *ConfigMapSetCreateUpdateHandler) validateConfigMapSetSpec(ctx context.Context, name, namespace string, spec *appsv1alpha1.ConfigMapSetSpec, fldPath *field.Path) *field.Error {
	if spec == nil {
		return field.Required(fldPath.Child("spec"), "")
	}
	if spec.Selector == nil {
		return field.Required(fldPath.Child("selector"), "selector is required")
	}

	if spec.Selector.MatchLabels == nil || len(spec.Selector.MatchLabels) == 0 {
		return field.Required(fldPath.Child("selector", "matchLabels"), "")
	}

	if len(spec.UpdateStrategy.MatchLabelKeys) > 0 {
		for _, key := range spec.UpdateStrategy.MatchLabelKeys {
			if _, exists := spec.Selector.MatchLabels[key]; exists {
				return field.Invalid(fldPath.Child("updateStrategy", "matchLabelKeys"), spec.UpdateStrategy.MatchLabelKeys, fmt.Sprintf("matchLabelKeys cannot intersect with selector matchLabels, conflict key: %s", key))
			}
		}
	}

	containerNames := make(map[string]struct{})
	for i, c := range spec.Containers {
		if c.Name != "" {
			if _, exists := containerNames[c.Name]; exists {
				return field.Duplicate(fldPath.Child("containers").Index(i).Child("name"), c.Name)
			}
			containerNames[c.Name] = struct{}{}
		}
	}

	// 计算当前 Spec.Data 的 Hash
	hash, err := configmapset.CalculateHash(spec.Data)
	if err != nil {
		return field.InternalError(fldPath.Child("data"), fmt.Errorf("failed to compute hash: %v", err))
	}
	// 检查是否已经有相同的revision / customVersion存在
	// 重新获取最新的 ConfigMap，避免并发冲突
	cmName := fmt.Sprintf("%s-%s", strings.ToLower(name), "hub")
	cmNamespace := namespace
	cm := &corev1.ConfigMap{}
	if err = h.Client.Get(context.TODO(), types.NamespacedName{Name: cmName, Namespace: cmNamespace}, cm); err != nil {
		if !errors.IsNotFound(err) {
			return field.InternalError(fldPath.Child("data"), fmt.Errorf("failed to get ConfigMap: %v", err))
		}
		cm = nil
	}

	if cm != nil {
		var revisions []configmapset.RevisionEntry
		if revData, exists := cm.Data["revisions"]; exists {
			if err := json.Unmarshal([]byte(revData), &revisions); err != nil {
				klog.Errorf("Failed to unmarshal revisions from ConfigMap %s: %v, resetting revisions", cmName, err)
				//revisions = []RevisionEntry{} // 解析失败时重置 ?
				return field.InternalError(fldPath.Child("data"), fmt.Errorf("failed to unmarshal revisions from ConfigMap: %v", err))
			}
		}

		for _, rev := range revisions {
			// hash must same if customVersion same
			if rev.CustomVersion == spec.CustomVersion && rev.Hash != hash {
				return field.Invalid(fldPath.Child("customVersion"), spec.CustomVersion, "configmapset already exists with hash "+rev.Hash+" for customVersion "+spec.CustomVersion)
			}
		}
	}

	strategy := spec.UpdateStrategy
	if strategy.Partition != nil && !validatePartition(strategy.Partition) {
		return field.Invalid(fldPath.Child("updateStrategy"),
			spec.UpdateStrategy,
			"invalid partition value")
	}

	if strategy.MaxUnavailable != nil && !validateMaxUnavailable(strategy.MaxUnavailable) {
		return field.Invalid(fldPath.Child("updateStrategy"),
			spec.UpdateStrategy,
			"invalid maxUnavailable value")
	}

	// validate ReloadSidecarConfig
	if spec.ReloadSidecarConfig != nil {
		reloadConfigPath := fldPath.Child("reloadSidecarConfig")
		if spec.ReloadSidecarConfig.Type == appsv1alpha1.K8sConfigReloadSidecarType {
			if spec.ReloadSidecarConfig.Config == nil || spec.ReloadSidecarConfig.Config.Name == "" {
				return field.Invalid(reloadConfigPath.Child("config", "name"), "", "name must be set when type is k8s-config")
			}
		} else if spec.ReloadSidecarConfig.Type == appsv1alpha1.SidecarSetReloadSidecarType {
			if spec.ReloadSidecarConfig.Config == nil || spec.ReloadSidecarConfig.Config.SidecarSetRef == nil {
				return field.Invalid(reloadConfigPath.Child("config", "sidecarSetRef"), "", "sidecarSetRef must be set when type is SidecarSet")
			}
			sidecarSetRef := spec.ReloadSidecarConfig.Config.SidecarSetRef
			sidecarSet := &appsv1alpha1.SidecarSet{}
			err = h.Client.Get(ctx, types.NamespacedName{Name: sidecarSetRef.Name}, sidecarSet)
			if err != nil {
				if errors.IsNotFound(err) {
					return field.Invalid(reloadConfigPath.Child("config", "sidecarSetRef", "name"), sidecarSetRef.Name, "SidecarSet not found")
				}
				return field.InternalError(reloadConfigPath.Child("config", "sidecarSetRef", "name"), fmt.Errorf("failed to get SidecarSet: %v", err))
			}
			containerFound := false
			for _, c := range sidecarSet.Spec.Containers {
				if c.Name == sidecarSetRef.ContainerName && c.Image != "" {
					containerFound = true
					break
				}
			}
			if !containerFound {
				return field.Invalid(reloadConfigPath.Child("config", "sidecarSetRef", "containerName"), sidecarSetRef.ContainerName, "container not found in SidecarSet")
			}

			// Validate if SidecarSet includes the current ConfigMapSet's Pods
			if sidecarSet.Spec.Namespace != "" && sidecarSet.Spec.Namespace != namespace {
				return field.Invalid(reloadConfigPath.Child("config", "sidecarSetRef", "name"), sidecarSetRef.Name, fmt.Sprintf("SidecarSet targets namespace %s which does not match ConfigMapSet namespace %s", sidecarSet.Spec.Namespace, namespace))
			}

			// Validate NamespaceSelector
			if sidecarSet.Spec.NamespaceSelector != nil {
				nsObj := &corev1.Namespace{}
				err = h.Client.Get(ctx, types.NamespacedName{Name: namespace}, nsObj)
				if err != nil {
					if errors.IsNotFound(err) {
						return field.Invalid(field.NewPath("metadata", "namespace"), namespace, "Namespace not found")
					}
					return field.InternalError(field.NewPath("metadata", "namespace"), fmt.Errorf("failed to get namespace: %v", err))
				}
				selector, err := util.ValidatedLabelSelectorAsSelector(sidecarSet.Spec.NamespaceSelector)
				if err != nil {
					return field.Invalid(reloadConfigPath.Child("config", "sidecarSetRef", "name"), sidecarSetRef.Name, fmt.Sprintf("invalid NamespaceSelector in SidecarSet: %v", err))
				}
				if !selector.Matches(labels.Set(nsObj.Labels)) {
					return field.Invalid(reloadConfigPath.Child("config", "sidecarSetRef", "name"), sidecarSetRef.Name, fmt.Sprintf("SidecarSet NamespaceSelector does not match ConfigMapSet namespace %s", namespace))
				}
			}

			// Validate if ConfigMapSet selector is a subset of SidecarSet selector
			if sidecarSet.Spec.Selector != nil {
				sidecarSetSelector, err := util.ValidatedLabelSelectorAsSelector(sidecarSet.Spec.Selector)
				if err != nil {
					return field.Invalid(reloadConfigPath.Child("config", "sidecarSetRef", "name"), sidecarSetRef.Name, fmt.Sprintf("invalid selector in SidecarSet: %v", err))
				}
				// Here we use a simplified approach as requested:
				// We just check if the ConfigMapSet's MatchLabels satisfy the SidecarSet's Selector.
				// This assumes ConfigMapSet pods will at least have these MatchLabels.
				if !sidecarSetSelector.Matches(labels.Set(spec.Selector.MatchLabels)) {
					return field.Invalid(reloadConfigPath.Child("config", "sidecarSetRef", "name"), sidecarSetRef.Name, "ConfigMapSet MatchLabels do not satisfy SidecarSet selector")
				}
			}
		} else if spec.ReloadSidecarConfig.Type == appsv1alpha1.CustomerReloadSidecarType {
			if spec.ReloadSidecarConfig.Config == nil || spec.ReloadSidecarConfig.Config.ConfigMapRef == nil {
				return field.Invalid(reloadConfigPath.Child("config", "configMapRef"), "", "configMapRef must be set when type is customer")
			}
			cmRef := spec.ReloadSidecarConfig.Config.ConfigMapRef
			customerCM := &corev1.ConfigMap{}
			// Use the ConfigMapSet's namespace if the ConfigMapRef namespace is not specified
			cmRefNamespace := cmRef.Namespace
			if cmRefNamespace == "" {
				cmRefNamespace = namespace
			}
			err = h.Client.Get(ctx, types.NamespacedName{Name: cmRef.Name, Namespace: cmRefNamespace}, customerCM)
			if err != nil {
				if errors.IsNotFound(err) {
					return field.Invalid(reloadConfigPath.Child("config", "configMapRef", "name"), cmRef.Name, "customer sidecar ConfigMap not found")
				}
				return field.InternalError(reloadConfigPath.Child("config", "configMapRef", "name"), fmt.Errorf("failed to get customer sidecar ConfigMap: %v", err))
			}

			// Validate if the specific key "reload-sidecar" exists and can be unmarshaled into a valid Container
			containerData, exists := customerCM.Data["reload-sidecar"]
			if !exists {
				return field.Invalid(reloadConfigPath.Child("config", "configMapRef", "name"), cmRef.Name, "customer sidecar ConfigMap must contain key 'reload-sidecar'")
			}

			var reloadSidecar corev1.Container
			if err = json.Unmarshal([]byte(containerData), &reloadSidecar); err != nil {
				return field.Invalid(reloadConfigPath.Child("config", "configMapRef", "name"), cmRef.Name, fmt.Sprintf("failed to unmarshal 'reload-sidecar' data to Container: %v", err))
			}

			if reloadSidecar.Image == "" {
				return field.Invalid(reloadConfigPath.Child("config", "configMapRef", "name"), cmRef.Name, "container defined in 'reload-sidecar' must have image specified")
			}
		} else {
			return field.Invalid(reloadConfigPath.Child("type"), spec.ReloadSidecarConfig.Type, "invalid type, must be k8s-config, SidecarSet or customer")
		}
	}

	return nil
}

var percentagePattern = regexp.MustCompile(`^\d+%$`)

func validatePartition(partition *intstr.IntOrString) bool {
	if partition == nil {
		return false
	}

	switch partition.Type {
	case intstr.Int:
		return partition.IntVal >= 0

	case intstr.String:
		if !percentagePattern.MatchString(partition.StrVal) {
			return false
		}
		// 去掉 % 后验证是否为0-100的整数
		valStr := partition.StrVal[:len(partition.StrVal)-1]
		val, err := strconv.Atoi(valStr)
		return err == nil && val >= 0 && val <= 100
	default:
		return false
	}
}

// 验证 maxUnavailable 是否为 1~100 的百分比，或正整数（>0）
func validateMaxUnavailable(maxUnavailable *intstr.IntOrString) bool {
	if maxUnavailable == nil {
		return false
	}

	switch maxUnavailable.Type {
	case intstr.Int:
		if maxUnavailable.IntVal > 0 {
			return true
		}
	case intstr.String:
		s := strings.TrimSpace(maxUnavailable.StrVal)
		if strings.HasSuffix(s, "%") {
			numStr := strings.TrimSuffix(s, "%")
			num, err := strconv.Atoi(numStr)
			if err != nil || num < 0 || num > 100 {
				return false
			}
			return true
		}
		// 非百分比情况，解析为整数
		num, err := strconv.Atoi(s)
		if err != nil || num < 0 {
			return false
		}
		return true
	}
	return false
}

func validateConfigMapSetName(name string, prefix bool) (allErrs []string) {
	if !validateConfigMapSetNameRegex.MatchString(name) {
		allErrs = append(allErrs, validationutil.RegexError(validateConfigMapSetNameMsg, validConfigMapSetNameFmt, "example-com"))
	}
	if len(name) > configMapSetNameMaxLen {
		allErrs = append(allErrs, validationutil.MaxLenError(configMapSetNameMaxLen))
	}
	return allErrs
}

var _ admission.Handler = &ConfigMapSetCreateUpdateHandler{}

// Handle handles admission requests.
func (h *ConfigMapSetCreateUpdateHandler) validateConfigMapSet(ctx context.Context, configMapSet *appsv1alpha1.ConfigMapSet) field.ErrorList {
	metaErrs := genericvalidation.ValidateObjectMeta(&configMapSet.ObjectMeta, true, validateConfigMapSetName, field.NewPath("metadata"))
	if len(metaErrs) > 0 {
		return metaErrs
	}
	specErr := h.validateConfigMapSetSpec(ctx, configMapSet.Name, configMapSet.Namespace, &configMapSet.Spec, field.NewPath("spec"))
	if specErr != nil {
		return field.ErrorList{specErr}
	}
	return nil
}

// Handle handles admission requests.
func (h *ConfigMapSetCreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &appsv1alpha1.ConfigMapSet{}
	oldObj := &appsv1alpha1.ConfigMapSet{}

	switch req.AdmissionRequest.Operation {
	case admissionv1.Create, admissionv1.Update:
		err := h.Decoder.Decode(req, obj)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}

		if req.AdmissionRequest.Operation == admissionv1.Update {
			if len(req.OldObject.Raw) > 0 {
				if err := h.Decoder.DecodeRaw(req.AdmissionRequest.OldObject, oldObj); err != nil {
					return admission.Errored(http.StatusBadRequest, err)
				}
				if oldObj.Spec.Selector != nil && obj.Spec.Selector != nil {
					if !reflect.DeepEqual(oldObj.Spec.Selector.MatchLabels, obj.Spec.Selector.MatchLabels) {
						errList := field.ErrorList{field.Forbidden(field.NewPath("spec", "selector", "matchLabels"), "field is immutable")}
						return admission.Errored(http.StatusUnprocessableEntity, errList.ToAggregate())
					}
				}
			}
		}

		if allErrs := h.validateConfigMapSet(ctx, obj); len(allErrs) > 0 {
			return admission.Errored(http.StatusUnprocessableEntity, allErrs.ToAggregate())
		}
	case admissionv1.Delete:
		if len(req.OldObject.Raw) == 0 {
			klog.InfoS("Skip to validate CloneSet %s/%s deletion for no old object, maybe because of Kubernetes version < 1.16", "namespace", req.Namespace, "name", req.Name)
			return admission.ValidationResponse(true, "")
		}
		if err := h.Decoder.DecodeRaw(req.AdmissionRequest.OldObject, oldObj); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		err := h.validateDeleteConfigMapSet(ctx, oldObj)
		if err != nil {
			return admission.Errored(http.StatusForbidden, err)
		}
	}

	return admission.ValidationResponse(true, "")
}

func (h *ConfigMapSetCreateUpdateHandler) validateDeleteConfigMapSet(ctx context.Context, cms *appsv1alpha1.ConfigMapSet) error {
	selector := cms.Spec.Selector
	if selector == nil {
		return nil
	}
	matchLabels := selector.MatchLabels
	podList := &corev1.PodList{}
	err := h.Client.List(ctx, podList, client.InNamespace(cms.Namespace), client.MatchingLabels(matchLabels))
	if err != nil {
		return err
	}
	if len(podList.Items) == 0 {
		return nil
	}
	for _, pod := range podList.Items {
		if !controller.IsPodActive(&pod) {
			continue
		}

		if pod.Annotations == nil {
			continue
		}

		if pod.Annotations[configmapset.GetConfigMapSetEnabledKey()] == "true" {
			return fmt.Errorf("pod %s/%s is still used configmapSet:%s/%s", pod.Namespace, pod.Name, cms.Namespace, cms.Name)
		}
	}
	return nil
}

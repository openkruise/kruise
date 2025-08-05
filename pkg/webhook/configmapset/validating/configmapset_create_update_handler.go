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
	"github.com/openkruise/kruise/pkg/controller/configmapset"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/webhook/util/deletionprotection"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
	"net/http"
	"regexp"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strconv"
	"strings"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	genericvalidation "k8s.io/apimachinery/pkg/api/validation"
	validationutil "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
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

func (h *ConfigMapSetCreateUpdateHandler) validateConfigMapSetSpec(name, namespace string, spec *appsv1alpha1.ConfigMapSetSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// 计算当前 Spec.Data 的 Hash
	hash, err := configmapset.CalculateHash(spec.Data)
	if err != nil {
		return append(allErrs, field.InternalError(fldPath.Child("data"), fmt.Errorf("failed to compute hash: %v", err)))
	}
	var revisions []configmapset.RevisionEntry
	// 检查是否已经有相同的revision / customVersion存在
	// 重新获取最新的 ConfigMap，避免并发冲突
	cmName := fmt.Sprintf("%s-hub", name)
	cmNamespace := namespace
	cm := &corev1.ConfigMap{}
	if err := h.Client.Get(context.TODO(), types.NamespacedName{Name: cmName, Namespace: cmNamespace}, cm); err != nil {
		if errors.IsNotFound(err) {
			// 如果 ConfigMap 不存在，不用判断历史版本
			// do nothing
		} else {
			return append(allErrs, field.InternalError(fldPath.Child("data"), fmt.Errorf("failed to get ConfigMap: %v", err)))
		}
	} else {
		// error == nil
		// 解析现有 ConfigMap 的 revisions
		if revData, exists := cm.Data["revisions"]; exists {
			if err := json.Unmarshal([]byte(revData), &revisions); err != nil {
				klog.Errorf("Failed to unmarshal revisions from ConfigMap %s: %v, resetting revisions", cmName, err)
				//revisions = []RevisionEntry{} // 解析失败时重置 ?
				return append(allErrs, field.InternalError(fldPath.Child("data"), fmt.Errorf("failed to unmarshal revisions from ConfigMap: %v", err)))
			}
		}

		// 检查 Hash 是否已存在
		for _, rev := range revisions {
			// 如果已存在的customVersion == 当前customVersion
			// 但是hash不一样, 拒绝
			// 反之则允许
			if rev.CustomVersion == spec.CustomVersion && rev.Hash != hash {
				return append(allErrs, field.Invalid(fldPath.Child("customVersion"), spec.CustomVersion, "configmapset already exists with hash "+rev.Hash+" for customVersion "+spec.CustomVersion))
			}
		}
	}
	strategy := spec.UpdateStrategy

	if strategy.Partition != nil && !validatePartition(strategy.Partition) {
		return append(allErrs, field.Invalid(fldPath.Child("updateStrategy"),
			spec.UpdateStrategy,
			"invalid partition value"))
	}

	if strategy.MaxUnavailable != nil && !validateMaxUnavailable(strategy.MaxUnavailable) {
		return append(allErrs, field.Invalid(fldPath.Child("updateStrategy"),
			spec.UpdateStrategy,
			"invalid maxUnavailable value"))
	}

	// 禁止同时定义distribution 和 partition
	if len(strategy.Distributions) > 0 && strategy.Partition != nil {
		return append(allErrs, field.Invalid(fldPath.Child("updateStrategy"),
			spec.UpdateStrategy,
			"distributions and partition cannot be set at the same time"))
	}
	// 都不定义也不行
	if len(strategy.Distributions) == 0 && strategy.Partition == nil {
		return append(allErrs, field.Invalid(fldPath.Child("updateStrategy"),
			spec.UpdateStrategy,
			"at least one of distributions or partition must be set"))
	}
	cnt := 0

	for _, distribution := range strategy.Distributions {
		if distribution.Reserved <= 0 {
			return append(allErrs, field.Invalid(fldPath.Child("updateStrategy"),
				spec.UpdateStrategy,
				"reserved must be greater than 0"))
		}
		if distribution.Preferred { // 所有distribution 里面, Preferred有且只能有一个为true
			cnt++
			if cnt > 1 {
				return append(allErrs, field.Invalid(fldPath.Child("updateStrategy"),
					spec.UpdateStrategy,
					"only one preferred distribution is allowed"))
			}
		}
	}
	if len(strategy.Distributions) > 0 && cnt == 0 {
		return append(allErrs, field.Invalid(fldPath.Child("updateStrategy"),
			spec.UpdateStrategy,
			"at least one preferred distribution must be set"))
	}
	return allErrs
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
func (h *ConfigMapSetCreateUpdateHandler) validateConfigMapSet(configMapSet, oldConfigMapSet *appsv1alpha1.ConfigMapSet) field.ErrorList {
	allErrs := genericvalidation.ValidateObjectMeta(&configMapSet.ObjectMeta, true, validateConfigMapSetName, field.NewPath("metadata"))
	//var oldCloneSetSpec *appsv1alpha1.ConfigMapSetSpec
	if oldConfigMapSet != nil {
		//oldCloneSetSpec = &oldConfigMapSet.Spec
	}
	allErrs = append(allErrs, h.validateConfigMapSetSpec(configMapSet.Name, configMapSet.Namespace, &configMapSet.Spec, field.NewPath("spec"))...)
	return allErrs
	//allErrs := apivalidation.ValidateObjectMeta(&configMapSet.ObjectMeta, true, apimachineryvalidation.NameIsDNSSubdomain, field.NewPath("metadata"))
	//
	//allErrs = append(allErrs, h.validateConfigMapSetSpec(&configMapSet.Spec, oldCloneSetSpec, &configMapSet.ObjectMeta, field.NewPath("spec"))...)
	//return allErrs
}

// Handle handles admission requests.
func (h *ConfigMapSetCreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &appsv1alpha1.ConfigMapSet{}
	oldObj := &appsv1alpha1.CloneSet{}

	switch req.AdmissionRequest.Operation {
	case admissionv1.Create, admissionv1.Update:
		err := h.Decoder.Decode(req, obj)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		if allErrs := h.validateConfigMapSet(obj, nil); len(allErrs) > 0 {
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
		if err := deletionprotection.ValidateWorkloadDeletion(oldObj, oldObj.Spec.Replicas); err != nil {
			deletionprotection.WorkloadDeletionProtectionMetrics.WithLabelValues(fmt.Sprintf("%s_%s_%s", req.Kind.Kind, oldObj.GetNamespace(), oldObj.GetName()), req.UserInfo.Username).Add(1)
			util.LoggerProtectionInfo(util.ProtectionEventDeletionProtection, req.Kind.Kind, oldObj.GetNamespace(), oldObj.GetName(), req.UserInfo.Username)
			return admission.Errored(http.StatusForbidden, err)
		}
	}

	return admission.ValidationResponse(true, "")
}

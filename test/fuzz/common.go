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

package fuzz

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
	"strings"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	alphaNum     = "abcdefghijklmnopqrstuvwxyz0123456789"
	allowedChars = alphaNum + "-." // Allowed characters: letters, numbers, hyphens, dots
)

func GenerateSubsetReplicas(cf *fuzz.ConsumeFuzzer) (intstr.IntOrString, error) {
	isInt, err := cf.GetBool()
	if err != nil {
		return intstr.IntOrString{}, err
	}

	if isInt {
		intVal, err := cf.GetInt()
		if err != nil {
			return intstr.IntOrString{}, err
		}
		return intstr.FromInt32(int32(intVal)), nil
	}

	hasSuffix, err := cf.GetBool()
	if err != nil {
		return intstr.IntOrString{}, err
	}

	if hasSuffix {
		percent, err := cf.GetInt()
		if err != nil {
			return intstr.IntOrString{}, err
		}
		return intstr.FromString(fmt.Sprintf("%d%%", percent%1000)), nil
	}

	strVal, err := cf.GetString()
	if err != nil {
		return intstr.IntOrString{}, err
	}
	return intstr.FromString(strVal), nil
}

func GenerateNodeSelectorTerm(cf *fuzz.ConsumeFuzzer, term *corev1.NodeSelectorTerm) error {
	isStructured, err := cf.GetBool()
	if err != nil {
		return err
	}

	if !isStructured {
		if err := cf.GenerateStruct(term); err != nil {
			return err
		}
		return nil
	}

	if len(term.MatchExpressions) == 0 {
		term.MatchExpressions = make([]corev1.NodeSelectorRequirement, rand.Intn(2)+1)
	}
	if len(term.MatchFields) == 0 {
		term.MatchFields = make([]corev1.NodeSelectorRequirement, rand.Intn(2)+1)
	}
	validOperators := []corev1.NodeSelectorOperator{
		corev1.NodeSelectorOpIn,
		corev1.NodeSelectorOpNotIn,
		corev1.NodeSelectorOpExists,
		corev1.NodeSelectorOpDoesNotExist,
		corev1.NodeSelectorOpGt,
		corev1.NodeSelectorOpLt,
	}
	generateRequirements := func(requirements *[]corev1.NodeSelectorRequirement) error {
		for i := range *requirements {
			req := corev1.NodeSelectorRequirement{}
			req.Key = GenerateValidKey()
			choice, err := cf.GetInt()
			if err != nil {
				return err
			}
			req.Operator = validOperators[choice%len(validOperators)]
			if req.Operator == corev1.NodeSelectorOpIn || req.Operator == corev1.NodeSelectorOpNotIn {
				if len(req.Values) == 0 {
					req.Values = make([]string, rand.Intn(2)+1)
				}
				for i := range req.Values {
					req.Values[i] = GenerateValidValue()
				}
				sort.Strings(req.Values)
			} else {
				req.Values = nil
			}
			(*requirements)[i] = req
		}
		return nil
	}
	if term.MatchExpressions != nil {
		if err := generateRequirements(&term.MatchExpressions); err != nil {
			return err
		}
	}
	if term.MatchFields != nil {
		if err := generateRequirements(&term.MatchFields); err != nil {
			return err
		}
	}
	return nil
}

func GenerateLabelSelector(cf *fuzz.ConsumeFuzzer, selector *metav1.LabelSelector) error {
	isStructured, err := cf.GetBool()
	if err != nil {
		return err
	}

	if !isStructured {
		if err := cf.GenerateStruct(selector); err != nil {
			return err
		}
		return nil
	}

	if len(selector.MatchExpressions) == 0 {
		selector.MatchExpressions = make([]metav1.LabelSelectorRequirement, rand.Intn(2)+1)
	}
	if len(selector.MatchLabels) == 0 {
		selector.MatchLabels = make(map[string]string, rand.Intn(2)+1)
	}

	if selector.MatchLabels != nil {
		fuzzedMatchLabels := make(map[string]string, len(selector.MatchLabels))
		for i := 0; i < len(selector.MatchLabels); i++ {
			fuzzedMatchLabels[GenerateValidKey()] = GenerateValidValue()
		}
		selector.MatchLabels = fuzzedMatchLabels
	}

	validOperators := []metav1.LabelSelectorOperator{
		metav1.LabelSelectorOpIn,
		metav1.LabelSelectorOpNotIn,
		metav1.LabelSelectorOpExists,
		metav1.LabelSelectorOpDoesNotExist,
	}

	if selector.MatchExpressions != nil {
		for i := range selector.MatchExpressions {
			req := metav1.LabelSelectorRequirement{}
			req.Key = GenerateValidKey()
			choice, err := cf.GetInt()
			if err != nil {
				return err
			}
			req.Operator = validOperators[choice%len(validOperators)]
			if req.Operator == metav1.LabelSelectorOpIn || req.Operator == metav1.LabelSelectorOpNotIn {
				if len(req.Values) == 0 {
					req.Values = make([]string, rand.Intn(2)+1)
				}
				for i := range req.Values {
					req.Values[i] = GenerateValidValue()
				}
				sort.Strings(req.Values)
			} else {
				req.Values = nil
			}
			selector.MatchExpressions[i] = req
		}

		sort.Slice(selector.MatchExpressions, func(a, b int) bool { return selector.MatchExpressions[a].Key < selector.MatchExpressions[b].Key })
	}
	return nil
}

func GenerateTolerations(cf *fuzz.ConsumeFuzzer, toleration *corev1.Toleration) error {
	isStructured, err := cf.GetBool()
	if err != nil {
		return err
	}

	if !isStructured {
		if err := cf.GenerateStruct(toleration); err != nil {
			return err
		}
		return nil
	}

	validOperators := []corev1.TolerationOperator{
		corev1.TolerationOpExists,
		corev1.TolerationOpEqual,
	}

	toleration.Key = GenerateValidKey()
	choice, err := cf.GetInt()
	if err != nil {
		return err
	}
	toleration.Operator = validOperators[choice%len(validOperators)]
	// If the operator is Exists, the value should be empty, otherwise just a regular string.
	if toleration.Operator == corev1.TolerationOpEqual {
		toleration.Value = GenerateValidValue()
	} else {
		toleration.Value = ""
	}
	return nil
}

func GeneratePatch(cf *fuzz.ConsumeFuzzer, rawExtension *runtime.RawExtension) error {
	isStructured, err := cf.GetBool()
	if err != nil {
		return err
	}

	if !isStructured {
		raw, err := cf.GetBytes()
		if err != nil {
			return err
		}
		rawExtension.Raw = raw
		return nil
	}

	choice, err := cf.GetInt()
	if err != nil {
		return err
	}
	switch choice % 2 {
	case 0: // patch metadata labels and annotations
		fuzzedLabels := make(map[string]string)
		for i := 0; i < rand.Intn(2)+1; i++ {
			fuzzedLabels[GenerateValidKey()] = GenerateValidValue()
		}

		fuzzedAnnotations := make(map[string]string)
		for i := 0; i < rand.Intn(2)+1; i++ {
			fuzzedAnnotations[GenerateValidKey()] = GenerateValidValue()
		}

		patch := map[string]interface{}{
			"metadata": map[string]interface {
			}{
				"labels":      fuzzedLabels,
				"annotations": fuzzedAnnotations,
			},
		}

		raw, err := json.Marshal(patch)
		if err != nil {
			return err
		}
		rawExtension.Raw = raw
		return nil
	case 1: // patch container resources, env
		randomQuantity := func() resource.Quantity {
			var q resource.Quantity
			if err := cf.GenerateStruct(&q); err != nil {
				return q
			}
			return q
		}
		cpuLimit := randomQuantity()
		memoryLimit := randomQuantity()

		patch := map[string]interface{}{
			"spec": map[string]interface {
			}{
				"containers": []map[string]interface{}{
					{
						"name": GenerateValidKey(),
						"resources": map[string]interface{}{
							"requests": map[string]interface{}{
								"cpu":    cpuLimit.String(),
								"memory": memoryLimit.String(),
							},
							"limits": map[string]interface{}{
								"cpu":    cpuLimit.String(),
								"memory": memoryLimit.String(),
							},
						},
						"env": []map[string]interface{}{
							{
								"name":  GenerateValidKey(),
								"value": GenerateValidValue(),
							},
						},
					},
				},
			},
		}

		raw, err := json.Marshal(patch)
		if err != nil {
			return err
		}
		rawExtension.Raw = raw
		return nil
	}
	return nil
}

func GenerateLabelPart() string {
	length := rand.Intn(63) + 1
	b := make([]byte, length)

	// Make sure the first and last characters are letters or numbers
	b[0] = alphaNum[rand.Intn(len(alphaNum))]
	b[length-1] = alphaNum[rand.Intn(len(alphaNum))]

	// The middle character can be any character allowed
	for i := 1; i < length-1; i++ {
		b[i] = allowedChars[rand.Intn(len(allowedChars))]
	}
	return string(b)
}

func GenerateValidKey() string {
	partsCount := rand.Intn(3) + 1
	parts := make([]string, partsCount)

	for i := range parts {
		parts[i] = GenerateLabelPart()
	}

	return strings.Join(parts, ".")
}

func GenerateValidValue() string {
	// 5% probability of generating a null value
	if rand.Intn(100) < 5 {
		return ""
	}
	partsCount := rand.Intn(3) + 1
	parts := make([]string, partsCount)

	for i := range parts {
		parts[i] = GenerateLabelPart()
	}
	return strings.Join(parts, ".")
}

/*
Copyright 2025 The Kruise Authors.

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
	fuzz "github.com/AdaLogics/go-fuzz-headers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"math/rand"
	"sort"
	"strings"
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

func GenerateUnstructured(cf *fuzz.ConsumeFuzzer, resource *unstructured.Unstructured) error {
	isStructured, err := cf.GetBool()
	if err != nil {
		return err
	}

	if !isStructured {
		obj := make(map[string]interface{})
		if err = cf.GenerateStruct(&obj); err != nil {
			return err
		}
		resource.Object = obj
		return nil
	}

	labels := make(map[string]string)
	if err := cf.FuzzMap(&labels); err != nil {
		return err
	}
	resource.SetLabels(labels)

	annotations := make(map[string]string)
	if err := cf.FuzzMap(&annotations); err != nil {
		return err
	}
	resource.SetAnnotations(annotations)

	finalizers := make([]string, 0)
	if err := cf.CreateSlice(&finalizers); err != nil {
		return err
	}
	resource.SetFinalizers(finalizers)
	return nil
}

func GenerateValidKey() string {
	return randomLabelKey()
}

func GenerateValidValue() string {
	return randomLabelPart(true)
}

func GenerateValidNamespaceName() string {
	return randomRFC1123()
}

func GenerateInvalidNamespaceName() string {
	invalidChars := []rune{'$', '_', ' '}
	validName := GenerateValidNamespaceName()
	runes := []rune(validName)
	// Make sure the first character is illegal
	runes[0] = invalidChars[rand.Intn(len(invalidChars))]
	return string(runes)
}

type charRange struct {
	first, last rune
}

func (c *charRange) choose() rune {
	count := int64(c.last - c.first + 1)
	ch := c.first + rune(rand.Int63n(count))

	return ch
}

func randomLabelKey() string {
	namePart := randomLabelPart(false)
	prefixPart := ""

	if rand.Intn(10) < 5 {
		prefixPartsLen := rand.Intn(2) + 1
		prefixParts := make([]string, prefixPartsLen)
		for i := range prefixParts {
			prefixParts[i] = randomRFC1123()
		}
		prefixPart = strings.Join(prefixParts, ".") + "/"
	}

	return prefixPart + namePart
}

func randomLabelPart(canBeEmpty bool) string {
	validStartEnd := []charRange{{'0', '9'}, {'a', 'z'}, {'A', 'Z'}}
	validMiddle := []charRange{{'0', '9'}, {'a', 'z'}, {'A', 'Z'},
		{'.', '.'}, {'-', '-'}, {'_', '_'}}

	partLen := rand.Intn(64) // len is [0, 63]
	if !canBeEmpty {
		partLen = rand.Intn(63) + 1 // len is [1, 63]
	}

	runes := make([]rune, partLen)
	if partLen == 0 {
		return string(runes)
	}

	runes[0] = validStartEnd[rand.Intn(len(validStartEnd))].choose()
	for i := range runes[1:] {
		runes[i+1] = validMiddle[rand.Intn(len(validMiddle))].choose()
	}
	runes[len(runes)-1] = validStartEnd[rand.Intn(len(validStartEnd))].choose()

	return string(runes)
}

func randomRFC1123() string {
	validStartEnd := []charRange{{'0', '9'}, {'a', 'z'}}
	validMiddle := []charRange{{'0', '9'}, {'a', 'z'}, {'-', '-'}}

	partLen := rand.Intn(63) + 1 // len is [1, 63]
	runes := make([]rune, partLen)

	runes[0] = validStartEnd[rand.Intn(len(validStartEnd))].choose()
	for i := range runes[1:] {
		runes[i+1] = validMiddle[rand.Intn(len(validMiddle))].choose()
	}
	runes[len(runes)-1] = validStartEnd[rand.Intn(len(validStartEnd))].choose()

	return string(runes)
}

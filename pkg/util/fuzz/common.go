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
	"fmt"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
	"k8s.io/apimachinery/pkg/util/intstr"
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

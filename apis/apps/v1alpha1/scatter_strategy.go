/*
Copyright 2020 The Kruise Authors.

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

package v1alpha1

import "fmt"

// UpdateScatterStrategy defines a map for label key-value. Pods matches the key-value will be scattered when update.
//
// Example1: [{"Key": "labelA", "Value": "AAA"}]
// It means all pods with label labelA=AAA will be scattered when update.
//
// Example2: [{"Key": "labelA", "Value": "AAA"}, {"Key": "labelB", "Value": "BBB"}]
// Controller will calculate the two sums of pods with labelA=AAA and with labelB=BBB,
// pods with the label that has bigger amount will be scattered first, then pods with the other label will be scattered.
type UpdateScatterStrategy []UpdateScatterTerm

type UpdateScatterTerm struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// FieldsValidation checks invalid fields in UpdateScatterStrategy.
func (strategy UpdateScatterStrategy) FieldsValidation() error {
	if len(strategy) == 0 {
		return nil
	}

	m := make(map[string]struct{}, len(strategy))
	for _, term := range strategy {
		if term.Key == "" {
			return fmt.Errorf("key should not be empty")
		}
		id := term.Key + ":" + term.Value
		if _, ok := m[id]; !ok {
			m[id] = struct{}{}
		} else {
			return fmt.Errorf("duplicated key=%v value=%v", term.Key, term.Value)
		}
	}

	return nil
}

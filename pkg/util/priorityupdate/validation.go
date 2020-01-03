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

package priorityupdate

import (
	"fmt"

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ValidatePriorityUpdateStrategy checks if the given UpdatePriorityStrategy is valid
func ValidatePriorityUpdateStrategy(strategy *appsv1alpha1.UpdatePriorityStrategy) error {
	if strategy == nil {
		return nil
	}

	if len(strategy.WeightPriority) > 0 && len(strategy.OrderPriority) > 0 {
		return fmt.Errorf("only one of weightPriority and orderPriority can be used")
	}

	for _, w := range strategy.WeightPriority {
		if w.Weight < 0 || w.Weight > 100 {
			return fmt.Errorf("weight must be valid number in the range 1-100")
		}
		if w.MatchSelector.Size() == 0 {
			return fmt.Errorf("selector can not be empty")
		}
		if _, err := metav1.LabelSelectorAsSelector(&w.MatchSelector); err != nil {
			return fmt.Errorf("invalid selector %v", err)
		}
	}

	for _, o := range strategy.OrderPriority {
		if len(o.OrderedKey) == 0 {
			return fmt.Errorf("order key can not be empty")
		}
	}

	return nil
}

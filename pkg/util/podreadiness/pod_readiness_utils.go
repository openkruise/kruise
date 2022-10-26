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

package podreadiness

import (
	"encoding/json"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/podadapter"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
)

func addNotReadyKey(adp podadapter.Adapter, pod *v1.Pod, msg Message, condType v1.PodConditionType) error {
	if alreadyHasKey(pod, msg, condType) {
		return nil
	}

	if !containsReadinessGate(pod, condType) {
		return nil
	}

	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		newPod, err := adp.GetPod(pod.Namespace, pod.Name)
		if err != nil {
			return err
		}

		condition := getReadinessCondition(newPod, condType)
		if condition == nil {
			_, messages := addMessage("", msg)
			newPod.Status.Conditions = append(newPod.Status.Conditions, v1.PodCondition{
				Type:               condType,
				Message:            messages.dump(),
				LastTransitionTime: metav1.Now(),
			})
		} else {
			changed, messages := addMessage(condition.Message, msg)
			if !changed {
				return nil
			}
			condition.Status = v1.ConditionFalse
			condition.Message = messages.dump()
			condition.LastTransitionTime = metav1.Now()
		}

		// set pod ready condition to "False"
		util.SetPodReadyCondition(newPod)
		return adp.UpdatePodStatus(newPod)
	})
	return err
}

func removeNotReadyKey(adp podadapter.Adapter, pod *v1.Pod, msg Message, condType v1.PodConditionType) error {
	if !containsReadinessGate(pod, condType) {
		return nil
	}

	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		newPod, err := adp.GetPod(pod.Namespace, pod.Name)
		if err != nil {
			return err
		}

		condition := getReadinessCondition(newPod, condType)
		if condition == nil {
			return nil
		}
		changed, messages := removeMessage(condition.Message, msg)
		if !changed {
			return nil
		}
		if len(messages) == 0 {
			condition.Status = v1.ConditionTrue
		}
		condition.Message = messages.dump()
		condition.LastTransitionTime = metav1.Now()
		return adp.UpdatePodStatus(newPod)
	})
}

func addMessage(base string, msg Message) (bool, messageList) {
	messages := messageList{}
	if base != "" {
		_ = json.Unmarshal([]byte(base), &messages)
	}
	for _, m := range messages {
		if m.UserAgent == msg.UserAgent && m.Key == msg.Key {
			return false, messages
		}
	}
	messages = append(messages, msg)
	return true, messages
}

func removeMessage(base string, msg Message) (bool, messageList) {
	messages := messageList{}
	if base != "" {
		_ = json.Unmarshal([]byte(base), &messages)
	}
	var removed bool
	newMessages := messageList{}
	for _, m := range messages {
		if m.UserAgent == msg.UserAgent && m.Key == msg.Key {
			removed = true
			continue
		}
		newMessages = append(newMessages, m)
	}
	return removed, newMessages
}

func GetReadinessCondition(pod *v1.Pod) *v1.PodCondition {
	return getReadinessCondition(pod, appspub.KruisePodReadyConditionType)
}

func ContainsReadinessGate(pod *v1.Pod) bool {
	return containsReadinessGate(pod, appspub.KruisePodReadyConditionType)
}

func getReadinessCondition(pod *v1.Pod, condType v1.PodConditionType) *v1.PodCondition {
	if pod == nil {
		return nil
	}
	for i := range pod.Status.Conditions {
		c := &pod.Status.Conditions[i]
		if c.Type == condType {
			return c
		}
	}
	return nil
}

func containsReadinessGate(pod *v1.Pod, condType v1.PodConditionType) bool {
	for _, g := range pod.Spec.ReadinessGates {
		if g.ConditionType == condType {
			return true
		}
	}
	return false
}

func alreadyHasKey(pod *v1.Pod, msg Message, condType v1.PodConditionType) bool {
	condition := getReadinessCondition(pod, condType)
	if condition == nil {
		return false
	}
	if condition.Status == v1.ConditionTrue || condition.Message == "" {
		return false
	}
	messages := messageList{}
	_ = json.Unmarshal([]byte(condition.Message), &messages)
	for _, m := range messages {
		if m.UserAgent == msg.UserAgent && m.Key == msg.Key {
			return true
		}
	}
	return false
}

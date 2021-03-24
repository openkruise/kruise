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
	"context"
	"encoding/json"
	"sort"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	"github.com/openkruise/kruise/pkg/util"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func AddNotReadyKey(c client.Client, pod *v1.Pod, msg Message) error {
	if alreadyHasKey(pod, msg) {
		return nil
	}
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		newPod := &v1.Pod{}
		if err := c.Get(context.TODO(), types.NamespacedName{Namespace: pod.Namespace, Name: pod.Name}, newPod); err != nil {
			return err
		}
		if !ContainsReadinessGate(pod) {
			return nil
		}

		condition := GetReadinessCondition(newPod)
		if condition == nil {
			_, messages := addMessage("", msg)
			newPod.Status.Conditions = append(newPod.Status.Conditions, v1.PodCondition{
				Type:               appspub.KruisePodReadyConditionType,
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

		return c.Status().Update(context.TODO(), newPod)
	})
}

func RemoveNotReadyKey(c client.Client, pod *v1.Pod, msg Message) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		newPod := &v1.Pod{}
		if err := c.Get(context.TODO(), types.NamespacedName{Namespace: pod.Namespace, Name: pod.Name}, newPod); err != nil {
			return err
		}
		if !ContainsReadinessGate(pod) {
			return nil
		}

		condition := GetReadinessCondition(newPod)
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

		return c.Status().Update(context.TODO(), newPod)
	})
}

type Message struct {
	UserAgent string `json:"userAgent"`
	Key       string `json:"key"`
}

type messageList []Message

func (c messageList) Len() int      { return len(c) }
func (c messageList) Swap(i, j int) { c[i], c[j] = c[j], c[i] }
func (c messageList) Less(i, j int) bool {
	if c[i].UserAgent == c[j].UserAgent {
		return c[i].Key < c[j].Key
	}
	return c[i].UserAgent < c[j].UserAgent
}

func (c messageList) dump() string {
	sort.Sort(c)
	return util.DumpJSON(c)
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
	if pod == nil {
		return nil
	}
	for i := range pod.Status.Conditions {
		c := &pod.Status.Conditions[i]
		if c.Type == appspub.KruisePodReadyConditionType {
			return c
		}
	}
	return nil
}

func ContainsReadinessGate(pod *v1.Pod) bool {
	for _, g := range pod.Spec.ReadinessGates {
		if g.ConditionType == appspub.KruisePodReadyConditionType {
			return true
		}
	}
	return false
}

func alreadyHasKey(pod *v1.Pod, msg Message) bool {
	condition := GetReadinessCondition(pod)
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

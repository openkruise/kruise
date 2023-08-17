package util

import (
	"encoding/json"

	v1 "k8s.io/api/core/v1"
)

// using pod condition message to get key-value pairs
func GetMessageKvFromCondition(condition *v1.PodCondition) (map[string]interface{}, error) {
	messageKv := make(map[string]interface{})
	if condition != nil && condition.Message != "" {
		if err := json.Unmarshal([]byte(condition.Message), &messageKv); err != nil {
			return nil, err
		}
	}
	return messageKv, nil
}

// using pod condition message to save key-value pairs
func UpdateMessageKvCondition(kv map[string]interface{}, condition *v1.PodCondition) {
	message, _ := json.Marshal(kv)
	condition.Message = string(message)
}

package livenessprobe

import (
	v1 "k8s.io/api/core/v1"
)

type ContainerLivenessProbe struct {
	Name          string   `json:"name"`
	LivenessProbe v1.Probe `json:"livenessProbe"`
}

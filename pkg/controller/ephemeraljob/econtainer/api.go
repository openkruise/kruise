package econtainer

import (
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	v1 "k8s.io/api/core/v1"
)

type EphemeralContainerInterface interface {
	// GetEphemeralContainers will return all ephemeral container status.
	// Maybe they are not created by current ephemeral jobs.
	GetEphemeralContainersStatus(target *v1.Pod) []v1.ContainerStatus
	// GetEphemeralContainers return all ephemeral containers which have been created in target pods.
	// Maybe they are not created by current ephemeral jobs.
	GetEphemeralContainers(target *v1.Pod) []v1.EphemeralContainer

	UpdateEphemeralContainer(target *v1.Pod) error
	CreateEphemeralContainer(target *v1.Pod) error
	RemoveEphemeralContainer(target *v1.Pod) error
}

func New(job *appsv1beta1.EphemeralJob) EphemeralContainerInterface {
	return &k8sControl{job}
}

func getEphemeralContainersMaps(containers []v1.EphemeralContainer) (map[string]v1.EphemeralContainer, bool) {
	res := make(map[string]v1.EphemeralContainer)
	for _, c := range containers {
		res[c.Name] = c
	}
	return res, len(res) == 0
}

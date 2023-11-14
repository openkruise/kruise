package econtainer

import (
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	v1 "k8s.io/api/core/v1"
)

type EphemeralContainerInterface interface {
	// GetEphemeralContainersStatus will return all ephemeral container status.
	// Maybe they are not created by current ephemeral jobs.
	GetEphemeralContainersStatus(target *v1.Pod) []v1.ContainerStatus
	// GetEphemeralContainers return all ephemeral containers which have been created in target pods.
	// Maybe they are not created by current ephemeral jobs.
	GetEphemeralContainers(target *v1.Pod) []v1.EphemeralContainer
	// ContainsEphemeralContainer return if target pod contains(1st return value) and owns(2nd return value)
	// the ephemeral containers having the same name with the ones ephemeralJob want to inject.
	// Owning an ephemeral containers to a ephemeralJob means KRUISE_EJOB_ID env of the ephemeral container
	// equals to this ephemeralJob's uid.
	ContainsEphemeralContainer(target *v1.Pod) (bool, bool)

	UpdateEphemeralContainer(target *v1.Pod) error
	CreateEphemeralContainer(target *v1.Pod) error
	RemoveEphemeralContainer(target *v1.Pod) (*time.Duration, error)
}

func New(job *appsv1alpha1.EphemeralJob) EphemeralContainerInterface {
	return &k8sControl{job}
}

func getEphemeralContainersMaps(containers []v1.EphemeralContainer) (map[string]v1.EphemeralContainer, bool) {
	res := make(map[string]v1.EphemeralContainer)
	for _, c := range containers {
		res[c.Name] = c
	}
	return res, len(res) == 0
}

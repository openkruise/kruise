package scale

import (
	"sort"

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
	v1 "k8s.io/api/core/v1"
	intstrutil "k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"k8s.io/utils/integer"
)

func getPodsToDelete(cs *appsv1alpha1.CloneSet, pods []*v1.Pod) []*v1.Pod {
	var podsToDelete []*v1.Pod
	s := sets.NewString(cs.Spec.ScaleStrategy.PodsToDelete...)
	for _, p := range pods {
		if s.Has(p.Name) {
			podsToDelete = append(podsToDelete, p)
		}
	}
	return podsToDelete
}

// Get available IDs, if the a PVC exists but the corresponding pod does not exist, then reusing the ID, i.e., reuse the pvc.
// If there is not enough existing available IDs, then generate ID using rand utility.
// More details: if template changes more than container image, controller will delete pod during update, and
// it will keep the pvc to reuse.
func getOrGenAvailableIDs(num int, pods []*v1.Pod, pvcs []*v1.PersistentVolumeClaim) sets.String {
	existingIDs := sets.NewString()
	availableIDs := sets.NewString()
	for _, pvc := range pvcs {
		if id := pvc.Labels[appsv1alpha1.CloneSetInstanceID]; len(id) > 0 {
			existingIDs.Insert(id)
			availableIDs.Insert(id)
		}
	}

	for _, pod := range pods {
		if id := pod.Labels[appsv1alpha1.CloneSetInstanceID]; len(id) > 0 {
			existingIDs.Insert(id)
			availableIDs.Delete(id)
		}
	}

	retIDs := sets.NewString()
	for i := 0; i < num; i++ {
		id := getOrGenInstanceID(existingIDs, availableIDs)
		retIDs.Insert(id)
	}

	return retIDs
}

func getOrGenInstanceID(existingIDs, availableIDs sets.String) string {
	id, _ := availableIDs.PopAny()
	if len(id) == 0 {
		for {
			id = rand.String(LengthOfInstanceID)
			if !existingIDs.Has(id) {
				break
			}
		}
	}
	return id
}

func calculateDiffs(cs *appsv1alpha1.CloneSet, revConsistent bool, totalPods int, notUpdatedPods int) (totalDiff int, currentRevDiff int) {
	var maxSurge int

	if !revConsistent {
		if cs.Spec.UpdateStrategy.Partition != nil {
			currentRevDiff = notUpdatedPods - int(*cs.Spec.UpdateStrategy.Partition)
		}

		// Use maxSurge only if partition has not satisfied
		if currentRevDiff > 0 {
			if cs.Spec.UpdateStrategy.MaxSurge != nil {
				maxSurge, _ = intstrutil.GetValueFromIntOrPercent(cs.Spec.UpdateStrategy.MaxSurge, int(*cs.Spec.Replicas), true)
				maxSurge = integer.IntMin(maxSurge, currentRevDiff)
			}
		}
	}
	totalDiff = totalPods - int(*cs.Spec.Replicas) - maxSurge

	if totalDiff != 0 && maxSurge > 0 {
		klog.V(3).Infof("CloneSet scale diff(%d),currentRevDiff(%d) with maxSurge %d", totalDiff, currentRevDiff, maxSurge)
	}
	return
}

func choosePodsToDelete(totalDiff int, currentRevDiff int, notUpdatedPods, updatedPods []*v1.Pod) []*v1.Pod {
	choose := func(pods []*v1.Pod, diff int) []*v1.Pod {
		// No need to sort pods if we are about to delete all of them.
		// diff will always be <= len(filteredPods), so not need to handle > case.
		if diff < len(pods) {
			// Sort the pods in the order such that not-ready < ready, unscheduled
			// < scheduled, and pending < running. This ensures that we delete pods
			// in the earlier stages whenever possible.
			sort.Sort(kubecontroller.ActivePods(pods))
		}
		return pods[:diff]
	}

	var podsToDelete []*v1.Pod
	if currentRevDiff >= totalDiff {
		podsToDelete = choose(notUpdatedPods, totalDiff)
	} else if currentRevDiff > 0 {
		podsToDelete = choose(notUpdatedPods, currentRevDiff)
		podsToDelete = append(podsToDelete, choose(updatedPods, totalDiff-currentRevDiff)...)
	} else {
		podsToDelete = choose(updatedPods, totalDiff)
	}

	return podsToDelete
}

package scale

import (
	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/sets"
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

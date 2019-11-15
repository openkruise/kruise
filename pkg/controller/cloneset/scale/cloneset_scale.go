package scale

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"k8s.io/klog"

	"k8s.io/apimachinery/pkg/util/sets"

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
	clonesetutils "github.com/openkruise/kruise/pkg/controller/cloneset/utils"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/expectations"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// LengthOfInstanceID is the length of instance-id
	LengthOfInstanceID = 5

	// When batching pod creates, initialBatchSize is the size of the initial batch.
	initialBatchSize = 1
)

// Interface for managing replicas including create and delete pod/pvc.
type Interface interface {
	ManageReplicas(
		currentCS, updateCS *appsv1alpha1.CloneSet,
		currentRevision, updateRevision string,
		pods []*v1.Pod, pvcs []*v1.PersistentVolumeClaim,
	) (bool, error)
}

// NewScaleControl returns a scale control.
func NewScaleControl(c client.Client, recorder record.EventRecorder, exp expectations.ScaleExpectations) Interface {
	return &realControl{Client: c, recorder: recorder, exp: exp}
}

type realControl struct {
	client.Client
	recorder record.EventRecorder
	exp      expectations.ScaleExpectations
}

func (r *realControl) ManageReplicas(
	currentCS, updateCS *appsv1alpha1.CloneSet,
	currentRevision, updateRevision string,
	pods []*v1.Pod, pvcs []*v1.PersistentVolumeClaim,
) (bool, error) {
	if updateCS.Spec.Replicas == nil {
		return false, fmt.Errorf("spec.Replicas is nil")
	}

	if podsToDelete := getPodsToDelete(updateCS, pods); len(podsToDelete) > 0 {
		klog.V(3).Infof("Begin to delete pods in podsToDelete: %v", podsToDelete)
		return true, r.deletePods(updateCS, podsToDelete, pvcs)
	}

	var err error
	diff := len(pods) - int(*updateCS.Spec.Replicas)

	if diff < 0 {
		// total number of this creation
		expectedCreations := diff * -1
		// lack number of current version
		expectedCurrentCreations := 0
		if updateCS.Spec.UpdateStrategy.Partition != nil {
			notUpdatedNum := len(pods) - len(clonesetutils.GetUpdatedPods(pods, updateRevision))
			expectedCurrentCreations = int(*updateCS.Spec.UpdateStrategy.Partition) - notUpdatedNum
		}

		klog.V(3).Infof("Begin to scale out %d pods", expectedCreations)

		// available instance-id come from free pvc
		availableIDs := getOrGenAvailableIDs(expectedCreations, pods, pvcs)
		// existing pvc names
		existingPVCNames := sets.NewString()
		for _, pvc := range pvcs {
			existingPVCNames.Insert(pvc.Name)
		}

		err = r.createPods(expectedCreations, expectedCurrentCreations,
			currentCS, updateCS, currentRevision, updateRevision, availableIDs.List(), existingPVCNames)
		return true, err

	} else if diff > 0 {

		klog.V(3).Infof("Begin to scale in %d pods", diff)

		podsToDelete := r.choosePodsToDelete(pods, diff)
		err = r.deletePods(updateCS, podsToDelete, pvcs)

		return true, err
	}

	return false, nil
}

func (r *realControl) createPods(
	expectedCreations, expectedCurrentCreations int,
	currentCS, updateCS *appsv1alpha1.CloneSet,
	currentRevision, updateRevision string,
	availableIDs []string, existingPVCNames sets.String,
) error {
	// list of all pods need to create
	var newPods []*v1.Pod
	if expectedCreations <= expectedCurrentCreations {
		newPods = newVersionedPods(currentCS, currentRevision, expectedCreations, &availableIDs)
	} else {
		newPods = newVersionedPods(currentCS, currentRevision, expectedCurrentCreations, &availableIDs)
		newPods = append(newPods, newVersionedPods(updateCS, updateRevision, expectedCreations-expectedCurrentCreations, &availableIDs)...)
	}

	podsCreationChan := make(chan *v1.Pod, len(newPods))
	for _, p := range newPods {
		r.exp.ExpectScale(clonesetutils.GetControllerKey(updateCS), expectations.Create, p.Name)
		podsCreationChan <- p
	}

	successPodNames := sync.Map{}
	_, err := clonesetutils.DoItSlowly(len(newPods), initialBatchSize, func() error {
		pod := <-podsCreationChan

		cs := updateCS
		if pod.Labels[apps.ControllerRevisionHashLabelKey] == currentRevision {
			cs = currentCS
		}

		var createErr error
		if createErr = r.createOnePod(cs, pod, existingPVCNames); createErr != nil {
			return createErr
		}
		successPodNames.Store(pod.Name, struct{}{})
		return nil
	})

	// rollback to ignore failure pods because the informer won't observe these pods
	for _, pod := range newPods {
		if _, ok := successPodNames.Load(pod.Name); !ok {
			r.exp.ObserveScale(clonesetutils.GetControllerKey(updateCS), expectations.Create, pod.Name)
		}
	}

	return err
}

func (r *realControl) createOnePod(cs *appsv1alpha1.CloneSet, pod *v1.Pod, existingPVCNames sets.String) error {
	claims := clonesetutils.GetPersistentVolumeClaims(cs, pod)
	for _, c := range claims {
		if existingPVCNames.Has(c.Name) {
			continue
		}
		r.exp.ExpectScale(clonesetutils.GetControllerKey(cs), expectations.Create, c.Name)
		if err := r.Create(context.TODO(), &c); err != nil {
			r.exp.ObserveScale(clonesetutils.GetControllerKey(cs), expectations.Create, c.Name)
			r.recorder.Event(cs, v1.EventTypeWarning, "FailedCreate", fmt.Sprintf("failed to create pvc %s: %v", util.DumpJSON(c), err))
			return err
		}
	}

	if err := r.Create(context.TODO(), pod); err != nil {
		r.recorder.Event(cs, v1.EventTypeWarning, "FailedCreate", fmt.Sprintf("failed to create pod %s: %v", util.DumpJSON(pod), err))
		return err
	}

	r.recorder.Event(cs, v1.EventTypeNormal, "SuccessfulCreate", fmt.Sprintf("succeed to create pod %s", pod.Name))
	return nil
}

func (r *realControl) deletePods(cs *appsv1alpha1.CloneSet, podsToDelete []*v1.Pod, pvcs []*v1.PersistentVolumeClaim) error {
	for _, pod := range podsToDelete {
		r.exp.ExpectScale(clonesetutils.GetControllerKey(cs), expectations.Delete, pod.Name)
		if err := r.Delete(context.TODO(), pod); err != nil {
			r.exp.ObserveScale(clonesetutils.GetControllerKey(cs), expectations.Delete, pod.Name)
			r.recorder.Event(cs, v1.EventTypeWarning, "FailedDelete", fmt.Sprintf("failed to delete pod %s: %v", pod.Name, err))
			return err
		}
		r.recorder.Event(cs, v1.EventTypeNormal, "SuccessfulDelete", fmt.Sprintf("succeed to delete pod %s", pod.Name))

		// delete pvcs which have the same instance-id
		for _, pvc := range pvcs {
			if pvc.Labels[appsv1alpha1.CloneSetInstanceID] != pod.Labels[appsv1alpha1.CloneSetInstanceID] {
				continue
			}

			r.exp.ExpectScale(clonesetutils.GetControllerKey(cs), expectations.Delete, pvc.Name)
			if err := r.Delete(context.TODO(), pvc); err != nil {
				r.exp.ObserveScale(clonesetutils.GetControllerKey(cs), expectations.Delete, pvc.Name)
				r.recorder.Event(cs, v1.EventTypeWarning, "FailedDelete", fmt.Sprintf("failed to delete pvc %s: %v", pvc.Name, err))
				return err
			}
		}
	}

	return nil
}

func (r *realControl) choosePodsToDelete(pods []*v1.Pod, diff int) []*v1.Pod {
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

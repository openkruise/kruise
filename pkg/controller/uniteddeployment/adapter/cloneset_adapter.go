package adapter

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/klog/v2"

	alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/refmanager"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilpointer "k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type CloneSetAdapter struct {
	client.Client
	Scheme *runtime.Scheme
}

func (a *CloneSetAdapter) NewResourceObject() client.Object {
	return &alpha1.CloneSet{}
}

func (a *CloneSetAdapter) NewResourceListObject() client.ObjectList {
	return &alpha1.CloneSetList{}
}

func (a *CloneSetAdapter) GetObjectMeta(obj metav1.Object) *metav1.ObjectMeta {
	return &obj.(*alpha1.CloneSet).ObjectMeta
}

func (a *CloneSetAdapter) GetStatusObservedGeneration(obj metav1.Object) int64 {
	return obj.(*alpha1.CloneSet).Status.ObservedGeneration
}

func (a *CloneSetAdapter) GetReplicaDetails(obj metav1.Object, updatedRevision string) (specReplicas, specPartition *int32, statusReplicas, statusReadyReplicas, statusUpdatedReplicas, statusUpdatedReadyReplicas int32, err error) {

	set := obj.(*alpha1.CloneSet)

	var pods []*corev1.Pod

	pods, err = a.getCloneSetPods(set)

	if err != nil {
		return
	}

	specReplicas = set.Spec.Replicas

	if set.Spec.UpdateStrategy.Partition != nil {
		partition, _ := intstr.GetValueFromIntOrPercent(set.Spec.UpdateStrategy.Partition, int(*set.Spec.Replicas), true)
		specPartition = utilpointer.Int32Ptr(int32(partition))
	}

	statusReplicas = set.Status.Replicas
	statusReadyReplicas = set.Status.ReadyReplicas
	statusUpdatedReplicas, statusUpdatedReadyReplicas = calculateUpdatedReplicas(pods, updatedRevision)

	return
}

func (a *CloneSetAdapter) GetSubsetFailure() *string {
	return nil
}

func (a *CloneSetAdapter) ApplySubsetTemplate(ud *alpha1.UnitedDeployment, subsetName, revision string, replicas, partition int32, obj runtime.Object) error {

	set := obj.(*alpha1.CloneSet)

	var subSetConfig *alpha1.Subset

	for _, subset := range ud.Spec.Topology.Subsets {
		if subset.Name == subsetName {
			subSetConfig = &subset
			break
		}
	}
	if subSetConfig == nil {
		return fmt.Errorf("fail to find subset config %s", subsetName)
	}

	set.Namespace = ud.Namespace

	if set.Labels == nil {
		set.Labels = map[string]string{}
	}
	for k, v := range ud.Spec.Template.CloneSetTemplate.Labels {
		set.Labels[k] = v
	}
	for k, v := range ud.Spec.Selector.MatchLabels {
		set.Labels[k] = v
	}
	set.Labels[alpha1.ControllerRevisionHashLabelKey] = revision
	// record the subset name as a label
	set.Labels[alpha1.SubSetNameLabelKey] = subsetName

	if set.Annotations == nil {
		set.Annotations = map[string]string{}
	}
	for k, v := range ud.Spec.Template.CloneSetTemplate.Annotations {
		set.Annotations[k] = v
	}

	set.GenerateName = getSubsetPrefix(ud.Name, subsetName)

	selectors := ud.Spec.Selector.DeepCopy()
	selectors.MatchLabels[alpha1.SubSetNameLabelKey] = subsetName

	if err := controllerutil.SetControllerReference(ud, set, a.Scheme); err != nil {
		return err
	}

	set.Spec.Selector = selectors
	set.Spec.Replicas = &replicas

	set.Spec.UpdateStrategy = ud.Spec.Template.CloneSetTemplate.Spec.UpdateStrategy

	set.Spec.UpdateStrategy.Partition = util.GetIntOrStrPointer(intstr.FromInt(int(partition)))

	set.Spec.Template = *ud.Spec.Template.CloneSetTemplate.Spec.Template.DeepCopy()

	if set.Spec.Template.Labels == nil {
		set.Spec.Template.Labels = map[string]string{}
	}

	set.Spec.Template.Labels[alpha1.SubSetNameLabelKey] = subsetName
	set.Spec.Template.Labels[alpha1.ControllerRevisionHashLabelKey] = revision
	set.Spec.RevisionHistoryLimit = ud.Spec.Template.CloneSetTemplate.Spec.RevisionHistoryLimit
	set.Spec.VolumeClaimTemplates = ud.Spec.Template.CloneSetTemplate.Spec.VolumeClaimTemplates

	attachNodeAffinity(&set.Spec.Template.Spec, subSetConfig)
	attachTolerations(&set.Spec.Template.Spec, subSetConfig)
	if subSetConfig.Patch.Raw != nil {
		TemplateSpecBytes, _ := json.Marshal(set.Spec.Template)
		modified, err := strategicpatch.StrategicMergePatch(TemplateSpecBytes, subSetConfig.Patch.Raw, &corev1.PodTemplateSpec{})
		if err != nil {
			klog.Errorf("failed to merge patch raw %s", subSetConfig.Patch.Raw)
			return err
		}
		patchedTemplateSpec := corev1.PodTemplateSpec{}
		if err = json.Unmarshal(modified, &patchedTemplateSpec); err != nil {
			klog.Errorf("failed to unmarshal %s to podTemplateSpec", modified)
			return err
		}

		set.Spec.Template = patchedTemplateSpec
		klog.V(2).Infof("CloneSet [%s/%s] was patched successfully: %s", set.Namespace, set.GenerateName, subSetConfig.Patch.Raw)
	}
	if set.Annotations == nil {
		set.Annotations = make(map[string]string)
	}
	set.Annotations[alpha1.AnnotationSubsetPatchKey] = string(subSetConfig.Patch.Raw)
	return nil
}

func (a *CloneSetAdapter) PostUpdate(ud *alpha1.UnitedDeployment, obj runtime.Object, revision string, partition int32) error {
	return nil
}

func (a *CloneSetAdapter) IsExpected(obj metav1.Object, revision string) bool {
	return obj.GetLabels()[alpha1.ControllerRevisionHashLabelKey] != revision
}

func (a *CloneSetAdapter) getCloneSetPods(set *alpha1.CloneSet) ([]*corev1.Pod, error) {

	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)

	if err != nil {
		return nil, err
	}

	podList := &corev1.PodList{}

	err = a.Client.List(context.TODO(), podList, &client.ListOptions{LabelSelector: selector})

	if err != nil {
		return nil, err
	}

	manager, err := refmanager.New(a.Client, set.Spec.Selector, set, a.Scheme)

	if err != nil {
		return nil, err
	}

	selected := make([]metav1.Object, len(podList.Items))

	for i, pod := range podList.Items {
		selected[i] = pod.DeepCopy()
	}

	claimed, err := manager.ClaimOwnedObjects(selected)

	if err != nil {
		return nil, err
	}

	claimedPods := make([]*corev1.Pod, len(claimed))

	for i, pod := range claimed {
		claimedPods[i] = pod.(*corev1.Pod)
	}

	return claimedPods, nil
}

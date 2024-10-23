package adapter

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"

	alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/refmanager"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
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

func (a *CloneSetAdapter) GetSubsetPods(obj metav1.Object) ([]*corev1.Pod, error) {
	return a.getCloneSetPods(obj.(*alpha1.CloneSet))
}

func (a *CloneSetAdapter) GetSpecReplicas(obj metav1.Object) *int32 {
	return obj.(*alpha1.CloneSet).Spec.Replicas
}

func (a *CloneSetAdapter) GetSpecPartition(obj metav1.Object, _ []*corev1.Pod) *int32 {
	set := obj.(*alpha1.CloneSet)
	if set.Spec.UpdateStrategy.Partition != nil {
		partition, _ := intstr.GetScaledValueFromIntOrPercent(set.Spec.UpdateStrategy.Partition, int(*set.Spec.Replicas), true)
		return ptr.To(int32(partition))
	}
	return nil
}

func (a *CloneSetAdapter) GetStatusReplicas(obj metav1.Object) int32 {
	return obj.(*alpha1.CloneSet).Status.Replicas
}

func (a *CloneSetAdapter) GetStatusReadyReplicas(obj metav1.Object) int32 {
	return obj.(*alpha1.CloneSet).Status.ReadyReplicas
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

	set.Spec = *ud.Spec.Template.CloneSetTemplate.Spec.DeepCopy()
	set.Spec.Selector = selectors
	set.Spec.Replicas = &replicas

	set.Spec.UpdateStrategy.Partition = util.GetIntOrStrPointer(intstr.FromInt32(partition))

	if set.Spec.Template.Labels == nil {
		set.Spec.Template.Labels = map[string]string{}
	}

	set.Spec.Template.Labels[alpha1.SubSetNameLabelKey] = subsetName
	set.Spec.Template.Labels[alpha1.ControllerRevisionHashLabelKey] = revision

	attachNodeAffinity(&set.Spec.Template.Spec, subSetConfig)
	attachTolerations(&set.Spec.Template.Spec, subSetConfig)
	if subSetConfig.Patch.Raw != nil {
		TemplateSpecBytes, _ := json.Marshal(set.Spec.Template)
		modified, err := strategicpatch.StrategicMergePatch(TemplateSpecBytes, subSetConfig.Patch.Raw, &corev1.PodTemplateSpec{})
		if err != nil {
			klog.ErrorS(err, "Failed to merge patch raw", "patch", subSetConfig.Patch.Raw)
			return err
		}
		patchedTemplateSpec := corev1.PodTemplateSpec{}
		if err = json.Unmarshal(modified, &patchedTemplateSpec); err != nil {
			klog.ErrorS(err, "Failed to unmarshal modified JSON to podTemplateSpec", "JSON", modified)
			return err
		}

		set.Spec.Template = patchedTemplateSpec
		klog.V(2).InfoS("CloneSet was patched successfully", "cloneSet", klog.KRef(set.Namespace, set.GenerateName), "patch", subSetConfig.Patch.Raw)
	}
	if set.Annotations == nil {
		set.Annotations = make(map[string]string)
	}
	set.Annotations[alpha1.AnnotationSubsetPatchKey] = string(subSetConfig.Patch.Raw)
	return nil
}

func (a *CloneSetAdapter) PostUpdate(_ *alpha1.UnitedDeployment, _ runtime.Object, _ string, _ int32) error {
	return nil
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

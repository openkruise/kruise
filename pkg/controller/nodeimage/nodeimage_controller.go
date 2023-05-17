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

package nodeimage

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"reflect"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclient "github.com/openkruise/kruise/pkg/client"
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	utildiscovery "github.com/openkruise/kruise/pkg/util/discovery"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	utilimagejob "github.com/openkruise/kruise/pkg/util/imagejob"
	"github.com/openkruise/kruise/pkg/util/requeueduration"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/apimachinery/pkg/util/sets"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/tools/reference"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	nodeutil "k8s.io/kubernetes/pkg/controller/util/node"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

func init() {
	flag.IntVar(&concurrentReconciles, "nodeimage-workers", concurrentReconciles, "Max concurrent workers for NodeImage controller.")
	flag.DurationVar(&nodeImageCreationDelayAfterNodeReady, "nodeimage-creation-delay", nodeImageCreationDelayAfterNodeReady, "Delay duration for NodeImage creation after Node ready.")
}

var (
	concurrentReconciles = 3
	controllerKind       = appsv1alpha1.SchemeGroupVersion.WithKind("NodeImage")

	nodeImageCreationDelayAfterNodeReady = time.Second * 30
)

const (
	controllerName     = "nodeimage-controller"
	minRequeueDuration = 3 * time.Second
	responseTimeout    = 10 * time.Minute

	// Allow fake NodeImage with no Node related, just for tests
	fakeLabelKey = "apps.kruise.io/fake-nodeimage"
)

// Add creates a new NodeImage Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	if !utildiscovery.DiscoverGVK(controllerKind) || !utilfeature.DefaultFeatureGate.Enabled(features.KruiseDaemon) {
		return nil
	}
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	recorder := mgr.GetEventRecorderFor(controllerName)
	if cli := kruiseclient.GetGenericClientWithName(controllerName); cli != nil {
		eventBroadcaster := record.NewBroadcaster()
		eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: cli.KubeClient.CoreV1().Events("")})
		recorder = eventBroadcaster.NewRecorder(mgr.GetScheme(), v1.EventSource{Component: controllerName})
	}
	return &ReconcileNodeImage{
		Client:        utilclient.NewClientFromManager(mgr, controllerName),
		scheme:        mgr.GetScheme(),
		clock:         clock.RealClock{},
		eventRecorder: recorder,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: concurrentReconciles})
	if err != nil {
		return err
	}

	// Watch for changes to NodeImage
	err = c.Watch(&source.Kind{Type: &appsv1alpha1.NodeImage{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to Node
	err = c.Watch(&source.Kind{Type: &v1.Node{}}, &nodeHandler{Reader: mgr.GetCache()})
	if err != nil {
		return err
	}

	// Watch for deletion to ImagePullJob
	err = c.Watch(&source.Kind{Type: &appsv1alpha1.ImagePullJob{}}, &imagePullJobHandler{Reader: mgr.GetCache()})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileNodeImage{}

// ReconcileNodeImage reconciles a NodeImage object
type ReconcileNodeImage struct {
	client.Client
	scheme        *runtime.Scheme
	clock         clock.Clock
	eventRecorder record.EventRecorder
}

// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps.kruise.io,resources=nodeimages,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.kruise.io,resources=nodeimages/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.kruise.io,resources=nodeimages/finalizers,verbs=update

// Reconcile reads that state of the cluster for a NodeImage object and makes changes based on the state read
// and what is in the NodeImage.Spec
// Automatically generate RBAC rules to allow the Controller to read and write nodes
func (r *ReconcileNodeImage) Reconcile(_ context.Context, request reconcile.Request) (res reconcile.Result, err error) {
	start := time.Now()
	klog.V(5).Infof("Starting to process NodeImage %v", request.Name)
	var requeueMsg string
	defer func() {
		if err != nil {
			klog.Warningf("Failed to process NodeImage %v err %v, elapsedTime %v", request.Name, time.Since(start), err)
		} else if res.RequeueAfter > 0 {
			klog.Infof("Finish to process NodeImage %v, elapsedTime %v, RetryAfter %v: %v", request.Name, time.Since(start), res.RequeueAfter, requeueMsg)
		} else {
			klog.Infof("Finish to process NodeImage %v, elapsedTime %v", request.Name, time.Since(start))
		}
	}()

	// Fetch the Node
	node := &v1.Node{}
	err = r.Get(context.TODO(), request.NamespacedName, node)
	if err != nil {
		if !errors.IsNotFound(err) {
			return reconcile.Result{}, fmt.Errorf("failed to get node %s: %v", request.NamespacedName.Name, err)
		}
		node = nil
	}

	// Fetch the NodeImage
	nodeImage := &appsv1alpha1.NodeImage{}
	err = r.Get(context.TODO(), request.NamespacedName, nodeImage)
	if err != nil {
		if !errors.IsNotFound(err) {
			return reconcile.Result{}, fmt.Errorf("failed to get nodeimage %s: %v", request.NamespacedName.Name, err)
		}
		nodeImage = nil
	}

	// If Node not exists or has been deleted
	if node == nil || node.DeletionTimestamp != nil {
		// All been deleted
		if nodeImage == nil || nodeImage.DeletionTimestamp != nil {
			return reconcile.Result{}, nil
		}

		if _, ok := nodeImage.Labels[fakeLabelKey]; !ok {
			// Delete nodeImage
			if err = r.Delete(context.TODO(), nodeImage); err != nil {
				return reconcile.Result{}, fmt.Errorf("failed to delete nodeimage %v, err: %v", nodeImage.Name, err)
			}
			return reconcile.Result{}, nil
		}
	}

	// If Node exists, we should create a NodeImage
	if nodeImage == nil {
		if isReady, delay := getNodeReadyAndDelayTime(node); !isReady {
			klog.V(4).Infof("Skip to create NodeImage %s for Node has not ready yet.", request.Name)
			return reconcile.Result{}, nil
		} else if delay > 0 {
			klog.V(4).Infof("Skip to create NodeImage %s for waiting Node ready for %s.", request.Name, delay)
			return reconcile.Result{RequeueAfter: delay}, nil
		}

		nodeImage = &appsv1alpha1.NodeImage{
			ObjectMeta: metav1.ObjectMeta{
				Name: request.Name,
			},
			Spec: appsv1alpha1.NodeImageSpec{},
		}
		if err = r.Create(context.TODO(), nodeImage); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to create nodeimage %v, err: %v", nodeImage.Name, err)
		}
		klog.Infof("Successfully create nodeimage %s", nodeImage.Name)
		return reconcile.Result{}, nil
	}

	if nodeImage.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	duration := &requeueduration.Duration{}
	if modified, err := r.updateNodeImage(nodeImage.Name, node, duration); err != nil {
		return reconcile.Result{}, err
	} else if modified {
		return reconcile.Result{}, nil
	}
	if err = r.updateNodeImageStatus(nodeImage, duration); err != nil {
		return reconcile.Result{}, err
	}

	res = reconcile.Result{}
	res.RequeueAfter, requeueMsg = duration.GetWithMsg()
	if res.RequeueAfter > 0 && res.RequeueAfter < minRequeueDuration {
		// 3~5s
		res.RequeueAfter = minRequeueDuration + time.Duration(rand.Int31n(2000))*time.Millisecond
	}
	return res, nil
}

func (r *ReconcileNodeImage) updateNodeImage(name string, node *v1.Node, duration *requeueduration.Duration) (bool, error) {
	var modified bool
	var messages []string
	tmpDuration := &requeueduration.Duration{}
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		nodeImage := &appsv1alpha1.NodeImage{}
		err = r.Get(context.TODO(), types.NamespacedName{Name: name}, nodeImage)
		if err != nil {
			return err
		}

		modified, messages, tmpDuration = r.doUpdateNodeImage(nodeImage, node)
		if !modified {
			return nil
		}
		return r.Update(context.TODO(), nodeImage)
	})
	duration.Merge(tmpDuration)
	if err != nil {
		klog.Errorf("Failed to update NodeImage %s spec(%v): %v", name, messages, err)
	} else if modified {
		klog.Infof("Successfully update NodeImage %s spec(%v)", name, messages)
	}
	return modified, err
}

func (r *ReconcileNodeImage) doUpdateNodeImage(nodeImage *appsv1alpha1.NodeImage, node *v1.Node) (modified bool, messages []string, wait *requeueduration.Duration) {
	wait = &requeueduration.Duration{}
	if node != nil {
		if !reflect.DeepEqual(nodeImage.Labels, node.Labels) {
			modified = true
			messages = append(messages, "node labels changed")
			nodeImage.Labels = node.Labels
		}
	}

	newImageMap := make(map[string]appsv1alpha1.ImageSpec, len(nodeImage.Spec.Images))
	for name, imageSpec := range nodeImage.Spec.Images {
		var newTags []appsv1alpha1.ImageTagSpec
		for i := range imageSpec.Tags {
			tagSpec := &imageSpec.Tags[i]
			fullName := fmt.Sprintf("%s:%s", name, tagSpec.Tag)

			// If createdAt field has not been injected by webhook, delete it
			if tagSpec.CreatedAt == nil {
				modified = true
				messages = append(messages, fmt.Sprintf("image %s has no createAt field", fullName))
				continue
			}

			// If tag has owners which have been deleted, delete this tag
			var activeRefs []v1.ObjectReference
			for _, ref := range tagSpec.OwnerReferences {
				gvk := ref.GroupVersionKind()
				if gvk.Group != controllerKind.Group || gvk.Kind != "ImagePullJob" {
					activeRefs = append(activeRefs, ref)
					continue
				}
				job := appsv1alpha1.ImagePullJob{}
				err := r.Get(context.TODO(), types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name}, &job)
				if err != nil {
					if errors.IsNotFound(err) {
						continue
					}
					klog.Errorf("Failed to check owners for %s in NodeImage %s, get job %v error: %v", fullName, name, util.DumpJSON(ref), err)
					activeRefs = append(activeRefs, ref)
					continue
				}
				if job.UID != ref.UID {
					klog.Warningf("When check owners for %s in NodeImage %s, get job %v find UID %v not equal", fullName, name, util.DumpJSON(ref), job.UID)
					continue
				}
				activeRefs = append(activeRefs, ref)
			}
			if len(activeRefs) != len(tagSpec.OwnerReferences) {
				modified = true
				messages = append(messages, fmt.Sprintf("image %s owners changed from %v to %v",
					fullName, util.DumpJSON(tagSpec.OwnerReferences), util.DumpJSON(activeRefs)))
				if len(activeRefs) == 0 {
					continue
				}
				tagSpec.OwnerReferences = activeRefs
			}

			// If tag has TTL and status has completed, prepare to delete this tag
			if tagSpec.PullPolicy != nil && tagSpec.PullPolicy.TTLSecondsAfterFinished != nil {

				var completionTime *metav1.Time
				if imageStatus, ok := nodeImage.Status.ImageStatuses[name]; ok {
					for _, tagStatus := range imageStatus.Tags {
						if tagStatus.Tag == tagSpec.Tag && tagStatus.Version == tagSpec.Version {
							completionTime = tagStatus.CompletionTime
							break
						}
					}
				}

				if completionTime != nil {
					leftTime := time.Duration(*tagSpec.PullPolicy.TTLSecondsAfterFinished)*time.Second - time.Since(completionTime.Time)
					if leftTime <= 0 {
						modified = true
						messages = append(messages, fmt.Sprintf("image %s has exceeded TTL (%v)s since %v completed", fullName, *tagSpec.PullPolicy.TTLSecondsAfterFinished, completionTime))
						continue
					}
					wait.UpdateWithMsg(leftTime, "[spec]image %s wait TTL (%v)s since %v completed", fullName, *tagSpec.PullPolicy.TTLSecondsAfterFinished, completionTime)
				}
			}
			newTags = append(newTags, *tagSpec)
		}
		if len(newTags) > 0 {
			imageSpec.Tags = newTags
			utilimagejob.SortSpecImageTags(&imageSpec)
			newImageMap[name] = imageSpec
		} else {
			modified = true
			messages = append(messages, fmt.Sprintf("no longer has %v image spec", name))
		}
	}
	if modified {
		nodeImage.Spec.Images = newImageMap
	}
	return
}

func (r *ReconcileNodeImage) updateNodeImageStatus(nodeImage *appsv1alpha1.NodeImage, duration *requeueduration.Duration) error {
	now := metav1.NewTime(r.clock.Now())

	specFullImages := sets.NewString()
	newStatus := nodeImage.Status.DeepCopy()

	for name, imageSpec := range nodeImage.Spec.Images {

		imageStatus := newStatus.ImageStatuses[name]
		for i := range imageSpec.Tags {
			tagSpec := &imageSpec.Tags[i]
			fullName := fmt.Sprintf("%s:%s", name, tagSpec.Tag)
			specFullImages.Insert(fullName)
			if tagSpec.CreatedAt == nil {
				continue
			}

			var tagStatus *appsv1alpha1.ImageTagStatus
			for i := range imageStatus.Tags {
				if imageStatus.Tags[i].Tag == tagSpec.Tag {
					tagStatus = &imageStatus.Tags[i]
					break
				}
			}

			var failed bool

			// image-puller not responded for a 1min
			if tagStatus == nil || tagStatus.Version != tagSpec.Version {
				if leftTime := responseTimeout - time.Since(tagSpec.CreatedAt.Time); leftTime > 0 {
					duration.UpdateWithMsg(leftTime, "[status]image %s wait response timeout (60)s since %v created", fullName, tagSpec.CreatedAt)
					continue
				}
				if tagStatus == nil {
					tagStatus = &appsv1alpha1.ImageTagStatus{
						Tag:            tagSpec.Tag,
						Phase:          appsv1alpha1.ImagePhaseFailed,
						CompletionTime: &now,
						Version:        tagSpec.Version,
						Message:        "node has not responded for a long time",
					}
					imageStatus.Tags = append(imageStatus.Tags, *tagStatus)
				} else {
					tagStatus.Phase = appsv1alpha1.ImagePhaseFailed
					tagStatus.CompletionTime = &now
					tagStatus.Version = tagSpec.Version
					tagStatus.Message = "node has not responded for a long time"
				}
				failed = true
			}

			// activeDeadlineSeconds timeout
			if tagStatus.CompletionTime == nil && tagSpec.PullPolicy.ActiveDeadlineSeconds != nil {
				if leftTime := time.Duration(*tagSpec.PullPolicy.ActiveDeadlineSeconds)*time.Second + responseTimeout - time.Since(tagSpec.CreatedAt.Time); leftTime > 0 {
					duration.UpdateWithMsg(leftTime, "[status]image %s wait deadline (%v)s since %v created", fullName, *tagSpec.PullPolicy.ActiveDeadlineSeconds, tagSpec.CreatedAt)
					continue
				}
				tagStatus.Phase = appsv1alpha1.ImagePhaseFailed
				tagStatus.CompletionTime = &now
				tagStatus.Version = tagSpec.Version
				tagStatus.Message = "pulling exceeds the activeDeadlineSeconds"
				failed = true
			}

			// It means this tag has been failed
			if failed {
				if r.eventRecorder != nil {
					for _, owner := range tagSpec.OwnerReferences {
						r.eventRecorder.Eventf(&owner, v1.EventTypeWarning, "PullImageFailed", "Failed to pull image %v on node %v for %v", fullName, nodeImage.Name, tagStatus.Message)
					}
					if ref, _ := reference.GetReference(r.scheme, nodeImage); ref != nil {
						r.eventRecorder.Eventf(ref, v1.EventTypeWarning, "PullImageFailed", "Failed to pull image %v on node %v for %v", fullName, nodeImage.Name, tagStatus.Message)
					}
				}
			}
		}

		if len(imageStatus.Tags) > 0 {
			if newStatus.ImageStatuses == nil {
				newStatus.ImageStatuses = make(map[string]appsv1alpha1.ImageStatus)
			}
			utilimagejob.SortStatusImageTags(&imageStatus)
			newStatus.ImageStatuses[name] = imageStatus
		}
	}

	if util.IsJSONObjectEqual(newStatus.ImageStatuses, nodeImage.Status.ImageStatuses) {
		return nil
	}

	succeeded, failed, pulling := 0, 0, 0
	newImagesStatus := make(map[string]appsv1alpha1.ImageStatus, len(nodeImage.Spec.Images))
	for name, imageStatus := range newStatus.ImageStatuses {
		if _, ok := nodeImage.Spec.Images[name]; !ok {
			continue
		}
		newTags := make([]appsv1alpha1.ImageTagStatus, 0, len(imageStatus.Tags))
		for _, tagStatus := range imageStatus.Tags {
			fullName := fmt.Sprintf("%s:%s", name, tagStatus.Tag)
			if !specFullImages.Has(fullName) {
				continue
			}
			newTags = append(newTags, tagStatus)
			switch tagStatus.Phase {
			case appsv1alpha1.ImagePhaseSucceeded:
				succeeded++
			case appsv1alpha1.ImagePhasePulling:
				pulling++
			case appsv1alpha1.ImagePhaseFailed:
				failed++
			}
		}
		if len(newTags) > 0 {
			imageStatus.Tags = newTags
			newImagesStatus[name] = imageStatus
		}
	}
	newStatus.ImageStatuses = newImagesStatus
	newStatus.Desired = int32(specFullImages.Len())
	newStatus.Pulling = int32(pulling)
	newStatus.Succeeded = int32(succeeded)
	newStatus.Failed = int32(failed)

	klog.V(3).Infof("Preparing to update status for NodeImage %s, old: %v, new: %v", nodeImage.Name, util.DumpJSON(nodeImage.Status), util.DumpJSON(newStatus))
	nodeImage.Status = *newStatus
	err := r.Status().Update(context.TODO(), nodeImage)
	if err != nil && !errors.IsConflict(err) {
		klog.Errorf("Failed to update status for NodeImage %v: %v", nodeImage.Name, err)
	}
	return err
}

func getNodeReadyAndDelayTime(node *v1.Node) (bool, time.Duration) {
	_, condition := nodeutil.GetNodeCondition(&node.Status, v1.NodeReady)
	if condition == nil || condition.Status != v1.ConditionTrue {
		return false, 0
	}
	delay := nodeImageCreationDelayAfterNodeReady - time.Since(condition.LastTransitionTime.Time)
	if delay > 0 {
		return true, delay
	}
	return true, 0
}

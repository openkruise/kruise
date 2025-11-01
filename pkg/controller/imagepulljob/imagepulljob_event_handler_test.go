package imagepulljob

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/util/fieldindex"
)

func init() {
	scheme = runtime.NewScheme()
	utilruntime.Must(appsv1beta1.AddToScheme(scheme))
}

var (
	scheme *runtime.Scheme
)

func TestNodeImageEventHandler_handleUpdate(t *testing.T) {
	tests := []struct {
		name         string
		jobs         []client.Object
		nodeImage    *appsv1beta1.NodeImage
		oldNodeImage *appsv1beta1.NodeImage
		expectedAdds []types.NamespacedName
	}{
		{
			name: "image spec changed",
			jobs: []client.Object{
				&appsv1beta1.ImagePullJob{
					ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "default"},
					Spec: appsv1beta1.ImagePullJobSpec{
						Image: "nginx:1.20",
					},
				},
			},
			nodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "1.20", Version: 1},
							},
						},
					},
				},
				Status: appsv1beta1.NodeImageStatus{
					ImageStatuses: map[string]appsv1beta1.ImageStatus{
						"nginx": {
							Tags: []appsv1beta1.ImageTagStatus{
								{Tag: "1.20", Version: 1, Phase: appsv1beta1.ImagePhaseSucceeded},
							},
						},
					},
				},
			},
			oldNodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "1.19", Version: 1},
							},
						},
					},
				},
				Status: appsv1beta1.NodeImageStatus{
					ImageStatuses: map[string]appsv1beta1.ImageStatus{
						"nginx": {
							Tags: []appsv1beta1.ImageTagStatus{
								{Tag: "1.19", Version: 1, Phase: appsv1beta1.ImagePhaseSucceeded},
							},
						},
					},
				},
			},
			expectedAdds: []types.NamespacedName{
				{Namespace: "default", Name: "job1"},
			},
		},
		{
			name: "image added to spec",
			jobs: []client.Object{
				&appsv1beta1.ImagePullJob{
					ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "default"},
					Spec: appsv1beta1.ImagePullJobSpec{
						Image: "redis:6.0",
					},
				},
			},
			nodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "1.20", Version: 1},
							},
						},
						"redis": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "6.0", Version: 1},
							},
						},
					},
				},
			},
			oldNodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "1.20", Version: 1},
							},
						},
					},
				},
			},
			expectedAdds: []types.NamespacedName{
				{Namespace: "default", Name: "job1"},
			},
		},
		{
			name: "image removed from spec",
			jobs: []client.Object{
				&appsv1beta1.ImagePullJob{
					ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "default"},
					Spec: appsv1beta1.ImagePullJobSpec{
						Image: "redis:6.0",
					},
				},
			},
			nodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "1.20", Version: 1},
							},
						},
					},
				},
			},
			oldNodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "1.20", Version: 1},
							},
						},
						"redis": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "6.0", Version: 1},
							},
						},
					},
				},
			},
			expectedAdds: []types.NamespacedName{
				{Namespace: "default", Name: "job1"},
			},
		},
		{
			name: "image status changed",
			jobs: []client.Object{
				&appsv1beta1.ImagePullJob{
					ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "default"},
					Spec: appsv1beta1.ImagePullJobSpec{
						Image: "nginx:1.20",
					},
				},
			},
			nodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "1.20", Version: 1},
							},
						},
					},
				},
				Status: appsv1beta1.NodeImageStatus{
					ImageStatuses: map[string]appsv1beta1.ImageStatus{
						"nginx": {
							Tags: []appsv1beta1.ImageTagStatus{
								{Tag: "1.20", Version: 1, Phase: appsv1beta1.ImagePhaseSucceeded},
							},
						},
					},
				},
			},
			oldNodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "1.20", Version: 1},
							},
						},
					},
				},
				Status: appsv1beta1.NodeImageStatus{
					ImageStatuses: map[string]appsv1beta1.ImageStatus{
						"nginx": {
							Tags: []appsv1beta1.ImageTagStatus{
								{Tag: "1.20", Version: 1, Phase: appsv1beta1.ImagePhasePulling},
							},
						},
					},
				},
			},
			expectedAdds: []types.NamespacedName{
				{Namespace: "default", Name: "job1"},
			},
		},
		{
			name: "no changes",
			jobs: []client.Object{
				&appsv1beta1.ImagePullJob{
					ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "default"},
					Spec: appsv1beta1.ImagePullJobSpec{
						Image: "nginx:1.20",
					},
				},
			},
			nodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "1.20", Version: 1},
							},
						},
					},
				},
				Status: appsv1beta1.NodeImageStatus{
					ImageStatuses: map[string]appsv1beta1.ImageStatus{
						"nginx": {
							Tags: []appsv1beta1.ImageTagStatus{
								{Tag: "1.20", Version: 1, Phase: appsv1beta1.ImagePhaseSucceeded},
							},
						},
					},
				},
			},
			oldNodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "1.20", Version: 1},
							},
						},
					},
				},
				Status: appsv1beta1.NodeImageStatus{
					ImageStatuses: map[string]appsv1beta1.ImageStatus{
						"nginx": {
							Tags: []appsv1beta1.ImageTagStatus{
								{Tag: "1.20", Version: 1, Phase: appsv1beta1.ImagePhaseSucceeded},
							},
						},
					},
				},
			},
			expectedAdds: []types.NamespacedName{},
		},
		{
			name: "invalid image",
			jobs: []client.Object{
				&appsv1beta1.ImagePullJob{
					ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "default"},
					Spec: appsv1beta1.ImagePullJobSpec{
						Image: "redis:6.0:latest",
					},
				},
			},
			nodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "1.20", Version: 1},
							},
						},
						"redis": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "6.0:latest", Version: 1},
							},
						},
					},
				},
			},
			oldNodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "1.20", Version: 1},
							},
						},
					},
				},
			},
			expectedAdds: []types.NamespacedName{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock queue
			q := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
			defer q.ShutDown()

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.jobs...).
				WithIndex(
					&appsv1beta1.ImagePullJob{}, fieldindex.IndexNameForIsActive, fieldindex.IndexImagePullJob,
				).Build()
			// Create event handler
			handler := &nodeImageEventHandler{fakeClient}

			// Call handleUpdate
			handler.handleUpdate(tt.nodeImage, tt.oldNodeImage, q)

			// Check the queue size
			assert.Equal(t, len(tt.expectedAdds), q.Len())

			// Check the added items if needed
			addedItems := make([]types.NamespacedName, 0, len(tt.expectedAdds))
			for i := 0; i < len(tt.expectedAdds); i++ {
				item, _ := q.Get()
				addedItems = append(addedItems, item.NamespacedName)
				q.Done(item)
			}
			assert.ElementsMatch(t, tt.expectedAdds, addedItems)
		})
	}
}

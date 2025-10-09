package imagepuller

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/tools/reference"
	"k8s.io/klog/v2"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	policyv1alpha1 "github.com/openkruise/kruise/apis/policy/v1alpha1"
	"github.com/openkruise/kruise/pkg/daemon/criruntime/imageruntime"
)

type fakeRuntime struct {
	mu     sync.Mutex
	images map[string]*imageStatus
}

type imageStatus struct {
	progress int
	statusCh chan int
}

func (f *fakeRuntime) PullImage(ctx context.Context, imageName, tag string, pullSecrets []v1.Secret, sandboxConfig *appsv1beta1.SandboxConfig) (imageruntime.ImagePullStatusReader, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	key := imageName + ":" + tag
	if _, exist := f.images[key]; !exist {
		f.images[key] = &imageStatus{
			progress: 0,
			statusCh: make(chan int, 10),
		}
	}
	r := newFakeStatusReader(key, f.images[key])
	go func() {
		ticker := time.NewTicker(time.Second * 1)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				func() {
					f.mu.Lock()
					defer f.mu.Unlock()
					val, exist := f.images[key]
					if !exist {
						return
					}
					if val.progress >= 100 {
						r.ch <- imageruntime.ImagePullStatus{
							Err:        nil,
							Process:    100,
							DetailInfo: "finished",
							Finish:     true,
						}
					}
				}()
			case <-r.done:
				func() {
					f.mu.Lock()
					defer f.mu.Unlock()
					delete(f.images, key)
				}()
				return
			}
		}
	}()
	return &r, nil
}

func (f *fakeRuntime) ListImages(ctx context.Context) ([]imageruntime.ImageInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var images []imageruntime.ImageInfo
	for name := range f.images {
		images = append(images, imageruntime.ImageInfo{ID: name})
	}
	return images, nil
}

func (f *fakeRuntime) increaseProgress(image string, delta int) {
	key := image
	f.mu.Lock()
	defer f.mu.Unlock()
	klog.Infof("increase progress %v +%v", key, delta)

	if status, exist := f.images[key]; exist {
		status.progress += delta
		if status.progress > 100 {
			status.progress = 100
		}
		status.statusCh <- status.progress
	}
}

func (f *fakeRuntime) clean() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.images = make(map[string]*imageStatus)
}

type fakeStatusReader struct {
	image  string
	status *imageStatus
	ch     chan imageruntime.ImagePullStatus
	mu     sync.Mutex
	closed bool
	done   chan struct{}
}

func newFakeStatusReader(image string, status *imageStatus) fakeStatusReader {
	return fakeStatusReader{
		image:  image,
		status: status,
		ch:     make(chan imageruntime.ImagePullStatus, 10),
		done:   make(chan struct{}),
	}
}

func (r *fakeStatusReader) C() <-chan imageruntime.ImagePullStatus {
	return r.ch
}

func (r *fakeStatusReader) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.closed {
		klog.InfoS("Closing reader", "image", r.image)
		close(r.done)
		r.closed = true
	}
}

type fakeSecret struct {
}

func (f *fakeSecret) GetSecrets(secret []appsv1beta1.ReferenceObject) ([]v1.Secret, error) {
	return nil, nil
}

// test case here
func realPullerSyncFn(t *testing.T, limitedPool bool) {
	eventRecorder := record.NewFakeRecorder(100)
	secretManager := fakeSecret{}

	baseNodeImage := &appsv1beta1.NodeImage{
		Spec: appsv1beta1.NodeImageSpec{
			Images: map[string]appsv1beta1.ImageSpec{
				"nginx": {
					Tags: []appsv1beta1.ImageTagSpec{
						{Tag: "latest", Version: 1},
					},
				},
			},
		},
	}

	testCases := []struct {
		name        string
		prePools    map[string]struct{}
		inputSpec   *appsv1beta1.NodeImage
		expectPools sets.String
		expectErr   bool
	}{
		{
			name:        "add new image",
			inputSpec:   baseNodeImage,
			expectPools: sets.NewString("nginx"),
		},
		{
			name:        "remove image",
			prePools:    map[string]struct{}{"redis": {}},
			inputSpec:   baseNodeImage,
			expectPools: sets.NewString("nginx"),
		},
		{
			name:     "add image",
			prePools: map[string]struct{}{"nginx": {}},
			inputSpec: &appsv1beta1.NodeImage{
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "latest", Version: 1},
							},
						},
						"busybox": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "1.15", Version: 1},
							},
						},
					},
				},
			},
			expectPools: sets.NewString("nginx", "busybox"),
		},
	}
	fakeRuntime := &fakeRuntime{images: make(map[string]*imageStatus)}
	workerLimitedPool = NewChanPool(2)
	workerLimitedPool.Start()
	p, _ := newRealPuller(fakeRuntime, &secretManager, eventRecorder)

	nameSuffuix := "_NoLimitPool"
	if limitedPool {
		nameSuffuix = "_LimitPool"
	}
	for _, tc := range testCases {
		t.Run(tc.name+nameSuffuix, func(t *testing.T) {
			for poolName := range tc.prePools {
				p.workerPools[poolName] = newRealWorkerPool(poolName, fakeRuntime, &secretManager, eventRecorder)
			}
			ref, _ := reference.GetReference(scheme, tc.inputSpec)
			err := p.Sync(tc.inputSpec, ref)
			if (err != nil) != tc.expectErr {
				t.Fatalf("expect error %v, but got %v", tc.expectErr, err)
			}

			actualPools := sets.String{}
			for name := range p.workerPools {
				p.workerPools[name].Stop()
				actualPools.Insert(name)
			}
			if !actualPools.Equal(tc.expectPools) {
				t.Errorf("expect pools %v, but got %v", tc.expectPools.List(), actualPools.List())
			}

			for prePool := range tc.prePools {
				if !tc.expectPools.Has(prePool) {
					if pool, exists := p.workerPools[prePool]; exists {
						if pool.(*realWorkerPool).active {
							t.Errorf("expect pool %s to be stopped", prePool)
						}
					}
				}
			}
		})
	}

	t.Log("clean fake runtime")
	time.Sleep(1 * time.Second)
	infos, _ := fakeRuntime.ListImages(context.Background())
	for _, info := range infos {
		fakeRuntime.increaseProgress(info.ID, 100)
	}
	time.Sleep(2 * time.Second)

	// clear errgroup
	for _, val := range p.workerPools {
		val.Stop()
	}
	fakeRuntime.clean()
}

func TestRealPullerSync(t *testing.T) {
	realPullerSyncFn(t, false)
	realPullerSyncFn(t, true)
}

var scheme *runtime.Scheme

func init() {
	scheme = runtime.NewScheme()
	utilruntime.Must(appsv1beta1.AddToScheme(scheme))
	utilruntime.Must(appsv1beta1.AddToScheme(scheme))
	utilruntime.Must(policyv1alpha1.AddToScheme(scheme))
}

func TestRealPullerSyncWithLimitedPool(t *testing.T) {
	eventRecorder := record.NewFakeRecorder(100)
	secretManager := fakeSecret{}

	baseNodeImage := &appsv1beta1.NodeImage{
		Spec: appsv1beta1.NodeImageSpec{
			Images: map[string]appsv1beta1.ImageSpec{
				"nginx": {
					Tags: []appsv1beta1.ImageTagSpec{
						{Tag: "latest", Version: 1},
					},
				},
				"busybox": {
					Tags: []appsv1beta1.ImageTagSpec{
						{Tag: "latest", Version: 1},
					},
				},
				"redis": {
					Tags: []appsv1beta1.ImageTagSpec{
						{Tag: "latest", Version: 1},
					},
				},
			},
		},
	}

	r := &fakeRuntime{images: make(map[string]*imageStatus)}
	workerLimitedPool = NewChanPool(2)
	workerLimitedPool.Start()
	p, _ := newRealPuller(r, &secretManager, eventRecorder)

	ref, _ := reference.GetReference(scheme, baseNodeImage)
	err := p.Sync(baseNodeImage, ref)
	assert.Nil(t, err)

	time.Sleep(100 * time.Millisecond)
	checkPhase := func(expectRunning, expectWaiting int) {
		running, waiting := 0, 0
		for imageName := range baseNodeImage.Spec.Images {
			status := p.GetStatus(imageName)
			t.Log(status)
			if status == nil {
				t.Errorf("expect image %s to be running", imageName)
				continue
			}
			if status.Tags[0].Phase == appsv1beta1.ImagePhasePulling {
				running++
			} else if status.Tags[0].Phase == appsv1beta1.ImagePhaseWaiting {
				waiting++
			}
		}
		assert.Equal(t, expectRunning, running)
		assert.Equal(t, expectWaiting, waiting)
	}
	// first pulling 2, and 1 waiting
	checkPhase(2, 1)
	infos, _ := r.ListImages(context.Background())
	if len(infos) > 0 {
		r.increaseProgress(infos[0].ID, 100)
	}
	time.Sleep(2 * time.Second)
	// when 1 success, 2 pulling, 0 waiting
	checkPhase(2, 0)
	infos, _ = r.ListImages(context.Background())
	for _, info := range infos {
		r.increaseProgress(info.ID, 100)
	}
	time.Sleep(2 * time.Second)
	// when 3 success, 0 pulling, 0 waiting
	checkPhase(0, 0)

	t.Log("clean fake runtime")
	// clear errgroup
	for _, val := range p.workerPools {
		val.Stop()
	}

	r.clean()
}

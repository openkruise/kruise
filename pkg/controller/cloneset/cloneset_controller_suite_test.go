/*
Copyright 2019 The Kruise Authors.

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

package cloneset

import (
	"context"
	"fmt"
	stdlog "log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	corev1 "k8s.io/kubernetes/pkg/apis/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/openkruise/kruise/apis"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	clonesettest "github.com/openkruise/kruise/pkg/controller/cloneset/test"
	"github.com/openkruise/kruise/pkg/util"
)

func init() {
	testscheme = k8sruntime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(testscheme))
	utilruntime.Must(appsv1alpha1.AddToScheme(testscheme))
}

var testscheme *k8sruntime.Scheme

var cfg *rest.Config

func TestMain(m *testing.M) {
	runtime.GOMAXPROCS(4)
	t := &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
	}
	utilruntime.Must(apis.AddToScheme(scheme.Scheme))

	var err error
	if cfg, err = t.Start(); err != nil {
		stdlog.Fatal(err)
	}

	code := m.Run()
	t.Stop()
	os.Exit(code)
}

// StartTestManager adds recFn
func StartTestManager(ctx context.Context, mgr manager.Manager, g *gomega.GomegaWithT) *sync.WaitGroup {
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		g.Expect(mgr.Start(ctx)).NotTo(gomega.HaveOccurred())
	}()
	return wg
}

func TestCleanupPVCs(t *testing.T) {
	cases := []struct {
		name       string
		getCS      func() *appsv1alpha1.CloneSet
		getPods    func() (activePods, inactivePods []*v1.Pod)
		getPVCs    func() []*v1.PersistentVolumeClaim
		expectPVCs func() (usedPVCs, uselessPVCs sets.String)
	}{
		{
			name: "test1",
			getCS: func() *appsv1alpha1.CloneSet {
				obj := clonesettest.NewCloneSet(5)
				return obj
			},
			getPods: func() (activePods, inactivePods []*v1.Pod) {
				ti := metav1.Now()
				for i := 0; i < 2; i++ {
					obj := &v1.Pod{
						TypeMeta: metav1.TypeMeta{
							Kind:       "Pod",
							APIVersion: "v1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: fmt.Sprintf("foo-%d", i),
							UID:  types.UID(fmt.Sprintf("foo-%d", i)),
							Labels: map[string]string{
								appsv1alpha1.CloneSetInstanceID: fmt.Sprintf("instance-%d", i),
							},
							DeletionTimestamp: &ti,
						},
					}
					activePods = append(activePods, obj)
				}
				for i := 2; i < 5; i++ {
					obj := &v1.Pod{
						TypeMeta: metav1.TypeMeta{
							Kind:       "Pod",
							APIVersion: "v1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: fmt.Sprintf("foo-%d", i),
							UID:  types.UID(fmt.Sprintf("foo-%d", i)),
							Labels: map[string]string{
								appsv1alpha1.CloneSetInstanceID: fmt.Sprintf("instance-%d", i),
							},
						},
					}
					inactivePods = append(inactivePods, obj)
				}
				return
			},
			getPVCs: func() []*v1.PersistentVolumeClaim {
				var pvcs []*v1.PersistentVolumeClaim
				cs := clonesettest.NewCloneSet(5)
				for i := 0; i < 5; i++ {
					obj := &v1.PersistentVolumeClaim{
						TypeMeta: metav1.TypeMeta{
							Kind:       "PersistentVolumeClaim",
							APIVersion: "v1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: fmt.Sprintf("foo-pvc-%d", i),
							Labels: map[string]string{
								appsv1alpha1.CloneSetInstanceID: fmt.Sprintf("instance-%d", i),
							},
						},
					}
					util.SetOwnerRef(obj, cs, cs.GroupVersionKind())
					pvcs = append(pvcs, obj)
				}
				return pvcs
			},
			expectPVCs: func() (usedPVCs, uselessPVCs sets.String) {
				usedPVCs = sets.NewString()
				uselessPVCs = sets.NewString()
				for i := 0; i < 5; i++ {
					if i < 2 {
						uselessPVCs.Insert(fmt.Sprintf("foo-pvc-%d", i))
					} else {
						usedPVCs.Insert(fmt.Sprintf("foo-pvc-%d", i))
					}
				}
				return
			},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(testscheme).Build()
			inactivePods, activePods := cs.getPods()
			pvcs := cs.getPVCs()
			for _, obj := range pvcs {
				_ = fakeClient.Create(context.TODO(), obj)
			}
			pvcList := v1.PersistentVolumeClaimList{}
			_ = fakeClient.List(context.TODO(), &pvcList)

			r := &ReconcileCloneSet{Client: fakeClient}
			oPVCs, err := r.cleanupPVCs(cs.getCS(), activePods, inactivePods, pvcs)
			if err != nil {
				t.Fatalf("cleanupPVCs failed: %s", err.Error())
			}
			usedPVCs, uselessPVCs := cs.expectPVCs()
			for _, obj := range oPVCs {
				if !usedPVCs.Has(obj.Name) {
					t.Fatalf("get pvc(%s) error", obj.Name)
				}
			}
			pvcList = v1.PersistentVolumeClaimList{}
			_ = fakeClient.List(context.TODO(), &pvcList)
			if len(pvcList.Items) != usedPVCs.Len()+len(uselessPVCs) {
				t.Fatalf("get(%s) error", util.DumpJSON(pvcList.Items))
			}
			for _, obj := range pvcList.Items {
				ref := metav1.GetControllerOf(&obj)
				if usedPVCs.Has(obj.Name) && ref.Kind != "CloneSet" {
					t.Fatalf("get pvc(%s) ref kind is CloneSet error", obj.Name)
				}
				if uselessPVCs.Has(obj.Name) && ref.Kind != "Pod" {
					t.Fatalf("get pvc(%s) ref kind is Pod error", obj.Name)
				}
			}
		})
	}
}

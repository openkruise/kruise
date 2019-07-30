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

package broadcastjob

import (
	"k8s.io/client-go/rest"
)

var cfg *rest.Config

/*func TestMain(m *testing.M) {
	//t := &envtest.Environment{
	//	CRDDirectoryPaths: []string{filepath.Join("..", "..", "..", "config", "crds")},
	//}
	//apis.AddToScheme(scheme.Scheme)
	//
	//var err error
	//if cfg, err = t.Start(); err != nil {
	//	stdlog.Fatal(err)
	//}
	//
	//code := m.Run()
	//t.Stop()
	//os.Exit(code)
}

// SetupTestReconcile returns a reconcile.Reconcile implementation that delegates to inner and
// writes the request to requests after Reconcile is finished.
func SetupTestReconcile(inner reconcile.Reconciler) (reconcile.Reconciler, chan reconcile.Request) {
	requests := make(chan reconcile.Request)
	fn := reconcile.Func(func(req reconcile.Request) (reconcile.Result, error) {
		result, err := inner.Reconcile(req)
		requests <- req
		return result, err
	})
	return fn, requests
}

// StartTestManager adds recFn
func StartTestManager(mgr manager.Manager, g *gomega.GomegaWithT) (chan struct{}, *sync.WaitGroup) {
	stop := make(chan struct{})
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		g.Expect(mgr.Start(stop)).NotTo(gomega.HaveOccurred())
	}()
	return stop, wg
}

var c client.Client

var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo", Namespace: "default"}}
var depKey = types.NamespacedName{Name: "foo-deployment", Namespace: "default"}

const timeout = time.Second * 5

func TestReconcile(t *testing.T) {
	//g := gomega.NewGomegaWithT(t)
	//instance := &appsv1alpha1.BroadcastJob{
	//	ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
	//	Spec: appsv1alpha1.BroadcastJobSpec {
	//		Template: v1.PodTemplateSpec{
	//			Spec: v1.PodSpec{
	//
	//			},
	//		},
	//	},
	//}
	//
	//// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	//// channel when it is finished.
	//mgr, err := manager.New(cfg, manager.Options{})
	//g.Expect(err).NotTo(gomega.HaveOccurred())
	//c = mgr.GetClient()
	//
	//recFn, requests := SetupTestReconcile(newReconciler(mgr))
	//g.Expect(add(mgr, recFn)).NotTo(gomega.HaveOccurred())
	//
	//stopMgr, mgrStopped := StartTestManager(mgr, g)
	//
	//defer func() {
	//	close(stopMgr)
	//	mgrStopped.Wait()
	//}()
	//
	//// Create the BroadcastJob object and expect the Reconcile and Deployment to be created
	//err = c.Create(context.TODO(), instance)
	//// The instance object may not be a valid object because it might be missing some required fields.
	//// Please modify the instance object by adding required fields and then remove the following if statement.
	//if apierrors.IsInvalid(err) {
	//	t.Logf("failed to create object, got an invalid object error: %v", err)
	//	return
	//}
	//g.Expect(err).NotTo(gomega.HaveOccurred())
	//defer c.Delete(context.TODO(), instance)
	//g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	//deploy := &appsv1.Deployment{}
	//g.Eventually(func() error { return c.Get(context.TODO(), depKey, deploy) }, timeout).
	//	Should(gomega.Succeed())
	//
	//// Delete the Deployment and expect Reconcile to be called for Deployment deletion
	//g.Expect(c.Delete(context.TODO(), deploy)).NotTo(gomega.HaveOccurred())
	//g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
	//g.Eventually(func() error { return c.Get(context.TODO(), depKey, deploy) }, timeout).
	//	Should(gomega.Succeed())
	//
	//// Manually delete Deployment since GC isn't enabled in the test control plane
	//g.Eventually(func() error { return c.Delete(context.TODO(), deploy) }, timeout).
	//	Should(gomega.MatchError("deployments.apps \"foo-deployment\" not found"))
}*/

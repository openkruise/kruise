package apps

import (
	"fmt"

	"github.com/onsi/ginkgo"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/test/e2e/framework"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientset "k8s.io/client-go/kubernetes"
)

var _ = SIGDescribe("uniteddeployment", func() {
	f := framework.NewDefaultFramework("uniteddeployment")
	var ns string
	var c clientset.Interface
	var kc kruiseclientset.Interface
	var tester *framework.UnitedDeploymentTester

	ginkgo.BeforeEach(func() {
		c = f.ClientSet
		kc = f.KruiseClientSet
		ns = f.Namespace.Name
		tester = framework.NewUnitedDeploymentTester(c, kc, ns)
	})

	ginkgo.It("united deployment with elastic allocator", func() {
		replicas := func(r int) *intstr.IntOrString { p := intstr.FromInt(r); return &p }
		udManager := tester.NewUnitedDeploymentManager("ud-elastic-test")
		udManager.AddSubset("subset-0", nil, replicas(1), replicas(2))
		udManager.AddSubset("subset-1", nil, replicas(1), replicas(2))
		udManager.AddSubset("subset-2", nil, replicas(1), nil)

		replicasMap := func(replicas []int32) map[string]int32 {
			replicaMap := make(map[string]int32)
			for i, r := range replicas {
				replicaMap[fmt.Sprintf("subset-%d", i)] = r
			}
			return replicaMap
		}
		ginkgo.By("test replicas equals to sum of min replicas")
		udManager.Create(3)
		udManager.CheckSubsets(replicasMap([]int32{1, 1, 1}))

		ginkgo.By("test replicas more than sum of min replicas")
		udManager.Scale(7)
		udManager.CheckSubsets(replicasMap([]int32{2, 2, 3}))

		ginkgo.By("test replicas less than sum of min replicas")
		udManager.Scale(1)
		udManager.CheckSubsets(replicasMap([]int32{1, 0, 0}))
	})

	ginkgo.It("united deployment with specific allocator", func() {
		replicas := func(p string) *intstr.IntOrString { x := intstr.FromString(p); return &x }
		udManager := tester.NewUnitedDeploymentManager("ud-specific-test")
		udManager.AddSubset("subset-0", replicas("25%"), nil, nil)
		udManager.AddSubset("subset-1", replicas("25%"), nil, nil)
		udManager.AddSubset("subset-2", nil, nil, nil)

		replicasMap := func(replicas []int32) map[string]int32 {
			replicaMap := make(map[string]int32)
			for i, r := range replicas {
				replicaMap[fmt.Sprintf("subset-%d", i)] = r
			}
			return replicaMap
		}
		ginkgo.By("create ud")
		udManager.Create(3)
		udManager.CheckSubsets(replicasMap([]int32{1, 1, 1}))

		ginkgo.By("scale up")
		udManager.Scale(4)
		udManager.CheckSubsets(replicasMap([]int32{1, 1, 2}))

		ginkgo.By("scale down")
		udManager.Scale(1)
		udManager.CheckSubsets(replicasMap([]int32{0, 0, 1}))
	})
})

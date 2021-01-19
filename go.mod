module github.com/openkruise/kruise

go 1.13

require (
	github.com/Nvveen/Gotty v0.0.0-20120604004816-cd527374f1e5 // indirect
	github.com/alibaba/pouch v0.0.0-20190328125340-37051654f368
	github.com/appscode/jsonpatch v1.0.1
	github.com/codegangsta/negroni v1.0.0
	github.com/containerd/containerd v1.2.10
	github.com/containerd/continuity v0.0.0-20201208142359-180525291bb7 // indirect
	github.com/containerd/fifo v0.0.0-20201026212402-0724c46b320c // indirect
	github.com/contiv/executor v0.0.0-20180626233236-d263f4daa3ad // indirect
	github.com/docker/distribution v2.7.2-0.20200708230840-70e0022e42fd+incompatible
	github.com/docker/docker v1.4.2-0.20180612054059-a9fbbdc8dd87
	github.com/docker/go-events v0.0.0-20190806004212-e31b211e4f1c // indirect
	github.com/go-bindata/go-bindata v3.1.2+incompatible // indirect
	github.com/gogo/googleapis v1.4.0 // indirect
	github.com/gorilla/mux v1.7.3
	github.com/onsi/ginkgo v1.12.1
	github.com/onsi/gomega v1.10.1
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.0.1
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.0.0
	github.com/robfig/cron v1.2.0
	github.com/stretchr/testify v1.4.0
	github.com/xyproto/simpleredis v0.0.0-20200201215242-1ff0da2967b4
	golang.org/x/net v0.0.0-20200520004742-59133d7f0dd7
	golang.org/x/time v0.0.0-20190308202827-9d24e82272b4
	gomodules.xyz/jsonpatch/v2 v2.0.1
	google.golang.org/grpc v1.23.0
	k8s.io/api v0.17.7
	k8s.io/apiextensions-apiserver v0.17.7
	k8s.io/apimachinery v0.17.7
	k8s.io/apiserver v0.16.6
	k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	k8s.io/code-generator v0.16.6
	k8s.io/cri-api v0.16.6
	k8s.io/klog v1.0.0
	k8s.io/kubernetes v1.16.6
	k8s.io/utils v0.0.0-20200619165400-6e3d28b6ed19
	sigs.k8s.io/controller-runtime v0.5.7
)

// Replace to match K8s 1.16
replace (
	k8s.io/api => k8s.io/api v0.16.6
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.16.6
	k8s.io/apimachinery => k8s.io/apimachinery v0.16.6
	k8s.io/apiserver => k8s.io/apiserver v0.16.6
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.16.6
	k8s.io/client-go => k8s.io/client-go v0.16.6
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.16.6
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.16.6
	k8s.io/code-generator => k8s.io/code-generator v0.16.6
	k8s.io/component-base => k8s.io/component-base v0.16.6
	k8s.io/cri-api => k8s.io/cri-api v0.16.6
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.16.6
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.16.6
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.16.6
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.16.6
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.16.6
	k8s.io/kubectl => k8s.io/kubectl v0.16.6
	k8s.io/kubelet => k8s.io/kubelet v0.16.6
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.16.6
	k8s.io/metrics => k8s.io/metrics v0.16.6
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.16.6

	github.com/go-bindata/go-bindata => github.com/go-bindata/go-bindata v3.1.1+incompatible
)

replace github.com/prometheus/client_golang => github.com/prometheus/client_golang v0.9.2

module github.com/openkruise/kruise

go 1.15

require (
	github.com/alibaba/pouch v0.0.0-20190328125340-37051654f368
	github.com/appscode/jsonpatch v1.0.1
	github.com/codegangsta/negroni v1.0.0
	github.com/containerd/containerd v1.5.8
	github.com/contiv/executor v0.0.0-20180626233236-d263f4daa3ad // indirect
	github.com/docker/distribution v2.7.2-0.20200708230840-70e0022e42fd+incompatible
	github.com/docker/docker v17.12.0-ce-rc1.0.20200916142827-bd33bbf0497b+incompatible
	github.com/fsnotify/fsnotify v1.4.9
	github.com/go-bindata/go-bindata v3.1.2+incompatible
	github.com/go-openapi/spec v0.19.3
	github.com/gorilla/mux v1.8.0
	github.com/onsi/ginkgo v1.14.1
	github.com/onsi/gomega v1.10.3
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.0.1
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.7.1
	github.com/robfig/cron v1.2.0
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.6.1
	github.com/xyproto/simpleredis v0.0.0-20200201215242-1ff0da2967b4
	golang.org/x/net v0.0.0-20210226172049-e18ecbb05110
	golang.org/x/time v0.0.0-20200630173020-3af7569d3a1e
	gomodules.xyz/jsonpatch/v2 v2.1.0
	google.golang.org/grpc v1.33.2
	k8s.io/api v0.20.10
	k8s.io/apiextensions-apiserver v0.20.10
	k8s.io/apimachinery v0.20.10
	k8s.io/apiserver v0.20.10
	k8s.io/client-go v0.20.10
	k8s.io/code-generator v0.20.10
	k8s.io/component-base v0.20.10
	k8s.io/component-helpers v0.20.10
	k8s.io/cri-api v0.20.10
	k8s.io/klog/v2 v2.4.0
	k8s.io/kube-openapi v0.0.0-20201113171705-d219536bb9fd
	k8s.io/kubernetes v1.20.10
	k8s.io/utils v0.0.0-20210111153108-fddb29f9d009
	sigs.k8s.io/controller-runtime v0.8.3
)

// Replace to match K8s 1.20.10
replace (
	k8s.io/api => k8s.io/api v0.20.10
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.20.10
	k8s.io/apimachinery => k8s.io/apimachinery v0.20.10
	k8s.io/apiserver => k8s.io/apiserver v0.20.10
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.20.10
	k8s.io/client-go => k8s.io/client-go v0.20.10
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.20.10
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.20.10
	k8s.io/code-generator => k8s.io/code-generator v0.20.10
	k8s.io/component-base => k8s.io/component-base v0.20.10
	k8s.io/component-helpers => k8s.io/component-helpers v0.20.10
	k8s.io/controller-manager => k8s.io/controller-manager v0.20.10
	k8s.io/cri-api => k8s.io/cri-api v0.20.10
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.20.10
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.20.10
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.20.10
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.20.10
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.20.10
	k8s.io/kubectl => k8s.io/kubectl v0.20.10
	k8s.io/kubelet => k8s.io/kubelet v0.20.10
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.20.10
	k8s.io/metrics => k8s.io/metrics v0.20.10
	k8s.io/mount-utils => k8s.io/mount-utils v0.20.10
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.20.10
)

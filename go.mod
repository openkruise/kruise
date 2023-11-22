module github.com/openkruise/kruise

go 1.19

require (
	github.com/alibaba/pouch v0.0.0-20190328125340-37051654f368
	github.com/appscode/jsonpatch v1.0.1
	github.com/codegangsta/negroni v1.0.0
	github.com/containerd/containerd v1.5.16
	github.com/docker/distribution v2.8.2+incompatible
	github.com/docker/docker v24.0.0+incompatible
	github.com/evanphx/json-patch v4.12.0+incompatible
	github.com/fsnotify/fsnotify v1.5.1
	github.com/go-bindata/go-bindata v3.1.2+incompatible
	github.com/google/go-containerregistry v0.16.1
	github.com/gorilla/mux v1.8.0
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/gomega v1.19.0
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.1.0-rc3
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.12.1
	github.com/robfig/cron/v3 v3.0.1
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.8.1
	github.com/xyproto/simpleredis v0.0.0-20200201215242-1ff0da2967b4
	golang.org/x/net v0.10.0
	golang.org/x/time v0.0.0-20220210224613-90d013bbcef8
	gomodules.xyz/jsonpatch/v2 v2.2.0
	google.golang.org/grpc v1.40.0
	k8s.io/api v0.24.16
	k8s.io/apiextensions-apiserver v0.24.2
	k8s.io/apimachinery v0.27.4
	k8s.io/apiserver v0.24.16
	k8s.io/client-go v0.27.4
	k8s.io/code-generator v0.24.16
	k8s.io/component-base v0.27.4
	k8s.io/component-helpers v0.24.16
	k8s.io/cri-api v0.24.16
	k8s.io/gengo v0.0.0-20211129171323-c02415ce4185
	k8s.io/klog/v2 v2.90.1
	k8s.io/kube-openapi v0.0.0-20230501164219-8b0f38b5fd1f
	k8s.io/kubernetes v1.24.0
	k8s.io/utils v0.0.0-20230209194617-a36077c30491
	sigs.k8s.io/controller-runtime v0.11.0
)

require (
	cloud.google.com/go v0.81.0 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20210617225240-d185dfc1b5a1 // indirect
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/Microsoft/hcsshim v0.8.24 // indirect
	github.com/asaskevich/govalidator v0.0.0-20200907205600-7a23bdc65eef // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/containerd/cgroups v1.0.3 // indirect
	github.com/containerd/console v1.0.3 // indirect
	github.com/containerd/continuity v0.3.0 // indirect
	github.com/containerd/fifo v1.0.0 // indirect
	github.com/containerd/stargz-snapshotter/estargz v0.14.3 // indirect
	github.com/containerd/ttrpc v1.1.0 // indirect
	github.com/containerd/typeurl v1.0.2 // indirect
	github.com/contiv/executor v0.0.0-20180626233236-d263f4daa3ad // indirect
	github.com/coreos/go-semver v0.3.0 // indirect
	github.com/coreos/go-systemd/v22 v22.3.2 // indirect
	github.com/cyphar/filepath-securejoin v0.2.3 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/docker/cli v24.0.0+incompatible // indirect
	github.com/docker/docker-credential-helpers v0.7.0 // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-events v0.0.0-20190806004212-e31b211e4f1c // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/emicklei/go-restful v2.9.5+incompatible // indirect
	github.com/go-logr/logr v1.2.3 // indirect
	github.com/go-openapi/analysis v0.21.2 // indirect
	github.com/go-openapi/errors v0.20.2 // indirect
	github.com/go-openapi/jsonpointer v0.19.6 // indirect
	github.com/go-openapi/jsonreference v0.20.1 // indirect
	github.com/go-openapi/loads v0.21.1 // indirect
	github.com/go-openapi/spec v0.20.4 // indirect
	github.com/go-openapi/strfmt v0.21.2 // indirect
	github.com/go-openapi/swag v0.22.3 // indirect
	github.com/go-openapi/validate v0.21.0 // indirect
	github.com/go-stack/stack v1.8.0 // indirect
	github.com/gogo/googleapis v1.4.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/gomodule/redigo v2.0.0+incompatible // indirect
	github.com/google/gnostic v0.5.7-v3refs // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/google/gofuzz v1.1.0 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/imdario/mergo v0.3.12 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/compress v1.16.5 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/mapstructure v1.4.1 // indirect
	github.com/moby/locker v1.0.1 // indirect
	github.com/moby/spdystream v0.2.0 // indirect
	github.com/moby/sys/mountinfo v0.6.0 // indirect
	github.com/moby/term v0.0.0-20221205130635-1aeaba878587 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/nxadm/tail v1.4.8 // indirect
	github.com/oklog/ulid v1.3.1 // indirect
	github.com/opencontainers/runc v1.1.6 // indirect
	github.com/opencontainers/runtime-spec v1.0.3-0.20210326190908-1c3f411f0417 // indirect
	github.com/opencontainers/selinux v1.10.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_model v0.2.0 // indirect
	github.com/prometheus/common v0.32.1 // indirect
	github.com/prometheus/procfs v0.7.3 // indirect
	github.com/sirupsen/logrus v1.9.1 // indirect
	github.com/vbatts/tar-split v0.11.3 // indirect
	github.com/xyproto/pinterface v0.0.0-20200201214933-70763765f31f // indirect
	go.etcd.io/etcd/api/v3 v3.5.1 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.5.1 // indirect
	go.etcd.io/etcd/client/v3 v3.5.1 // indirect
	go.mongodb.org/mongo-driver v1.7.5 // indirect
	go.opencensus.io v0.23.0 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	go.uber.org/multierr v1.6.0 // indirect
	go.uber.org/zap v1.19.1 // indirect
	golang.org/x/mod v0.10.0 // indirect
	golang.org/x/oauth2 v0.8.0 // indirect
	golang.org/x/sync v0.2.0 // indirect
	golang.org/x/sys v0.8.0 // indirect
	golang.org/x/term v0.8.0 // indirect
	golang.org/x/text v0.9.0 // indirect
	golang.org/x/tools v0.9.1 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20220107163113-42d7afdf6368 // indirect
	google.golang.org/protobuf v1.30.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/cloud-provider v0.24.16 // indirect
	k8s.io/csi-translation-lib v0.24.16 // indirect
	k8s.io/kube-scheduler v0.0.0 // indirect
	k8s.io/mount-utils v0.24.16 // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect
	sigs.k8s.io/yaml v1.3.0 // indirect
)

// Replace to match K8s 1.24.16
replace (
	k8s.io/api => k8s.io/api v0.24.16
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.24.16
	k8s.io/apimachinery => k8s.io/apimachinery v0.24.16
	k8s.io/apiserver => k8s.io/apiserver v0.24.16
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.24.16
	k8s.io/client-go => k8s.io/client-go v0.24.16
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.24.16
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.24.16
	k8s.io/code-generator => k8s.io/code-generator v0.22.6
	k8s.io/component-base => k8s.io/component-base v0.24.16
	k8s.io/component-helpers => k8s.io/component-helpers v0.24.16
	k8s.io/controller-manager => k8s.io/controller-manager v0.24.16
	k8s.io/cri-api => k8s.io/cri-api v0.24.16
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.24.16
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.24.16
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.24.16
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20220328201542-3ee0da9b0b42
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.24.16
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.24.16
	k8s.io/kubectl => k8s.io/kubectl v0.24.16
	k8s.io/kubelet => k8s.io/kubelet v0.24.16
	k8s.io/kubernetes => k8s.io/kubernetes v1.24.16
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.24.16
	k8s.io/metrics => k8s.io/metrics v0.24.16
	k8s.io/mount-utils => k8s.io/mount-utils v0.24.16
	k8s.io/pod-security-admission => k8s.io/pod-security-admission v0.24.16
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.24.16
	sigs.k8s.io/controller-runtime => sigs.k8s.io/controller-runtime v0.12.3
)

# new project
kubebuilder create api --group apps --version v1alpha1 --kind DaemonSet

# update client
vendor/k8s.io/code-generator/generate-groups.sh all github.com/openkruise/kruise/pkg/client github.com/openkruise/kruise/pkg/apis apps:v1alpha1

# update webhook
kubebuilder alpha webhook --group apps --version v1alpha1 --kind DaemonSet --type=mutating --operations=create,update
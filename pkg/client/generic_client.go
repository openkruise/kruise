package client

import (
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	kubeclientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// GenericClientset defines a generic client
type GenericClientset struct {
	KubeClient   kubeclientset.Interface
	KruiseClient kruiseclientset.Interface
}

// NewForConfig creates a new Clientset for the given config.
func newForConfig(c *rest.Config) (*GenericClientset, error) {
	kubeClient, err := kubeclientset.NewForConfig(c)
	if err != nil {
		return nil, err
	}
	kruiseClient, err := kruiseclientset.NewForConfig(c)
	if err != nil {
		return nil, err
	}
	return &GenericClientset{
		KubeClient:   kubeClient,
		KruiseClient: kruiseClient,
	}, nil
}

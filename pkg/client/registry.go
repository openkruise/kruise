package client

import (
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var (
	genericClient *GenericClientset
)

// NewRegistry creates clientset by client-go
func NewRegistry(mgr manager.Manager) error {
	var err error
	genericClient, err = newForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}
	return nil
}

// GetGenericClient returns clientset
func GetGenericClient() GenericClientset {
	return *genericClient
}

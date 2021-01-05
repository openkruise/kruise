package client

import (
	"k8s.io/client-go/rest"
)

var (
	genericClient *GenericClientset
)

// NewRegistry creates clientset by client-go
func NewRegistry(cfg *rest.Config) error {
	var err error
	genericClient, err = newForConfig(cfg)
	if err != nil {
		return err
	}
	return nil
}

// GetGenericClient returns clientset
func GetGenericClient() *GenericClientset {
	return genericClient
}

/*
Copyright 2019 The Kruise Authors.
Copyright 2018 The Kubernetes Authors.

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

package framework

import (
	"fmt"
	"os"
	"sync"

	v1 "k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"
)

// Factory represents the factory pattern
type Factory func() (ProviderInterface, error)

var (
	providers = make(map[string]Factory)
	mutex     sync.Mutex
)

// RegisterProvider is expected to be called during application init,
// typically by an init function in a provider package.
func RegisterProvider(name string, factory Factory) {
	mutex.Lock()
	defer mutex.Unlock()
	if _, ok := providers[name]; ok {
		panic(fmt.Sprintf("provider %s already registered", name))
	}
	providers[name] = factory
}

func init() {
	// "local" or "skeleton" can always be used.
	RegisterProvider("local", func() (ProviderInterface, error) {
		return NullProvider{}, nil
	})
	RegisterProvider("skeleton", func() (ProviderInterface, error) {
		return NullProvider{}, nil
	})
	// The empty string also works, but triggers a warning.
	RegisterProvider("", func() (ProviderInterface, error) {
		Logf("The --provider flag is not set.  Treating as a conformance test.  Some tests may not be run.")
		return NullProvider{}, nil
	})
}

// SetupProviderConfig validates the chosen provider and creates
// an interface instance for it.
func SetupProviderConfig(providerName string) (ProviderInterface, error) {
	var err error

	mutex.Lock()
	defer mutex.Unlock()
	factory, ok := providers[providerName]
	if !ok {
		return nil, fmt.Errorf("The provider %s is unknown.: %w", providerName, os.ErrNotExist)
	}
	provider, err := factory()

	return provider, err
}

// ProviderInterface contains the implementation for certain
// provider-specific functionality.
type ProviderInterface interface {
	FrameworkBeforeEach(f *Framework)
	FrameworkAfterEach(f *Framework)

	ResizeGroup(group string, size int32) error
	GetGroupNodes(group string) ([]string, error)
	GroupSize(group string) (int, error)

	CreatePD(zone string) (string, error)
	DeletePD(pdName string) error
	CreatePVSource(zone, diskName string) (*v1.PersistentVolumeSource, error)
	DeletePVSource(pvSource *v1.PersistentVolumeSource) error

	CleanupServiceResources(c clientset.Interface, loadBalancerName, region, zone string)

	EnsureLoadBalancerResourcesDeleted(ip, portRange string) error
	LoadBalancerSrcRanges() []string
	EnableAndDisableInternalLB() (enable, disable func(svc *v1.Service))
}

// NullProvider is the default implementation of the ProviderInterface
// which doesn't do anything.
type NullProvider struct{}

// FrameworkBeforeEach is a framework before each
func (n NullProvider) FrameworkBeforeEach(f *Framework) {}

// FrameworkAfterEach is framework after each
func (n NullProvider) FrameworkAfterEach(f *Framework) {}

// ResizeGroup no usages
func (n NullProvider) ResizeGroup(string, int32) error {
	return fmt.Errorf("provider does not support InstanceGroups")
}

// GetGroupNodes no usages
func (n NullProvider) GetGroupNodes(group string) ([]string, error) {
	return nil, fmt.Errorf("provider does not support InstanceGroups")
}

// GroupSize no usages
func (n NullProvider) GroupSize(group string) (int, error) {
	return -1, fmt.Errorf("provider does not support InstanceGroups")
}

// CreatePD no usages
func (n NullProvider) CreatePD(zone string) (string, error) {
	return "", fmt.Errorf("provider does not support volume creation")
}

// DeletePD no usages
func (n NullProvider) DeletePD(pdName string) error {
	return fmt.Errorf("provider does not support volume deletion")
}

// CreatePVSource no usages
func (n NullProvider) CreatePVSource(zone, diskName string) (*v1.PersistentVolumeSource, error) {
	return nil, fmt.Errorf("Provider not supported")
}

// DeletePVSource no usages
func (n NullProvider) DeletePVSource(pvSource *v1.PersistentVolumeSource) error {
	return fmt.Errorf("Provider not supported")
}

// CleanupServiceResources no usages
func (n NullProvider) CleanupServiceResources(c clientset.Interface, loadBalancerName, region, zone string) {
}

// EnsureLoadBalancerResourcesDeleted no usages
func (n NullProvider) EnsureLoadBalancerResourcesDeleted(ip, portRange string) error {
	return nil
}

// LoadBalancerSrcRanges no usages
func (n NullProvider) LoadBalancerSrcRanges() []string {
	return nil
}

// EnableAndDisableInternalLB no usages
func (n NullProvider) EnableAndDisableInternalLB() (enable, disable func(svc *v1.Service)) {
	nop := func(svc *v1.Service) {}
	return nop, nop
}

var _ ProviderInterface = NullProvider{}

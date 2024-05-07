/*
Copyright 2021 The Kruise Authors.

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
// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	"context"

	v1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeWorkloadSpreads implements WorkloadSpreadInterface
type FakeWorkloadSpreads struct {
	Fake *FakeAppsV1alpha1
	ns   string
}

var workloadspreadsResource = v1alpha1.SchemeGroupVersion.WithResource("workloadspreads")

var workloadspreadsKind = v1alpha1.SchemeGroupVersion.WithKind("WorkloadSpread")

// Get takes name of the workloadSpread, and returns the corresponding workloadSpread object, and an error if there is any.
func (c *FakeWorkloadSpreads) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.WorkloadSpread, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(workloadspreadsResource, c.ns, name), &v1alpha1.WorkloadSpread{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.WorkloadSpread), err
}

// List takes label and field selectors, and returns the list of WorkloadSpreads that match those selectors.
func (c *FakeWorkloadSpreads) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha1.WorkloadSpreadList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(workloadspreadsResource, workloadspreadsKind, c.ns, opts), &v1alpha1.WorkloadSpreadList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.WorkloadSpreadList{ListMeta: obj.(*v1alpha1.WorkloadSpreadList).ListMeta}
	for _, item := range obj.(*v1alpha1.WorkloadSpreadList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested workloadSpreads.
func (c *FakeWorkloadSpreads) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(workloadspreadsResource, c.ns, opts))

}

// Create takes the representation of a workloadSpread and creates it.  Returns the server's representation of the workloadSpread, and an error, if there is any.
func (c *FakeWorkloadSpreads) Create(ctx context.Context, workloadSpread *v1alpha1.WorkloadSpread, opts v1.CreateOptions) (result *v1alpha1.WorkloadSpread, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(workloadspreadsResource, c.ns, workloadSpread), &v1alpha1.WorkloadSpread{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.WorkloadSpread), err
}

// Update takes the representation of a workloadSpread and updates it. Returns the server's representation of the workloadSpread, and an error, if there is any.
func (c *FakeWorkloadSpreads) Update(ctx context.Context, workloadSpread *v1alpha1.WorkloadSpread, opts v1.UpdateOptions) (result *v1alpha1.WorkloadSpread, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(workloadspreadsResource, c.ns, workloadSpread), &v1alpha1.WorkloadSpread{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.WorkloadSpread), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeWorkloadSpreads) UpdateStatus(ctx context.Context, workloadSpread *v1alpha1.WorkloadSpread, opts v1.UpdateOptions) (*v1alpha1.WorkloadSpread, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(workloadspreadsResource, "status", c.ns, workloadSpread), &v1alpha1.WorkloadSpread{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.WorkloadSpread), err
}

// Delete takes name of the workloadSpread and deletes it. Returns an error if one occurs.
func (c *FakeWorkloadSpreads) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteActionWithOptions(workloadspreadsResource, c.ns, name, opts), &v1alpha1.WorkloadSpread{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeWorkloadSpreads) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(workloadspreadsResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &v1alpha1.WorkloadSpreadList{})
	return err
}

// Patch applies the patch and returns the patched workloadSpread.
func (c *FakeWorkloadSpreads) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.WorkloadSpread, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(workloadspreadsResource, c.ns, name, pt, data, subresources...), &v1alpha1.WorkloadSpread{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.WorkloadSpread), err
}

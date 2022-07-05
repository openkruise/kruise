/*
Copyright 2022 The Kruise Authors.

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

package client

import (
	"context"
	"flag"

	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	disableNoDeepCopy bool
)

func init() {
	flag.BoolVar(&disableNoDeepCopy, "disable-no-deepcopy", false, "If you are going to disable NoDeepCopy List in some controllers and webhooks.")
}

type internalCache struct {
	cache.Cache
	noDeepCopyLister *noDeepCopyLister
}

func NewCache(config *rest.Config, opts cache.Options) (cache.Cache, error) {
	if opts.Scheme == nil {
		opts.Scheme = clientgoscheme.Scheme
	}
	c, err := cache.New(config, opts)
	if err != nil {
		return nil, err
	}
	return &internalCache{
		Cache:            c,
		noDeepCopyLister: &noDeepCopyLister{cache: c, scheme: opts.Scheme},
	}, nil
}

func (ic *internalCache) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if !disableNoDeepCopy && isDisableDeepCopy(opts) {
		return ic.noDeepCopyLister.List(ctx, list, opts...)
	}
	return ic.Cache.List(ctx, list, opts...)
}

var DisableDeepCopy = disableDeepCopy{}

type disableDeepCopy struct{}

func (disableDeepCopy) ApplyToList(*client.ListOptions) {
}

func isDisableDeepCopy(opts []client.ListOption) bool {
	for _, opt := range opts {
		if opt == DisableDeepCopy {
			return true
		}
	}
	return false
}

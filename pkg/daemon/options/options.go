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

package options

import (
	daemonruntime "github.com/openkruise/kruise/pkg/daemon/criruntime"
	daemonutil "github.com/openkruise/kruise/pkg/daemon/util"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Options struct {
	NodeName      string
	Scheme        *runtime.Scheme
	RuntimeClient runtimeclient.Client
	PodInformer   cache.SharedIndexInformer

	RuntimeFactory daemonruntime.Factory
	Healthz        *daemonutil.Healthz
}

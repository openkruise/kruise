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

package containermeta

import (
	"encoding/json"
	"hash/fnv"
	"sort"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/kubernetes/pkg/fieldpath"
	hashutil "k8s.io/kubernetes/pkg/util/hash"
)

var (
	excludeEnvs = sets.NewString(
		"SIDECARSET_VERSION",
		"SIDECARSET_VERSION_ALT",
	)
)

type EnvGetter func(key string) (string, error)

type EnvFromMetadataHasher interface {
	GetExpectHash(c *v1.Container, objMeta metav1.Object) uint64
	GetCurrentHash(c *v1.Container, getter EnvGetter) (uint64, error)
}

type envFromMetadataHasher struct{}

func NewEnvFromMetadataHasher() EnvFromMetadataHasher {
	return &envFromMetadataHasher{}
}

func (h *envFromMetadataHasher) GetExpectHash(c *v1.Container, objMeta metav1.Object) uint64 {
	var envs []v1.EnvVar
	for i := range c.Env {
		if c.Env[i].Value != "" || c.Env[i].ValueFrom == nil || c.Env[i].ValueFrom.FieldRef == nil {
			continue
		} else if excludeEnvs.Has(c.Env[i].Name) {
			continue
		}

		// Currently only supports `metadata.labels['<KEY>']`, `metadata.annotations['<KEY>']`
		path, subscript, ok := fieldpath.SplitMaybeSubscriptedPath(c.Env[i].ValueFrom.FieldRef.FieldPath)
		if !ok {
			continue
		}

		env := v1.EnvVar{Name: c.Env[i].Name}
		switch path {
		case "metadata.annotations":
			env.Value = objMeta.GetAnnotations()[subscript]
		case "metadata.labels":
			env.Value = objMeta.GetLabels()[subscript]
		default:
			continue
		}

		envs = append(envs, env)
	}

	sort.SliceStable(envs, func(i, j int) bool {
		return envs[i].Name < envs[j].Name
	})
	return hashEnvs(envs)
}

func (h *envFromMetadataHasher) GetCurrentHash(c *v1.Container, getter EnvGetter) (uint64, error) {
	var envs []v1.EnvVar
	for i := range c.Env {
		if c.Env[i].Value != "" || c.Env[i].ValueFrom == nil || c.Env[i].ValueFrom.FieldRef == nil {
			continue
		} else if excludeEnvs.Has(c.Env[i].Name) {
			continue
		}

		// Currently only supports `metadata.labels['<KEY>']`, `metadata.annotations['<KEY>']`
		path, _, ok := fieldpath.SplitMaybeSubscriptedPath(c.Env[i].ValueFrom.FieldRef.FieldPath)
		if !ok {
			continue
		}

		var err error
		env := v1.EnvVar{Name: c.Env[i].Name}
		switch path {
		case "metadata.annotations", "metadata.labels":
			env.Value, err = getter(c.Env[i].Name)
			if err != nil {
				return 0, err
			}
		default:
			continue
		}

		envs = append(envs, env)
	}

	sort.SliceStable(envs, func(i, j int) bool {
		return envs[i].Name < envs[j].Name
	})
	return hashEnvs(envs), nil
}

func hashEnvs(envs []v1.EnvVar) uint64 {
	hash := fnv.New32a()
	envsJSON, _ := json.Marshal(envs)
	hashutil.DeepHashObject(hash, envsJSON)
	return uint64(hash.Sum32())
}

func IsContainerReferenceToMeta(c *v1.Container, path, key string) bool {
	for i := range c.Env {
		if c.Env[i].Value != "" || c.Env[i].ValueFrom == nil || c.Env[i].ValueFrom.FieldRef == nil {
			continue
		} else if excludeEnvs.Has(c.Env[i].Name) {
			continue
		}

		reqPath, subscript, ok := fieldpath.SplitMaybeSubscriptedPath(c.Env[i].ValueFrom.FieldRef.FieldPath)
		if !ok {
			continue
		}

		if reqPath == path && subscript == key {
			return true
		}
	}
	return false
}

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

package configuration

import (
	"context"
	"encoding/json"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/openkruise/kruise/pkg/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// kruise configmap name
	KruiseConfigurationName = "kruise-configuration"
)

func GetSidecarSetPatchMetadataWhiteList(client client.Client) (*SidecarSetPatchMetadataWhiteList, error) {
	data, err := getKruiseConfiguration(client)
	if err != nil {
		return nil, err
	} else if len(data) == 0 {
		return nil, nil
	}
	value, ok := data[SidecarSetPatchPodMetadataWhiteListKey]
	if !ok {
		return nil, nil
	}
	whiteList := &SidecarSetPatchMetadataWhiteList{}
	if err = json.Unmarshal([]byte(value), whiteList); err != nil {
		return nil, err
	}
	return whiteList, nil
}

func GetPPSWatchCustomWorkloadWhiteList(client client.Client) (*CustomWorkloadWhiteList, error) {
	whiteList := &CustomWorkloadWhiteList{Workloads: make([]schema.GroupVersionKind, 0)}
	data, err := getKruiseConfiguration(client)
	if err != nil {
		return nil, err
	} else if len(data) == 0 {
		return whiteList, nil
	}
	value, ok := data[PPSWatchCustomWorkloadWhiteList]
	if !ok {
		return whiteList, nil
	}
	if err = json.Unmarshal([]byte(value), whiteList); err != nil {
		return nil, err
	}
	return whiteList, nil
}

func GetWSWatchCustomWorkloadWhiteList(client client.Reader) (WSCustomWorkloadWhiteList, error) {
	whiteList := WSCustomWorkloadWhiteList{}
	data, err := getKruiseConfiguration(client)
	if err != nil {
		return whiteList, err
	} else if len(data) == 0 {
		return whiteList, nil
	}
	value, ok := data[WSWatchCustomWorkloadWhiteList]
	if !ok {
		return whiteList, nil
	}
	if err = json.Unmarshal([]byte(value), &whiteList); err != nil {
		return whiteList, err
	}
	return whiteList, nil
}

func getKruiseConfiguration(c client.Reader) (map[string]string, error) {
	cfg := &corev1.ConfigMap{}
	err := c.Get(context.TODO(), client.ObjectKey{Namespace: util.GetKruiseNamespace(), Name: KruiseConfigurationName}, cfg)
	if err != nil {
		if errors.IsNotFound(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	return cfg.Data, nil
}

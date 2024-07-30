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

package util

import (
	"context"
	"math/rand"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// SecretManager is the interface to get secrets from API Server.
type SecretManager interface {
	GetSecrets(secret []appsv1alpha1.ReferenceObject) ([]v1.Secret, error)
}

// NewCacheBasedSecretManager create a cache based SecretManager
func NewCacheBasedSecretManager(client clientset.Interface) SecretManager {
	return &cacheBasedSecretManager{
		client: client,
		cache:  make(map[appsv1alpha1.ReferenceObject]cacheSecretItem),
	}
}

type cacheBasedSecretManager struct {
	client clientset.Interface
	cache  map[appsv1alpha1.ReferenceObject]cacheSecretItem
}

type cacheSecretItem struct {
	deadline time.Time
	secret   *v1.Secret
}

func (c cacheSecretItem) isExpired() bool {
	return time.Now().After(c.deadline)
}

func (c *cacheBasedSecretManager) GetSecrets(secrets []appsv1alpha1.ReferenceObject) (ret []v1.Secret, err error) {
	for _, secret := range secrets {
		if item, ok := c.cache[secret]; ok && !item.isExpired() {
			ret = append(ret, *item.secret)
		} else {
			s, err := c.client.CoreV1().Secrets(secret.Namespace).Get(context.TODO(), secret.Name, metav1.GetOptions{ResourceVersion: "0"})
			if err != nil {
				klog.ErrorS(err, "failed to get secret", "secret", secret)
			} else {
				// renew cache in 5~10 minutes
				interval := time.Duration(rand.Int31n(6)+5) * time.Minute
				c.cache[secret] = cacheSecretItem{
					deadline: time.Now().Add(interval),
					secret:   s,
				}
				ret = append(ret, *s)
			}
		}
	}
	return ret, nil
}

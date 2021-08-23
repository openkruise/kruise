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
	"reflect"
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGetSecretsFromCache(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	secretManager := NewCacheBasedSecretManager(fakeClient)
	secretFoo := v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns-foo",
			Name:      "foo",
		},
		Type: v1.SecretTypeOpaque,
		Data: map[string][]byte{
			"data": []byte("foo"),
		},
	}

	secretBar := v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns-bar",
			Name:      "bar",
		},
		Type: v1.SecretTypeOpaque,
		Data: map[string][]byte{
			"data": []byte("bar"),
		},
	}

	if _, err := fakeClient.CoreV1().Secrets(secretFoo.Namespace).Create(context.TODO(), &secretFoo, metav1.CreateOptions{}); err != nil {
		t.Fatalf("failed to create fake secret: %v", err)
	}

	if _, err := fakeClient.CoreV1().Secrets(secretBar.Namespace).Create(context.TODO(), &secretBar, metav1.CreateOptions{}); err != nil {
		t.Fatalf("failed to create fake secret: %v", err)
	}

	secrets, err := secretManager.GetSecrets([]appsv1alpha1.ReferenceObject{{Namespace: "ns-bar", Name: "bar"}})
	if err != nil || len(secrets) != 1 {
		t.Errorf("failed to get secret: %v", err)
	}

	secrets, err = secretManager.GetSecrets([]appsv1alpha1.ReferenceObject{{Namespace: "ns-foo", Name: "foo"}})
	if err != nil || len(secrets) != 1 {
		t.Fatalf("failed to get secret: %v", err)
	}
	secretDataOld := secrets[0].Data

	secretFoo.Data = nil
	if _, err := fakeClient.CoreV1().Secrets(secretFoo.Namespace).Update(context.TODO(), &secretFoo, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("failed to update fake secret: %v", err)
	}
	secrets, err = secretManager.GetSecrets([]appsv1alpha1.ReferenceObject{{Namespace: "ns-foo", Name: "foo"}})
	if err != nil || len(secrets) != 1 {
		t.Fatalf("failed to get secret: %v", err)
	}
	if !reflect.DeepEqual(secretDataOld, secrets[0].Data) {
		t.Errorf("unexpected data change")
	}
}

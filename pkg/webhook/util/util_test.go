/*
Copyright 2025 The Kruise Authors.

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
	"os"
	"testing"
	"time"
)

func resetRenewBefore() {
	renewBefore = 0
}

func TestGetEnvDefaults(t *testing.T) {
	t.Setenv("WEBHOOK_HOST", "example.com")
	if v := GetHost(); v != "example.com" {
		t.Errorf("expect(example.com), but get(%s)", v)
	}
	t.Setenv("SECRET_NAME", "my-secret")
	if v := GetSecretName(); v != "my-secret" {
		t.Errorf("expect(my-secret), but get(%s)", v)
	}
	os.Unsetenv("SECRET_NAME")
	if v := GetSecretName(); v != "kruise-webhook-certs" {
		t.Errorf("GetSecretName() default = %s, want kruise-webhook-certs", v)
	}
	t.Setenv("SERVICE_NAME", "svc")
	if v := GetServiceName(); v != "svc" {
		t.Errorf("expect(svc), but get(%s)", v)
	}
	os.Unsetenv("SERVICE_NAME")
	if v := GetServiceName(); v != "kruise-webhook-service" {
		t.Errorf("GetServiceName() default = %s, want kruise-webhook-service", v)
	}
	t.Setenv("WEBHOOK_CERT_DIR", "/certs")
	if v := GetCertDir(); v != "/certs" {
		t.Errorf("expect(/certs), but get %s", v)
	}
	os.Unsetenv("WEBHOOK_CERT_DIR")
	if v := GetCertDir(); v != "/tmp/kruise-webhook-certs" {
		t.Errorf("GetCertDir() default = %s, want /tmp/kruise-webhook-certs", v)
	}

	t.Setenv("WEBHOOK_CERT_WRITER", "my-writer")
	if v := GetCertWriter(); v != "my-writer" {
		t.Errorf("expect(my-writer), but get %s", v)
	}
}

func TestGetPort(t *testing.T) {
	tests := []struct {
		env     string
		want    int
		wantErr bool
	}{
		{"9443", 9443, false},
		{"", 9876, false},
	}

	for _, tt := range tests {
		t.Run("port="+tt.env, func(t *testing.T) {
			if tt.env != "" {
				t.Setenv("WEBHOOK_PORT", tt.env)
			} else {
				os.Unsetenv("WEBHOOK_PORT")
			}
			if tt.wantErr {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("expected panic for input %q", tt.env)
					}
				}()
			}
			if !tt.wantErr {
				got := GetPort()
				if got != tt.want {
					t.Errorf("GetPort() = %d, want %d", got, tt.want)
				}
			} else {
				GetPort()
			}
		})
	}
}

func TestGetRenewBeforeTime(t *testing.T) {
	tests := []struct {
		name     string
		env      string
		expected time.Duration
	}{
		{"default", "", 6 * 30 * 24 * time.Hour},
		{"10d", "10d", 10 * 24 * time.Hour},
		{"2m", "2m", 2 * 30 * 24 * time.Hour},
		{"1y", "1y", 365 * 24 * time.Hour},
		{"invalid suffix", "5x", 6 * 30 * 24 * time.Hour},
		{"invalid format", "oops", 6 * 30 * 24 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetRenewBefore()
			if tt.env != "" {
				t.Setenv("CERTS_RENEW_BEFORE", tt.env)
			} else {
				os.Unsetenv("CERTS_RENEW_BEFORE")
			}
			got := GetRenewBeforeTime()
			if got != tt.expected {
				t.Errorf("GetRenewBeforeTime() = %v, want %v", got, tt.expected)
			}
		})
	}
}

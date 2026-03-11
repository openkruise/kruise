/*
Copyright 2020 The Kruise Authors.

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

package health

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestTryInitializeSuccess tests successful initialization
func TestTryInitializeSuccess(t *testing.T) {
	// Reset global state
	resetHealthChecker()

	// Create a temporary CA cert file
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "ca-cert.pem")
	testCert := []byte(`-----BEGIN CERTIFICATE-----
MIIC+DCCAeCgAwIBAgIJAKkZq3J2RKLlMA0GCSqGSIb3DQEBCwUAMBMxETAPBgNV
BAMMCGt1YmUtYXBpMB4XDTE5MDEwMTAwMDAwMFoXDTI5MDEwMTAwMDAwMFowEzER
MA8GA1UEAwwIa3ViZS1hcGkwggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIB
AQDHlZjp0nA3Y8HbAqw6LLe7y0qbWkKx5FwKRqrP8PUJ8+A9wRSNPLk+Ju9Q/Pjz
-----END CERTIFICATE-----`)
	if err := os.WriteFile(certPath, testCert, 0644); err != nil {
		t.Fatalf("Failed to create test cert: %v", err)
	}

	// Override cert path
	oldCertPath := caCertFilePath
	caCertFilePath = certPath
	defer func() { caCertFilePath = oldCertPath }()

	// Test initialization
	err := tryInitialize()
	if err != nil {
		t.Errorf("Expected successful initialization, got error: %v", err)
	}

	// Verify state
	if state := atomic.LoadInt32(&initState); state != 2 {
		t.Errorf("Expected initState=2 (success), got %d", state)
	}

	// Test idempotency - should return immediately
	err = tryInitialize()
	if err != nil {
		t.Errorf("Second initialization should succeed: %v", err)
	}
}

// TestTryInitializeFailCACertMissing tests initialization failure when CA cert is missing
func TestTryInitializeFailCACertMissing(t *testing.T) {
	resetHealthChecker()

	// Point to non-existent cert
	oldCertPath := caCertFilePath
	caCertFilePath = "/nonexistent/path/ca-cert.pem"
	defer func() { caCertFilePath = oldCertPath }()

	err := tryInitialize()
	if err == nil {
		t.Error("Expected error for missing CA cert, got nil")
	}

	// Verify failed state
	if state := atomic.LoadInt32(&initState); state != 3 {
		t.Errorf("Expected initState=3 (failed), got %d", state)
	}

	if initErr == nil {
		t.Error("Expected initErr to be set")
	}
}

// TestCheckerFailsWhenNotInitialized tests that Checker returns error when initialization fails
func TestCheckerFailsWhenNotInitialized(t *testing.T) {
	resetHealthChecker()

	// Point to non-existent cert
	oldCertPath := caCertFilePath
	caCertFilePath = "/nonexistent/path/ca-cert.pem"
	defer func() { caCertFilePath = oldCertPath }()

	err := Checker(nil)
	if err == nil {
		t.Error("Checker should fail when initialization fails")
	}

	// Verify error message contains initialization failure
	if err != nil && err.Error() == "" {
		t.Error("Error should have descriptive message")
	}
}

// TestConcurrentInitialization tests that concurrent calls to tryInitialize are safe
func TestConcurrentInitialization(t *testing.T) {
	resetHealthChecker()

	// Create a temporary CA cert file
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "ca-cert.pem")
	testCert := []byte(`-----BEGIN CERTIFICATE-----
MIIC+DCCAeCgAwIBAgIJAKkZq3J2RKLlMA0GCSqGSIb3DQEBCwUAMBMxETAPBgNV
BAMMCGt1YmUtYXBpMB4XDTE5MDEwMTAwMDAwMFoXDTI5MDEwMTAwMDAwMFowEzER
MA8GA1UEAwwIa3ViZS1hcGkwggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIB
AQDHlZjp0nA3Y8HbAqw6LLe7y0qbWkKx5FwKRqrP8PUJ8+A9wRSNPLk+Ju9Q/Pjz
-----END CERTIFICATE-----`)
	if err := os.WriteFile(certPath, testCert, 0644); err != nil {
		t.Fatalf("Failed to create test cert: %v", err)
	}

	oldCertPath := caCertFilePath
	caCertFilePath = certPath
	defer func() { caCertFilePath = oldCertPath }()

	// Launch multiple goroutines
	const numGoroutines = 10
	var wg sync.WaitGroup
	errChan := make(chan error, numGoroutines)

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			if err := tryInitialize(); err != nil {
				errChan <- err
			}
		}()
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		// "initialization in progress" is acceptable
		if err.Error() != "initialization in progress by another goroutine" {
			t.Errorf("Unexpected error during concurrent init: %v", err)
		}
	}

	// Verify final state is success
	if state := atomic.LoadInt32(&initState); state != 2 {
		t.Errorf("Expected final initState=2, got %d", state)
	}
}

// TestInitStateTransitions tests that state transitions are correct
func TestInitStateTransitions(t *testing.T) {
	resetHealthChecker()

	// Initial state should be 0 (uninitialized)
	if state := atomic.LoadInt32(&initState); state != 0 {
		t.Errorf("Initial state should be 0, got %d", state)
	}

	// Cause a failure
	oldCertPath := caCertFilePath
	caCertFilePath = "/nonexistent/ca-cert.pem"
	defer func() { caCertFilePath = oldCertPath }()

	_ = tryInitialize()

	// Should be in failed state
	if state := atomic.LoadInt32(&initState); state != 3 {
		t.Errorf("After failure, state should be 3, got %d", state)
	}
}

// resetHealthChecker resets the package-level state for testing
func resetHealthChecker() {
	atomic.StoreInt32(&initState, 0)
	initLock = sync.Mutex{}
	initErr = nil
	lock = sync.Mutex{}
	client = nil
	if certWatcher != nil {
		certWatcher.Close()
		// Give goroutine time to exit
		time.Sleep(10 * time.Millisecond)
		certWatcher = nil
	}
}

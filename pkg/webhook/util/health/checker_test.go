package health

import (
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestChecker(t *testing.T) {
	// Start a dummy TLS test server
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	// Extract the port and set it as WEBHOOK_PORT
	port := ts.Listener.Addr().(*net.TCPAddr).Port
	os.Setenv("WEBHOOK_PORT", strconv.Itoa(port))
	defer os.Unsetenv("WEBHOOK_PORT")

	// create a temp dir for dummy cert
	tempDir, err := os.MkdirTemp("", "kruise-cert")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	cert := ts.Certificate()
	dummyCertPath := filepath.Join(tempDir, "ca-cert.pem")
	certOut, err := os.Create(dummyCertPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}); err != nil {
		t.Fatal(err)
	}
	certOut.Close()

	// Override caCertFilePath
	caCertFilePath = dummyCertPath

	// First call to trigger onceWatch
	Checker(nil)

	// Now client is initialized. Override Transport to skip verify so localhost cert from httptest works
	lock.Lock()
	if client != nil && client.Transport != nil {
		client.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify = true
	}
	lock.Unlock()

	// Second call should succeed, hitting `defer resp.Body.Close()`
	err = Checker(nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	
	// Wait a bit to ensure background watcher doesn't cause issues on exit
	time.Sleep(100 * time.Millisecond)
}

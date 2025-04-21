package podprobe

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
	"k8s.io/kubernetes/pkg/probe"
	httpprobe "k8s.io/kubernetes/pkg/probe/http"
	tcpprobe "k8s.io/kubernetes/pkg/probe/tcp"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
)

// New creates Prober.
func New() prober {
	return prober{
		tcp:  tcpprobe.New(),
		http: httpprobe.New(false),
	}
}

func TestRunProbe(t *testing.T) {
	// Setup a test server that responds to probing correctly
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	tHost, tPortStr, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	tPort, err := strconv.Atoi(tPortStr)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	tests := []struct {
		name                   string
		p                      *appsv1alpha1.ContainerProbeSpec
		probeKey               probeKey
		containerRuntimeStatus *runtimeapi.ContainerStatus
		containerID            string

		expectedStatus probe.Result
		expectedError  error
	}{
		{
			name: "test tcpProbe check, a connection is made and probing would succeed",
			p: &appsv1alpha1.ContainerProbeSpec{
				Probe: corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						TCPSocket: &corev1.TCPSocketAction{
							Port: intstr.IntOrString{Type: intstr.Int, IntVal: int32(tPort)},
						},
					},
				},
			},
			probeKey: probeKey{
				podIP: tHost,
			},
			containerRuntimeStatus: &runtimeapi.ContainerStatus{
				Metadata: &runtimeapi.ContainerMetadata{
					Name: "container-name",
				},
			},

			expectedStatus: probe.Success,
			expectedError:  nil,
		},

		{
			name: "test tcpProbe check, no connection can be made and probing would fail",
			p: &appsv1alpha1.ContainerProbeSpec{
				Probe: corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						TCPSocket: &corev1.TCPSocketAction{
							Port: intstr.IntOrString{Type: intstr.Int, IntVal: int32(-1)},
						},
					},
				},
			},
			probeKey: probeKey{
				podIP: tHost,
			},
			expectedStatus: probe.Failure,
			expectedError:  nil,
		},

		{
			name: "test httpProbe check, a connection is made and probing would succeed",
			p: &appsv1alpha1.ContainerProbeSpec{
				Probe: corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{
							Path: "/",
							Port: intstr.FromInt(tPort),
							Host: tHost,
						},
					},
				},
			},
			probeKey: probeKey{
				podIP: tHost,
			},
			expectedStatus: probe.Success,
			expectedError:  nil,
		},

		{
			name: "invalid probe handler",
			p: &appsv1alpha1.ContainerProbeSpec{
				Probe: corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{},
				},
			},
			containerRuntimeStatus: &runtimeapi.ContainerStatus{
				Metadata: &runtimeapi.ContainerMetadata{
					Name: "container-name",
				},
			},
			expectedStatus: probe.Unknown,
			expectedError:  fmt.Errorf("missing probe handler for %s", "container-name"),
		},
	}

	prober := New()
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, _, err := prober.runProbe(tt.p, tt.probeKey, tt.containerRuntimeStatus, tt.containerID)
			if status != tt.expectedStatus {
				t.Errorf("#%d: expected status=%v, get=%v", i, tt.expectedStatus, status)
			}
			if err != tt.expectedError {
				// Check if the error message contains the expected error message
				if err != nil && tt.expectedError != nil && err.Error() == tt.expectedError.Error() {
					return
				} else {
					t.Errorf("#%d: expected error=%v, get=%v", i, tt.expectedError, err)
				}
			}
		})
	}

}

func TestProbe(t *testing.T) {
	cases := []struct {
		name                   string
		p                      *appsv1alpha1.ContainerProbeSpec
		probeKey               probeKey
		containerRuntimeStatus *runtimeapi.ContainerStatus
		containerID            string

		expectProbeState appsv1alpha1.ProbeState
		expectStr        string
		expectErr        error
	}{
		{
			name: "Failed to find probe builder for container",
			p: &appsv1alpha1.ContainerProbeSpec{
				Probe: corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{
							Path:   "/index.html",
							Port:   intstr.IntOrString{Type: intstr.String, StrVal: "main-port"},
							Scheme: corev1.URISchemeHTTP,
						},
					},
					InitialDelaySeconds: 100,
				},
			},
			containerRuntimeStatus: &runtimeapi.ContainerStatus{
				Metadata: &runtimeapi.ContainerMetadata{
					Name: "c1",
				},
			},
			expectProbeState: appsv1alpha1.ProbeFailed,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := prober{}
			getProbeState, getStr, getErr := p.probe(tc.p, tc.probeKey, tc.containerRuntimeStatus, tc.containerID)
			if !reflect.DeepEqual(getProbeState, tc.expectProbeState) {
				t.Errorf("expect: %v, but: %v", util.DumpJSON(tc.expectProbeState), util.DumpJSON(getProbeState))
			}
			if !reflect.DeepEqual(getStr, tc.expectStr) {
				t.Errorf("expect: %v, but: %v", util.DumpJSON(tc.expectStr), util.DumpJSON(getStr))
			}
			if reflect.DeepEqual(getErr, tc.expectErr) {
				t.Errorf("expect: %v, but: %v", util.DumpJSON(tc.expectErr), util.DumpJSON(getErr))
			}
		})
	}

}

func TestFormatURL(t *testing.T) {
	testCases := []struct {
		scheme string
		host   string
		port   int
		path   string
		result string
	}{
		{"http", "localhost", 93, "", "http://localhost:93"},
		{"https", "localhost", 93, "/path", "https://localhost:93/path"},
		{"http", "localhost", 93, "?foo", "http://localhost:93?foo"},
		{"https", "localhost", 93, "/path?bar", "https://localhost:93/path?bar"},
		{"http", "localhost", 93, "/path#foo", "http://localhost:93/path#foo"},
	}
	for _, test := range testCases {
		url := formatURL(test.scheme, test.host, test.port, test.path)
		if url.String() != test.result {
			t.Errorf("Expected %s, got %s", test.result, url.String())
		}
	}
}

func Test_v1HeaderToHTTPHeader(t *testing.T) {
	tests := []struct {
		name       string
		headerList []corev1.HTTPHeader
		want       http.Header
	}{
		{
			name: "not empty input",
			headerList: []corev1.HTTPHeader{
				{Name: "Connection", Value: "Keep-Alive"},
				{Name: "Content-Type", Value: "text/html"},
				{Name: "Accept-Ranges", Value: "bytes"},
			},
			want: http.Header{
				"Connection":    {"Keep-Alive"},
				"Content-Type":  {"text/html"},
				"Accept-Ranges": {"bytes"},
			},
		},
		{
			name: "case insensitive",
			headerList: []corev1.HTTPHeader{
				{Name: "HOST", Value: "example.com"},
				{Name: "FOO-bAR", Value: "value"},
			},
			want: http.Header{
				"Host":    {"example.com"},
				"Foo-Bar": {"value"},
			},
		},
		{
			name:       "empty input",
			headerList: []corev1.HTTPHeader{},
			want:       http.Header{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := v1HeaderToHTTPHeader(tt.headerList); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("v1HeaderToHTTPHeader() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHeaderConversion(t *testing.T) {
	testCases := []struct {
		headers  []corev1.HTTPHeader
		expected http.Header
	}{
		{
			[]corev1.HTTPHeader{
				{
					Name:  "Accept",
					Value: "application/json",
				},
			},
			http.Header{
				"Accept": []string{"application/json"},
			},
		},
		{
			[]corev1.HTTPHeader{
				{Name: "accept", Value: "application/json"},
			},
			http.Header{
				"Accept": []string{"application/json"},
			},
		},
		{
			[]corev1.HTTPHeader{
				{Name: "accept", Value: "application/json"},
				{Name: "Accept", Value: "*/*"},
				{Name: "pragma", Value: "no-cache"},
				{Name: "X-forwarded-for", Value: "username"},
			},
			http.Header{
				"Accept":          []string{"application/json", "*/*"},
				"Pragma":          []string{"no-cache"},
				"X-Forwarded-For": []string{"username"},
			},
		},
	}
	for i, test := range testCases {
		t.Run(fmt.Sprintf("case-%d", i), func(t *testing.T) {
			headers := v1HeaderToHTTPHeader(test.headers)
			if !reflect.DeepEqual(headers, test.expected) {
				t.Errorf("Expected %v, got %v", test.expected, headers)
			}
		})
	}
}

func TestNewRequestForHTTPGetAction(t *testing.T) {
	tests := []struct {
		name              string
		httpGet           *corev1.HTTPGetAction
		podIP             string
		userAgentFragment string
		expectURL         string
		expectHeader      http.Header
		expectError       bool
	}{
		{
			name: "valid request with default scheme",
			httpGet: &corev1.HTTPGetAction{
				Port:   intstr.FromInt(8080),
				Path:   "/health",
				Host:   "localhost",
				Scheme: "",
				HTTPHeaders: []corev1.HTTPHeader{
					{Name: "X-Custom-Header", Value: "value"},
				},
			},
			podIP:             "192.168.1.1",
			userAgentFragment: "test",
			expectURL:         "http://localhost:8080/health",
			expectHeader: http.Header{
				"X-Custom-Header": {"value"},
				"User-Agent":      {userAgent("test")},
				"Accept":          {"*/*"},
			},
			expectError: false,
		},
		{
			name: "valid request with https scheme",
			httpGet: &corev1.HTTPGetAction{
				Port:   intstr.FromInt(8443),
				Path:   "/secure",
				Host:   "secure.example.com",
				Scheme: corev1.URISchemeHTTPS,
			},
			podIP:             "192.168.1.1",
			userAgentFragment: "test",
			expectURL:         "https://secure.example.com:8443/secure",
			expectHeader: http.Header{
				"User-Agent": {userAgent("test")},
				"Accept":     {"*/*"},
			},
			expectError: false,
		},
		{
			name: "default host to podIP when not provided",
			httpGet: &corev1.HTTPGetAction{
				Port:   intstr.FromInt(9090),
				Path:   "/metrics",
				Scheme: corev1.URISchemeHTTP,
			},
			podIP:             "10.0.0.1",
			userAgentFragment: "test",
			expectURL:         "http://10.0.0.1:9090/metrics",
			expectHeader: http.Header{
				"User-Agent": {userAgent("test")},
				"Accept":     {"*/*"},
			},
			expectError: false,
		},
		{
			name: "custom headers override default ones",
			httpGet: &corev1.HTTPGetAction{
				Port:   intstr.FromInt(8080),
				Path:   "/health",
				Host:   "localhost",
				Scheme: corev1.URISchemeHTTP,
				HTTPHeaders: []corev1.HTTPHeader{
					{Name: "Accept", Value: "application/json"},
					{Name: "User-Agent", Value: "custom-agent"},
				},
			},
			podIP:             "192.168.1.1",
			userAgentFragment: "test",
			expectURL:         "http://localhost:8080/health",
			expectHeader: http.Header{
				"Accept":     {"application/json"},
				"User-Agent": {"custom-agent"},
			},
			expectError: false,
		},
		{
			name: "invalid port - too low",
			httpGet: &corev1.HTTPGetAction{
				Port:   intstr.FromInt(-1),
				Path:   "/health",
				Host:   "localhost",
				Scheme: corev1.URISchemeHTTP,
			},
			podIP:             "192.168.1.1",
			userAgentFragment: "test",
			expectError:       true,
		},
		{
			name: "invalid port - too high",
			httpGet: &corev1.HTTPGetAction{
				Port:   intstr.FromInt(65536),
				Path:   "/health",
				Host:   "localhost",
				Scheme: corev1.URISchemeHTTP,
			},
			podIP:             "192.168.1.1",
			userAgentFragment: "test",
			expectError:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := newRequestForHTTPGetAction(tt.httpGet, tt.podIP, tt.userAgentFragment)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if req.URL.String() != tt.expectURL {
				t.Errorf("Expected URL %q but got %q", tt.expectURL, req.URL.String())
			}

			// Check headers
			for key, values := range tt.expectHeader {
				if !reflect.DeepEqual(req.Header[key], values) {
					t.Errorf("Header mismatch for %s: expected %v, got %v", key, values, req.Header[key])
				}
			}
		})
	}
}

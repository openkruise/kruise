package podprobe

import (
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
	tcpprobe "k8s.io/kubernetes/pkg/probe/tcp"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
)

// New creates Prober.
func New() prober {
	return prober{
		tcp: tcpprobe.New(),
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
	}

	prober := New()
	for i, tt := range tests {
		status, _, err := prober.runProbe(tt.p, tt.probeKey, tt.containerRuntimeStatus, tt.containerID)
		if status != tt.expectedStatus {
			t.Errorf("#%d: expected status=%v, get=%v", i, tt.expectedStatus, status)
		}
		if err != tt.expectedError {
			t.Errorf("#%d: expected error=%v, get=%v", i, tt.expectedError, err)
		}
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

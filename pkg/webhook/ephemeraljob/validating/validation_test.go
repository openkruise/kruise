package validating

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/kubernetes/pkg/apis/core"
	"k8s.io/kubernetes/pkg/apis/core/validation"
)

var (
	containerRestartPolicyAlways    = core.ContainerRestartPolicyAlways
	containerRestartPolicyOnFailure = core.ContainerRestartPolicy("OnFailure")
	containerRestartPolicyNever     = core.ContainerRestartPolicy("Never")
	containerRestartPolicyInvalid   = core.ContainerRestartPolicy("invalid")
	containerRestartPolicyEmpty     = core.ContainerRestartPolicy("")
)

func line() string {
	_, _, line, ok := runtime.Caller(1)
	var s string
	if ok {
		s = fmt.Sprintf("%d", line)
	} else {
		s = "<??>"
	}
	return s
}

func prettyErrorList(errs field.ErrorList) string {
	var s string
	for _, e := range errs {
		s += fmt.Sprintf("\t%s\n", e)
	}
	return s
}

func TestValidateEphemeralContainers(t *testing.T) {
	// Success Cases
	for title, ephemeralContainers := range map[string][]core.EphemeralContainer{
		"Empty Ephemeral Containers": {},
		"Single Container": {
			{EphemeralContainerCommon: core.EphemeralContainerCommon{Name: "debug", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
		},
		"Multiple Containers": {
			{EphemeralContainerCommon: core.EphemeralContainerCommon{Name: "debug1", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			{EphemeralContainerCommon: core.EphemeralContainerCommon{Name: "debug2", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
		},
		"Single Container with Target": {{
			EphemeralContainerCommon: core.EphemeralContainerCommon{Name: "debug", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"},
			TargetContainerName:      "ctr",
		}},
		"All allowed fields": {{
			EphemeralContainerCommon: core.EphemeralContainerCommon{

				Name:       "debug",
				Image:      "image",
				Command:    []string{"bash"},
				Args:       []string{"bash"},
				WorkingDir: "/",
				EnvFrom: []core.EnvFromSource{{
					ConfigMapRef: &core.ConfigMapEnvSource{
						LocalObjectReference: core.LocalObjectReference{Name: "dummy"},
						Optional:             &[]bool{true}[0],
					},
				}},
				Env: []core.EnvVar{
					{Name: "TEST", Value: "TRUE"},
				},
				VolumeMounts: []core.VolumeMount{
					{Name: "vol", MountPath: "/vol"},
				},
				VolumeDevices: []core.VolumeDevice{
					{Name: "blk", DevicePath: "/dev/block"},
				},
				TerminationMessagePath:   "/dev/termination-log",
				TerminationMessagePolicy: "File",
				ImagePullPolicy:          "IfNotPresent",
				SecurityContext: &core.SecurityContext{
					Capabilities: &core.Capabilities{
						Add: []core.Capability{"SYS_ADMIN"},
					},
				},
				Stdin:     true,
				StdinOnce: true,
				TTY:       true,
			},
		}},
	} {
		if errs := validateEphemeralContainers(ephemeralContainers, field.NewPath("ephemeralContainers"), validation.PodValidationOptions{}, true); len(errs) != 0 {
			t.Errorf("expected success for '%s' but got errors: %v", title, errs)
		}
	}

	// Failure Cases
	tcs := []struct {
		title, line         string
		ephemeralContainers []core.EphemeralContainer
		expectedErrors      field.ErrorList
	}{{
		"Name Collision with EphemeralContainers",
		line(),
		[]core.EphemeralContainer{
			{EphemeralContainerCommon: core.EphemeralContainerCommon{Name: "debug1", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			{EphemeralContainerCommon: core.EphemeralContainerCommon{Name: "debug1", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
		},
		field.ErrorList{{Type: field.ErrorTypeDuplicate, Field: "ephemeralContainers[1].name"}},
	}, {
		"empty Container",
		line(),
		[]core.EphemeralContainer{
			{EphemeralContainerCommon: core.EphemeralContainerCommon{}},
		},
		field.ErrorList{
			{Type: field.ErrorTypeRequired, Field: "ephemeralContainers[0].name"},
			{Type: field.ErrorTypeRequired, Field: "ephemeralContainers[0].image"},
			{Type: field.ErrorTypeRequired, Field: "ephemeralContainers[0].terminationMessagePolicy"},
			{Type: field.ErrorTypeRequired, Field: "ephemeralContainers[0].imagePullPolicy"},
		},
	}, {
		"empty Container Name",
		line(),
		[]core.EphemeralContainer{
			{EphemeralContainerCommon: core.EphemeralContainerCommon{Name: "", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
		},
		field.ErrorList{{Type: field.ErrorTypeRequired, Field: "ephemeralContainers[0].name"}},
	}, {
		"whitespace padded image name",
		line(),
		[]core.EphemeralContainer{
			{EphemeralContainerCommon: core.EphemeralContainerCommon{Name: "debug", Image: " image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
		},
		field.ErrorList{{Type: field.ErrorTypeInvalid, Field: "ephemeralContainers[0].image"}},
	}, {
		"invalid image pull policy",
		line(),
		[]core.EphemeralContainer{
			{EphemeralContainerCommon: core.EphemeralContainerCommon{Name: "debug", Image: "image", ImagePullPolicy: "PullThreeTimes", TerminationMessagePolicy: "File"}},
		},
		field.ErrorList{{Type: field.ErrorTypeNotSupported, Field: "ephemeralContainers[0].imagePullPolicy"}},
	}, {
		"Container uses disallowed field: Lifecycle",
		line(),
		[]core.EphemeralContainer{{
			EphemeralContainerCommon: core.EphemeralContainerCommon{
				Name:                     "debug",
				Image:                    "image",
				ImagePullPolicy:          "IfNotPresent",
				TerminationMessagePolicy: "File",
				Lifecycle: &core.Lifecycle{
					PreStop: &core.LifecycleHandler{
						Exec: &core.ExecAction{Command: []string{"ls", "-l"}},
					},
				},
			},
		}},
		field.ErrorList{{Type: field.ErrorTypeForbidden, Field: "ephemeralContainers[0].lifecycle"}},
	}, {
		"Container uses disallowed field: LivenessProbe",
		line(),
		[]core.EphemeralContainer{{
			EphemeralContainerCommon: core.EphemeralContainerCommon{
				Name:                     "debug",
				Image:                    "image",
				ImagePullPolicy:          "IfNotPresent",
				TerminationMessagePolicy: "File",
				LivenessProbe: &core.Probe{
					ProbeHandler: core.ProbeHandler{
						TCPSocket: &core.TCPSocketAction{Port: intstr.FromInt32(80)},
					},
					SuccessThreshold: 1,
				},
			},
		}},
		field.ErrorList{{Type: field.ErrorTypeForbidden, Field: "ephemeralContainers[0].livenessProbe"}},
	}, {
		"Container uses disallowed field: Ports",
		line(),
		[]core.EphemeralContainer{{
			EphemeralContainerCommon: core.EphemeralContainerCommon{
				Name:                     "debug",
				Image:                    "image",
				ImagePullPolicy:          "IfNotPresent",
				TerminationMessagePolicy: "File",
				Ports: []core.ContainerPort{
					{Protocol: "TCP", ContainerPort: 80},
				},
			},
		}},
		field.ErrorList{{Type: field.ErrorTypeForbidden, Field: "ephemeralContainers[0].ports"}},
	}, {
		"Container uses disallowed field: ReadinessProbe",
		line(),
		[]core.EphemeralContainer{{
			EphemeralContainerCommon: core.EphemeralContainerCommon{
				Name:                     "debug",
				Image:                    "image",
				ImagePullPolicy:          "IfNotPresent",
				TerminationMessagePolicy: "File",
				ReadinessProbe: &core.Probe{
					ProbeHandler: core.ProbeHandler{
						TCPSocket: &core.TCPSocketAction{Port: intstr.FromInt32(80)},
					},
				},
			},
		}},
		field.ErrorList{{Type: field.ErrorTypeForbidden, Field: "ephemeralContainers[0].readinessProbe"}},
	}, {
		"Container uses disallowed field: StartupProbe",
		line(),
		[]core.EphemeralContainer{{
			EphemeralContainerCommon: core.EphemeralContainerCommon{
				Name:                     "debug",
				Image:                    "image",
				ImagePullPolicy:          "IfNotPresent",
				TerminationMessagePolicy: "File",
				StartupProbe: &core.Probe{
					ProbeHandler: core.ProbeHandler{
						TCPSocket: &core.TCPSocketAction{Port: intstr.FromInt32(80)},
					},
					SuccessThreshold: 1,
				},
			},
		}},
		field.ErrorList{{Type: field.ErrorTypeForbidden, Field: "ephemeralContainers[0].startupProbe"}},
	}, {
		"Container uses disallowed field: Resources",
		line(),
		[]core.EphemeralContainer{{
			EphemeralContainerCommon: core.EphemeralContainerCommon{
				Name:                     "debug",
				Image:                    "image",
				ImagePullPolicy:          "IfNotPresent",
				TerminationMessagePolicy: "File",
				Resources: core.ResourceRequirements{
					Limits: core.ResourceList{
						core.ResourceCPU: resource.MustParse("10"),
					},
				},
			},
		}},
		field.ErrorList{{Type: field.ErrorTypeForbidden, Field: "ephemeralContainers[0].resources"}},
	}, {
		"Container uses disallowed field: VolumeMount.SubPath",
		line(),
		[]core.EphemeralContainer{{
			EphemeralContainerCommon: core.EphemeralContainerCommon{
				Name:                     "debug",
				Image:                    "image",
				ImagePullPolicy:          "IfNotPresent",
				TerminationMessagePolicy: "File",
				VolumeMounts: []core.VolumeMount{
					{Name: "vol", MountPath: "/vol"},
					{Name: "vol", MountPath: "/volsub", SubPath: "foo"},
				},
			},
		}},
		field.ErrorList{{Type: field.ErrorTypeForbidden, Field: "ephemeralContainers[0].volumeMounts[1].subPath"}},
	}, {
		"Container uses disallowed field: VolumeMount.SubPathExpr",
		line(),
		[]core.EphemeralContainer{{
			EphemeralContainerCommon: core.EphemeralContainerCommon{
				Name:                     "debug",
				Image:                    "image",
				ImagePullPolicy:          "IfNotPresent",
				TerminationMessagePolicy: "File",
				VolumeMounts: []core.VolumeMount{
					{Name: "vol", MountPath: "/vol"},
					{Name: "vol", MountPath: "/volsub", SubPathExpr: "$(POD_NAME)"},
				},
			},
		}},
		field.ErrorList{{Type: field.ErrorTypeForbidden, Field: "ephemeralContainers[0].volumeMounts[1].subPathExpr"}},
	}, {
		"Disallowed field with other errors should only return a single Forbidden",
		line(),
		[]core.EphemeralContainer{{
			EphemeralContainerCommon: core.EphemeralContainerCommon{
				Name:                     "debug",
				Image:                    "image",
				ImagePullPolicy:          "IfNotPresent",
				TerminationMessagePolicy: "File",
				Lifecycle: &core.Lifecycle{
					PreStop: &core.LifecycleHandler{
						Exec: &core.ExecAction{Command: []string{}},
					},
				},
			},
		}},
		field.ErrorList{{Type: field.ErrorTypeForbidden, Field: "ephemeralContainers[0].lifecycle"}},
	}, {
		"Container uses disallowed field: ResizePolicy",
		line(),
		[]core.EphemeralContainer{{
			EphemeralContainerCommon: core.EphemeralContainerCommon{
				Name:                     "resources-resize-policy",
				Image:                    "image",
				ImagePullPolicy:          "IfNotPresent",
				TerminationMessagePolicy: "File",
				ResizePolicy: []core.ContainerResizePolicy{
					{ResourceName: "cpu", RestartPolicy: "NotRequired"},
				},
			},
		}},
		field.ErrorList{{Type: field.ErrorTypeForbidden, Field: "ephemeralContainers[0].resizePolicy"}},
	}, {
		"Forbidden RestartPolicy: Always",
		line(),
		[]core.EphemeralContainer{{
			EphemeralContainerCommon: core.EphemeralContainerCommon{
				Name:                     "foo",
				Image:                    "image",
				ImagePullPolicy:          "IfNotPresent",
				TerminationMessagePolicy: "File",
				RestartPolicy:            &containerRestartPolicyAlways,
			},
		}},
		field.ErrorList{{Type: field.ErrorTypeForbidden, Field: "ephemeralContainers[0].restartPolicy"}},
	}, {
		"Forbidden RestartPolicy: OnFailure",
		line(),
		[]core.EphemeralContainer{{
			EphemeralContainerCommon: core.EphemeralContainerCommon{
				Name:                     "foo",
				Image:                    "image",
				ImagePullPolicy:          "IfNotPresent",
				TerminationMessagePolicy: "File",
				RestartPolicy:            &containerRestartPolicyOnFailure,
			},
		}},
		field.ErrorList{{Type: field.ErrorTypeForbidden, Field: "ephemeralContainers[0].restartPolicy"}},
	}, {
		"Forbidden RestartPolicy: Never",
		line(),
		[]core.EphemeralContainer{{
			EphemeralContainerCommon: core.EphemeralContainerCommon{
				Name:                     "foo",
				Image:                    "image",
				ImagePullPolicy:          "IfNotPresent",
				TerminationMessagePolicy: "File",
				RestartPolicy:            &containerRestartPolicyNever,
			},
		}},
		field.ErrorList{{Type: field.ErrorTypeForbidden, Field: "ephemeralContainers[0].restartPolicy"}},
	}, {
		"Forbidden RestartPolicy: invalid",
		line(),
		[]core.EphemeralContainer{{
			EphemeralContainerCommon: core.EphemeralContainerCommon{
				Name:                     "foo",
				Image:                    "image",
				ImagePullPolicy:          "IfNotPresent",
				TerminationMessagePolicy: "File",
				RestartPolicy:            &containerRestartPolicyInvalid,
			},
		}},
		field.ErrorList{{Type: field.ErrorTypeForbidden, Field: "ephemeralContainers[0].restartPolicy"}},
	}, {
		"Forbidden RestartPolicy: empty",
		line(),
		[]core.EphemeralContainer{{
			EphemeralContainerCommon: core.EphemeralContainerCommon{
				Name:                     "foo",
				Image:                    "image",
				ImagePullPolicy:          "IfNotPresent",
				TerminationMessagePolicy: "File",
				RestartPolicy:            &containerRestartPolicyEmpty,
			},
		}},
		field.ErrorList{{Type: field.ErrorTypeForbidden, Field: "ephemeralContainers[0].restartPolicy"}},
	},
	}

	for _, tc := range tcs {
		t.Run(tc.title+"__@L"+tc.line, func(t *testing.T) {
			errs := validateEphemeralContainers(tc.ephemeralContainers, field.NewPath("ephemeralContainers"), validation.PodValidationOptions{}, true)
			if len(errs) == 0 {
				t.Fatal("expected error but received none")
			}

			if diff := cmp.Diff(tc.expectedErrors, errs, cmpopts.IgnoreFields(field.Error{}, "BadValue", "Detail")); diff != "" {
				t.Errorf("unexpected diff in errors (-want, +got):\n%s", diff)
				t.Errorf("INFO: all errors:\n%s", prettyErrorList(errs))
			}
		})
	}
}

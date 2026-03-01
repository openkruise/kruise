package validating

import (
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"

	"k8s.io/apimachinery/pkg/util/sets"
	utilvalidation "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/kubernetes/pkg/apis/core"
	"k8s.io/kubernetes/pkg/apis/core/validation"
)

// reference: https://github.com/kubernetes/kubernetes/blob/0d90d1ffa5e87dfc4d3098da7f281351c7ff1972/pkg/apis/core/validation/validation.go\#L3171
var supportedPullPolicies = sets.NewString(string(core.PullAlways), string(core.PullIfNotPresent), string(core.PullNever))
var supportedPortProtocols = sets.NewString(string(core.ProtocolTCP), string(core.ProtocolUDP), string(core.ProtocolSCTP))

var allowedEphemeralContainerFields = map[string]bool{
	"Name":                     true,
	"Image":                    true,
	"Command":                  true,
	"Args":                     true,
	"WorkingDir":               true,
	"Ports":                    false,
	"EnvFrom":                  true,
	"Env":                      true,
	"Resources":                false,
	"VolumeMounts":             true,
	"VolumeDevices":            true,
	"LivenessProbe":            false,
	"ReadinessProbe":           false,
	"StartupProbe":             false,
	"Lifecycle":                false,
	"TerminationMessagePath":   true,
	"TerminationMessagePolicy": true,
	"ImagePullPolicy":          true,
	"SecurityContext":          true,
	"Stdin":                    true,
	"StdinOnce":                true,
	"TTY":                      true,
}

func validateContainerPorts(ports []core.ContainerPort, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	allNames := sets.String{}
	for i, port := range ports {
		idxPath := fldPath.Index(i)
		if len(port.Name) > 0 {
			if msgs := utilvalidation.IsValidPortName(port.Name); len(msgs) != 0 {
				for i = range msgs {
					allErrs = append(allErrs, field.Invalid(idxPath.Child("name"), port.Name, msgs[i]))
				}
			} else if allNames.Has(port.Name) {
				allErrs = append(allErrs, field.Duplicate(idxPath.Child("name"), port.Name))
			} else {
				allNames.Insert(port.Name)
			}
		}
		if port.ContainerPort == 0 {
			allErrs = append(allErrs, field.Required(idxPath.Child("containerPort"), ""))
		} else {
			for _, msg := range utilvalidation.IsValidPortNum(int(port.ContainerPort)) {
				allErrs = append(allErrs, field.Invalid(idxPath.Child("containerPort"), port.ContainerPort, msg))
			}
		}
		if port.HostPort != 0 {
			for _, msg := range utilvalidation.IsValidPortNum(int(port.HostPort)) {
				allErrs = append(allErrs, field.Invalid(idxPath.Child("hostPort"), port.HostPort, msg))
			}
		}
		if len(port.Protocol) == 0 {
			allErrs = append(allErrs, field.Required(idxPath.Child("protocol"), ""))
		} else if !supportedPortProtocols.Has(string(port.Protocol)) {
			allErrs = append(allErrs, field.NotSupported(idxPath.Child("protocol"), port.Protocol, supportedPortProtocols.List()))
		}
	}
	return allErrs
}

func validatePullPolicy(policy core.PullPolicy, fldPath *field.Path) field.ErrorList {
	allErrors := field.ErrorList{}

	switch policy {
	case core.PullAlways, core.PullIfNotPresent, core.PullNever:
		break
	case "":
		allErrors = append(allErrors, field.Required(fldPath, ""))
	default:
		allErrors = append(allErrors, field.NotSupported(fldPath, policy, supportedPullPolicies.List()))
	}
	return allErrors
}

// validateContainerCommon applies validation common to all container types. It's called by regular, init, and ephemeral
// container list validation to require a properly formatted name, image, etc.
func validateContainerCommon(ctr *core.Container, path *field.Path, opts validation.PodValidationOptions, hostUsers bool) field.ErrorList {
	var allErrs field.ErrorList

	namePath := path.Child("name")
	if len(ctr.Name) == 0 {
		allErrs = append(allErrs, field.Required(namePath, ""))
	} else {
		allErrs = append(allErrs, validation.ValidateDNS1123Label(ctr.Name, namePath)...)
	}

	// TODO: do not validate leading and trailing whitespace to preserve backward compatibility.
	// for example: https://github.com/openshift/origin/issues/14659 image = " " is special token in pod template
	// others may have done similar
	if len(ctr.Image) == 0 {
		allErrs = append(allErrs, field.Required(path.Child("image"), ""))
	}

	switch ctr.TerminationMessagePolicy {
	case core.TerminationMessageReadFile, core.TerminationMessageFallbackToLogsOnError:
	case "":
		allErrs = append(allErrs, field.Required(path.Child("terminationMessagePolicy"), ""))
	default:
		supported := []string{
			string(core.TerminationMessageReadFile),
			string(core.TerminationMessageFallbackToLogsOnError),
		}
		allErrs = append(allErrs, field.NotSupported(path.Child("terminationMessagePolicy"), ctr.TerminationMessagePolicy, supported))
	}

	allErrs = append(allErrs, validateContainerPorts(ctr.Ports, path.Child("ports"))...)
	allErrs = append(allErrs, validation.ValidateEnv(ctr.Env, path.Child("env"), opts)...)
	allErrs = append(allErrs, validation.ValidateEnvFrom(ctr.EnvFrom, path.Child("envFrom"), opts)...)
	allErrs = append(allErrs, validatePullPolicy(ctr.ImagePullPolicy, path.Child("imagePullPolicy"))...)
	allErrs = append(allErrs, validation.ValidateSecurityContext(ctr.SecurityContext, path.Child("securityContext"), hostUsers)...)
	return allErrs
}

// validateContainerOnlyForPod does pod-only (i.e. not pod template) validation for a single container.
// This is called by validateContainersOnlyForPod and validateEphemeralContainers directly.
func validateContainerOnlyForPod(ctr *core.Container, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if len(ctr.Image) != len(strings.TrimSpace(ctr.Image)) {
		allErrs = append(allErrs, field.Invalid(path.Child("image"), ctr.Image, "must not have leading or trailing whitespace"))
	}
	return allErrs
}

// ValidateFieldAcceptList checks that only allowed fields are set.
// The value must be a struct (not a pointer to a struct!).
func validateFieldAllowList(value interface{}, allowedFields map[string]bool, errorText string, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	reflectType, reflectValue := reflect.TypeOf(value), reflect.ValueOf(value)
	for i := 0; i < reflectType.NumField(); i++ {
		f := reflectType.Field(i)
		if allowedFields[f.Name] {
			continue
		}

		// Compare the value of this field to its zero value to determine if it has been set
		if !reflect.DeepEqual(reflectValue.Field(i).Interface(), reflect.Zero(f.Type).Interface()) {
			r, n := utf8.DecodeRuneInString(f.Name)
			lcName := string(unicode.ToLower(r)) + f.Name[n:]
			allErrs = append(allErrs, field.Forbidden(fldPath.Child(lcName), errorText))
		}
	}

	return allErrs
}

// validateEphemeralContainers is called by pod spec and template validation to validate the list of ephemeral containers.
// Note that this is called for pod template even though ephemeral containers aren't allowed in pod templates.
func validateEphemeralContainers(ephemeralContainers []core.EphemeralContainer, fldPath *field.Path, opts validation.PodValidationOptions, hostUsers bool) field.ErrorList {
	var allErrs field.ErrorList

	if len(ephemeralContainers) == 0 {
		return allErrs
	}

	allNames := sets.String{}

	for i, ec := range ephemeralContainers {
		idxPath := fldPath.Index(i)

		c := (*core.Container)(&ec.EphemeralContainerCommon)
		allErrs = append(allErrs, validateContainerCommon(c, idxPath, opts, hostUsers)...)
		// Ephemeral containers don't need looser constraints for pod templates, so it's convenient to apply both validations
		// here where we've already converted EphemeralContainerCommon to Container.
		allErrs = append(allErrs, validateContainerOnlyForPod(c, idxPath)...)

		// Ephemeral containers must have a name unique across all container types.
		if allNames.Has(ec.Name) {
			allErrs = append(allErrs, field.Duplicate(idxPath.Child("name"), ec.Name))
		} else {
			allNames.Insert(ec.Name)
		}

		// Ephemeral containers should not be relied upon for fundamental pod services, so fields such as
		// Lifecycle, probes, resources and ports should be disallowed. This is implemented as a list
		// of allowed fields so that new fields will be given consideration prior to inclusion in ephemeral containers.
		allErrs = append(allErrs, validateFieldAllowList(ec.EphemeralContainerCommon, allowedEphemeralContainerFields, "cannot be set for an Ephemeral Container", idxPath)...)

		// VolumeMount subpaths have the potential to leak resources since they're implemented with bind mounts
		// that aren't cleaned up until the pod exits. Since they also imply that the container is being used
		// as part of the workload, they're disallowed entirely.
		for i, vm := range ec.VolumeMounts {
			if vm.SubPath != "" {
				allErrs = append(allErrs, field.Forbidden(idxPath.Child("volumeMounts").Index(i).Child("subPath"), "cannot be set for an Ephemeral Container"))
			}
			if vm.SubPathExpr != "" {
				allErrs = append(allErrs, field.Forbidden(idxPath.Child("volumeMounts").Index(i).Child("subPathExpr"), "cannot be set for an Ephemeral Container"))
			}
		}
	}

	return allErrs
}

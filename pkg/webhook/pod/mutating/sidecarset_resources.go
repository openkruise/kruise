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

package mutating

import (
	"fmt"
	"regexp"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog/v2"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/control/sidecarcontrol"
	"github.com/openkruise/kruise/pkg/util/calculator"
)

// applyResourcesPolicy applies the resources policy to the sidecar container
// based on the target containers in the pod
func applyResourcesPolicy(
	pod *corev1.Pod,
	sidecarContainer *appsv1alpha1.SidecarContainer,
	matchedSidecarSets []sidecarcontrol.SidecarControl,
) error {
	// Only apply when ResourcesPolicy is configured
	if sidecarContainer.ResourcesPolicy == nil {
		return nil
	}

	policy := sidecarContainer.ResourcesPolicy
	klog.V(4).InfoS("Applying resources policy", "container", sidecarContainer.Name,
		"mode", policy.TargetContainerMode, "regex", policy.TargetContainersNameRegex)

	// Get target containers (exclude Kruise sidecar containers)
	targetContainers, err := getTargetContainers(pod, policy.TargetContainersNameRegex, matchedSidecarSets)
	if err != nil {
		return fmt.Errorf("failed to get target containers: %v", err)
	}

	if len(targetContainers) == 0 {
		return fmt.Errorf("no containers match the regex pattern %q", policy.TargetContainersNameRegex)
	}

	klog.V(4).InfoS("Found target containers", "count", len(targetContainers),
		"names", getContainerNames(targetContainers))

	// Calculate aggregated resources based on mode
	aggregatedLimits, aggregatedRequests := aggregateResources(targetContainers, policy.TargetContainerMode)

	// Apply resource expressions
	resources := corev1.ResourceRequirements{}

	// Calculate limits
	if policy.ResourceExpr.Limits != nil {
		limits := corev1.ResourceList{}

		// CPU limits
		if policy.ResourceExpr.Limits.CPU != "" {
			// Check if aggregated CPU limit exists (not unlimited)
			if cpuValue, exists := aggregatedLimits[corev1.ResourceCPU]; exists {
				cpuLimit, err := evaluateResourceExpression(
					policy.ResourceExpr.Limits.CPU,
					cpuValue,
					true, // isLimit
				)
				if err != nil {
					return fmt.Errorf("failed to evaluate CPU limits expression: %v", err)
				}
				if cpuLimit != nil {
					limits[corev1.ResourceCPU] = *cpuLimit
				}
			}
			// else: aggregated CPU limit doesn't exist (unlimited), so don't set it
		}

		// Memory limits
		if policy.ResourceExpr.Limits.Memory != "" {
			// Check if aggregated Memory limit exists (not unlimited)
			if memValue, exists := aggregatedLimits[corev1.ResourceMemory]; exists {
				memLimit, err := evaluateResourceExpression(
					policy.ResourceExpr.Limits.Memory,
					memValue,
					true, // isLimit
				)
				if err != nil {
					return fmt.Errorf("failed to evaluate memory limits expression: %v", err)
				}
				if memLimit != nil {
					limits[corev1.ResourceMemory] = *memLimit
				}
			}
			// else: aggregated Memory limit doesn't exist (unlimited), so don't set it
		}

		if len(limits) > 0 {
			resources.Limits = limits
		}
	}

	// Calculate requests
	if policy.ResourceExpr.Requests != nil {
		requests := corev1.ResourceList{}

		// CPU requests
		if policy.ResourceExpr.Requests.CPU != "" {
			cpuRequest, err := evaluateResourceExpression(
				policy.ResourceExpr.Requests.CPU,
				aggregatedRequests[corev1.ResourceCPU],
				false, // isLimit
			)
			if err != nil {
				return fmt.Errorf("failed to evaluate CPU requests expression: %v", err)
			}
			if cpuRequest != nil {
				requests[corev1.ResourceCPU] = *cpuRequest
			}
		}

		// Memory requests
		if policy.ResourceExpr.Requests.Memory != "" {
			memRequest, err := evaluateResourceExpression(
				policy.ResourceExpr.Requests.Memory,
				aggregatedRequests[corev1.ResourceMemory],
				false, // isLimit
			)
			if err != nil {
				return fmt.Errorf("failed to evaluate memory requests expression: %v", err)
			}
			if memRequest != nil {
				requests[corev1.ResourceMemory] = *memRequest
			}
		}

		if len(requests) > 0 {
			resources.Requests = requests
		}
	}

	// Apply calculated resources to the sidecar container
	sidecarContainer.Resources = resources

	klog.V(3).InfoS("Applied resources policy", "container", sidecarContainer.Name,
		"limits", resources.Limits, "requests", resources.Requests)

	return nil
}

// getTargetContainers returns containers that match the regex pattern
// Excludes Kruise sidecar containers
func getTargetContainers(
	pod *corev1.Pod,
	nameRegex string,
	matchedSidecarSets []sidecarcontrol.SidecarControl,
) ([]corev1.Container, error) {
	// Compile regex pattern
	pattern, err := regexp.Compile(nameRegex)
	if err != nil {
		return nil, fmt.Errorf("invalid regex pattern %q: %v", nameRegex, err)
	}

	// Build set of Kruise sidecar container names
	kruiseSidecarNames := make(map[string]bool)
	for _, control := range matchedSidecarSets {
		sidecarSet := control.GetSidecarset()
		for i := range sidecarSet.Spec.InitContainers {
			kruiseSidecarNames[sidecarSet.Spec.InitContainers[i].Name] = true
		}
		for i := range sidecarSet.Spec.Containers {
			kruiseSidecarNames[sidecarSet.Spec.Containers[i].Name] = true
		}
	}

	var targetContainers []corev1.Container

	// Check native sidecar containers (init containers with restartPolicy Always)
	for i := range pod.Spec.InitContainers {
		container := &pod.Spec.InitContainers[i]
		// Skip if it's a Kruise sidecar
		if kruiseSidecarNames[container.Name] {
			continue
		}

		// Only include init containers with RestartPolicy Always (native sidecars)
		// Skip if RestartPolicy is nil or not Always
		if container.RestartPolicy == nil || *container.RestartPolicy != corev1.ContainerRestartPolicyAlways {
			continue
		}

		// Check if name matches the pattern
		if pattern.MatchString(container.Name) {
			targetContainers = append(targetContainers, *container)
		}
	}

	// Check plain containers
	for i := range pod.Spec.Containers {
		container := &pod.Spec.Containers[i]
		// Skip if it's a Kruise sidecar
		if kruiseSidecarNames[container.Name] {
			continue
		}
		// Check if name matches the pattern
		if pattern.MatchString(container.Name) {
			targetContainers = append(targetContainers, *container)
		}
	}

	return targetContainers, nil
}

// aggregateResources aggregates resources from target containers based on mode
func aggregateResources(
	containers []corev1.Container,
	mode appsv1alpha1.TargetContainerModeType,
) (limits, requests corev1.ResourceList) {
	limits = corev1.ResourceList{}
	requests = corev1.ResourceList{}

	switch mode {
	case appsv1alpha1.TargetContainerModeSum:
		limits, requests = aggregateResourcesBySum(containers)
	case appsv1alpha1.TargetContainerModeMax:
		limits, requests = aggregateResourcesByMax(containers)
	}

	return limits, requests
}

// aggregateResourcesBySum sums up resources from all containers
func aggregateResourcesBySum(containers []corev1.Container) (limits, requests corev1.ResourceList) {
	limits = corev1.ResourceList{}
	requests = corev1.ResourceList{}

	cpuLimit := resource.NewQuantity(0, resource.DecimalSI)
	memLimit := resource.NewQuantity(0, resource.DecimalSI)
	cpuRequest := resource.NewQuantity(0, resource.DecimalSI)
	memRequest := resource.NewQuantity(0, resource.DecimalSI)

	// Track whether ALL containers have limits configured
	// If any container doesn't have a limit, the aggregated result should be unlimited
	allHaveLimitCPU := true
	allHaveLimitMemory := true

	for _, container := range containers {
		// Sum CPU limits - but track if any container doesn't have it
		if cpu, ok := container.Resources.Limits[corev1.ResourceCPU]; ok {
			cpuLimit.Add(cpu)
		} else {
			// This container has no CPU limit (unlimited), so aggregate should be unlimited
			allHaveLimitCPU = false
		}

		// Sum Memory limits - but track if any container doesn't have it
		if mem, ok := container.Resources.Limits[corev1.ResourceMemory]; ok {
			memLimit.Add(mem)
		} else {
			// This container has no memory limit (unlimited), so aggregate should be unlimited
			allHaveLimitMemory = false
		}

		// Sum CPU requests - treat missing as 0
		if cpu, ok := container.Resources.Requests[corev1.ResourceCPU]; ok {
			cpuRequest.Add(cpu)
		}
		// If not present, treat as 0 (no need to add)

		// Sum Memory requests - treat missing as 0
		if mem, ok := container.Resources.Requests[corev1.ResourceMemory]; ok {
			memRequest.Add(mem)
		}
		// If not present, treat as 0 (no need to add)
	}

	// Set aggregated limits only if ALL containers have limits configured
	if allHaveLimitCPU {
		limits[corev1.ResourceCPU] = *cpuLimit
	}
	// else: at least one container is unlimited, so don't set limit (unlimited)

	if allHaveLimitMemory {
		limits[corev1.ResourceMemory] = *memLimit
	}
	// else: at least one container is unlimited, so don't set limit (unlimited)

	// Set aggregated requests (0 is valid for requests)
	if !cpuRequest.IsZero() {
		requests[corev1.ResourceCPU] = *cpuRequest
	}
	if !memRequest.IsZero() {
		requests[corev1.ResourceMemory] = *memRequest
	}

	return limits, requests
}

// aggregateResourcesByMax takes the maximum resource from all containers
func aggregateResourcesByMax(containers []corev1.Container) (limits, requests corev1.ResourceList) {
	limits = corev1.ResourceList{}
	requests = corev1.ResourceList{}

	// Track whether ALL containers have limits configured
	// If any container doesn't have a limit, the aggregated result should be unlimited
	allHaveLimitCPU := true
	allHaveLimitMemory := true

	for _, container := range containers {
		// Max CPU limits - but track if any container doesn't have it
		if cpu, ok := container.Resources.Limits[corev1.ResourceCPU]; ok {
			if maxCPU, exists := limits[corev1.ResourceCPU]; !exists || cpu.Cmp(maxCPU) > 0 {
				limits[corev1.ResourceCPU] = cpu
			}
		} else {
			// This container has no CPU limit (unlimited), so aggregate should be unlimited
			allHaveLimitCPU = false
		}

		// Max Memory limits - but track if any container doesn't have it
		if mem, ok := container.Resources.Limits[corev1.ResourceMemory]; ok {
			if maxMem, exists := limits[corev1.ResourceMemory]; !exists || mem.Cmp(maxMem) > 0 {
				limits[corev1.ResourceMemory] = mem
			}
		} else {
			// This container has no memory limit (unlimited), so aggregate should be unlimited
			allHaveLimitMemory = false
		}

		// Max CPU requests - treat missing as 0
		if cpu, ok := container.Resources.Requests[corev1.ResourceCPU]; ok {
			if maxCPU, exists := requests[corev1.ResourceCPU]; !exists || cpu.Cmp(maxCPU) > 0 {
				requests[corev1.ResourceCPU] = cpu
			}
		}
		// If not present, treat as 0 (implicitly handled by not updating requests map)

		// Max Memory requests - treat missing as 0
		if mem, ok := container.Resources.Requests[corev1.ResourceMemory]; ok {
			if maxMem, exists := requests[corev1.ResourceMemory]; !exists || mem.Cmp(maxMem) > 0 {
				requests[corev1.ResourceMemory] = mem
			}
		}
		// If not present, treat as 0 (implicitly handled by not updating requests map)
	}

	// If any container doesn't have limit configured, remove it to indicate unlimited
	if !allHaveLimitCPU {
		delete(limits, corev1.ResourceCPU)
	}
	if !allHaveLimitMemory {
		delete(limits, corev1.ResourceMemory)
	}

	return limits, requests
}

// evaluateResourceExpression evaluates a resource expression using the calculator
// Returns nil if the result is unlimited
// Note: This function should only be called when aggregatedValue is valid (not unlimited)
func evaluateResourceExpression(
	expr string,
	aggregatedValue resource.Quantity,
	isLimit bool,
) (*resource.Quantity, error) {
	// Prepare variable for calculator
	varName := "cpu"
	if expr == "" {
		// Empty expression means:
		// - For limits: unlimited (return nil)
		// - For requests: 0 (return zero)
		if isLimit {
			return nil, nil
		}
		return resource.NewQuantity(0, resource.DecimalSI), nil
	}

	// Detect if expression contains "cpu" or "memory" variable
	if regexp.MustCompile(`\bmemory\b`).MatchString(expr) {
		varName = "memory"
	}

	// Create calculator with variable
	calc := calculator.NewCalculator()
	vars := make(map[string]*calculator.Value)
	vars[varName] = &calculator.Value{
		IsQuantity: true,
		Quantity:   aggregatedValue,
	}
	calc.SetVariables(vars)

	// Parse and evaluate expression
	result, err := calc.Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse expression %q: %v", expr, err)
	}

	// Convert result to Quantity
	if !result.IsQuantity {
		// If result is a number, convert to Quantity
		// For CPU: use milli (m) unit
		// For memory: use binary (Mi/Gi) unit based on magnitude
		if varName == "cpu" {
			// Convert to millicores
			millis := int64(result.Number * 1000)
			return resource.NewMilliQuantity(millis, resource.DecimalSI), nil
		} else {
			// For memory, use DecimalSI
			return resource.NewQuantity(int64(result.Number), resource.DecimalSI), nil
		}
	}

	return &result.Quantity, nil
}

// getContainerNames returns a slice of container names (for logging)
func getContainerNames(containers []corev1.Container) []string {
	names := make([]string, len(containers))
	for i, c := range containers {
		names[i] = c.Name
	}
	return names
}

package pvc

import v1 "k8s.io/api/core/v1"

type CheckClaimFn = func(*v1.PersistentVolumeClaim, *v1.PersistentVolumeClaim) bool

// IsClaimCompatibleWithoutSize PersistentVolumeClaim Compare function
func IsClaimCompatibleWithoutSize(claim, template *v1.PersistentVolumeClaim) bool {
	// when there is default sc,
	// template StorageClassName is nil but claim is not nil
	if template.Spec.StorageClassName != nil &&
		claim.Spec.StorageClassName != nil &&
		*claim.Spec.StorageClassName != *template.Spec.StorageClassName {
		return false
	}
	// use set compare?
	if len(claim.Spec.AccessModes) != len(template.Spec.AccessModes) {
		return false
	}
	for i, mode := range claim.Spec.AccessModes {
		if template.Spec.AccessModes[i] != mode {
			return false
		}
	}
	return true
}

// IsPatchPVCCompleted PersistentVolumeClaim Compare function
func IsPatchPVCCompleted(claim, template *v1.PersistentVolumeClaim) bool {
	compatible, ready := IsPVCCompatibleAndReady(claim, template)
	if compatible && ready {
		return true
	}
	if compatible {
		pending := false
		for _, condition := range claim.Status.Conditions {
			if condition.Type == v1.PersistentVolumeClaimFileSystemResizePending &&
				condition.Status == v1.ConditionTrue {
				pending = true
			}
		}
		// if pending, patch PVC completed
		return pending
	}
	return false
}

// CompareWithCheckFn compares a PersistentVolumeClaim with a template claim using a custom comparison function.
// This function first checks if the claim is compatible with the template claim excluding size considerations.
// If the claim is compatible, it then uses the provided CheckClaimFn function to perform a detailed comparison.
// The function returns two boolean values: the first indicates whether the claim matches the template,
// and the second provides the result of the custom comparison function.
//
// Parameters:
// claim: The PersistentVolumeClaim to be compared.
// template: The template PersistentVolumeClaim to compare against.
// cmp: A CheckClaimFn function used for detailed comparison.
//
// Return values:
// matched: Indicates whether the claim matches the template (true if it matches, false otherwise).
// cmpResult: The result of the custom comparison function (true if the function returns true, false otherwise).
func CompareWithCheckFn(claim, template *v1.PersistentVolumeClaim, cmp CheckClaimFn) (matched, cmpResult bool) {
	if !IsClaimCompatibleWithoutSize(claim, template) {
		return false, false
	}
	if cmp(claim, template) {
		return false, true
	}
	return true, false
}

func IsPVCCompatibleAndReady(claim, template *v1.PersistentVolumeClaim) (compatible bool, ready bool) {
	if !IsClaimCompatibleWithoutSize(claim, template) {
		return false, false
	}
	compatible = func(claim, template *v1.PersistentVolumeClaim) bool {
		// claim >= template => compatible
		// claim < template => not compatible (need patch by controller)
		if claim.Spec.Resources.Requests.Storage() != nil &&
			template.Spec.Resources.Requests.Storage() != nil &&
			claim.Spec.Resources.Requests.Storage().Cmp(*template.Spec.Resources.Requests.Storage()) >= 0 {
			return true
		}
		return false
	}(claim, template)

	ready = func(claim, template *v1.PersistentVolumeClaim) bool {
		// cap >= spec => ready
		// cap < spec => need storage expansion by csi
		if claim.Status.Capacity.Storage() != nil &&
			claim.Spec.Resources.Requests.Storage() != nil &&
			claim.Status.Capacity.Storage().Cmp(*claim.Spec.Resources.Requests.Storage()) >= 0 {
			return true
		}
		return false
	}(claim, template)
	return
}

// IsPVCNeedExpand checks if the given PersistentVolumeClaim (PVC) has expanded based on a template.
// Parameters:
//
//	claim: The PVC object to be inspected.
//	template: The PVC template object for comparison.
//
// Return:
//
//	A boolean indicating whether the PVC has expanded beyond the template's definitions in terms of storage requests or limits.
//
// This function determines if the PVC has expanded by comparing the storage requests and limits between the PVC and the template.
// If either the storage request of the PVC exceeds that defined in the template, it is considered expanded.
func IsPVCNeedExpand(claim, template *v1.PersistentVolumeClaim) bool {
	// pvc spec < template spec => need expand
	if claim.Spec.Resources.Requests.Storage() != nil &&
		template.Spec.Resources.Requests.Storage() != nil &&
		claim.Spec.Resources.Requests.Storage().Cmp(*template.Spec.Resources.Requests.Storage()) < 0 {
		return true
	}
	return false
}

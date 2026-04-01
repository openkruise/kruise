package validating

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/validation/field"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

func validateUnitedDeployment(unitedDeployment *appsv1alpha1.UnitedDeployment) field.ErrorList {
	betaObj, errList := convertAlphaUnitedDeploymentToV1beta1(unitedDeployment)
	if len(errList) > 0 {
		return errList
	}
	return validateUnitedDeploymentV1beta1(betaObj)
}

func ValidateUnitedDeploymentUpdate(unitedDeployment, oldUnitedDeployment *appsv1alpha1.UnitedDeployment) field.ErrorList {
	betaObj, errList := convertAlphaUnitedDeploymentToV1beta1(unitedDeployment)
	if len(errList) > 0 {
		return errList
	}

	oldBetaObj, oldErrList := convertAlphaUnitedDeploymentToV1beta1(oldUnitedDeployment)
	if len(oldErrList) > 0 {
		return oldErrList
	}

	return ValidateUnitedDeploymentUpdateV1beta1(betaObj, oldBetaObj)
}

func validateUnitedDeploymentSpec(spec *appsv1alpha1.UnitedDeploymentSpec, fldPath *field.Path) field.ErrorList {
	alphaObj := &appsv1alpha1.UnitedDeployment{Spec: *spec.DeepCopy()}
	betaObj, errList := convertAlphaUnitedDeploymentToV1beta1(alphaObj)
	if len(errList) > 0 {
		return errList
	}

	return validateUnitedDeploymentSpecV1beta1(&betaObj.Spec, fldPath)
}

func validateSubsetReplicas(expectedReplicas *int32, subsets []appsv1alpha1.Subset, fldPath *field.Path) field.ErrorList {
	alphaObj := &appsv1alpha1.UnitedDeployment{
		Spec: appsv1alpha1.UnitedDeploymentSpec{
			Replicas: expectedReplicas,
			Topology: appsv1alpha1.Topology{
				Subsets: subsets,
			},
		},
	}
	betaObj, errList := convertAlphaUnitedDeploymentToV1beta1(alphaObj)
	if len(errList) > 0 {
		return errList
	}

	return validateSubsetReplicasV1beta1(betaObj.Spec.Replicas, betaObj.Spec.Topology.Subsets, fldPath)
}

func convertAlphaUnitedDeploymentToV1beta1(alphaObj *appsv1alpha1.UnitedDeployment) (*appsv1beta1.UnitedDeployment, field.ErrorList) {
	betaObj := &appsv1beta1.UnitedDeployment{}
	if err := alphaObj.ConvertTo(betaObj); err != nil {
		return nil, field.ErrorList{
			field.InternalError(field.NewPath(""), fmt.Errorf("failed to convert v1alpha1 UnitedDeployment to v1beta1: %w", err)),
		}
	}
	return betaObj, nil
}

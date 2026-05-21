package validating

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
)

func (h *ConfigMapSetCreateUpdateHandler) checkConflictsWithExistingConfigMapSets(ctx context.Context, obj *appsv1alpha1.ConfigMapSet) error {
	cmsList := &appsv1alpha1.ConfigMapSetList{}
	if err := h.Client.List(ctx, cmsList, client.InNamespace(obj.Namespace)); err != nil {
		return fmt.Errorf("failed to list existing ConfigMapSets: %v", err)
	}

	objSidecarName, err := h.getReloadSidecarName(ctx, obj)
	if err != nil {
		return fmt.Errorf("failed to get reload-sidecar name for current ConfigMapSet: %v", err)
	}

	for _, existingCMS := range cmsList.Items {
		if existingCMS.Name == obj.Name {
			continue
		}

		if obj.Spec.Selector != nil && existingCMS.Spec.Selector != nil {
			if !util.IsSelectorOverlapping(obj.Spec.Selector, existingCMS.Spec.Selector) {
				continue
			}
		}

		// Check for reload-sidecar name conflict
		existingSidecarName, err := h.getReloadSidecarName(ctx, &existingCMS)
		if err == nil && objSidecarName != "" && existingSidecarName != "" && objSidecarName == existingSidecarName {
			return fmt.Errorf("reload-sidecar name conflict: both %s and %s use the same sidecar name '%s' and their selectors overlap", obj.Name, existingCMS.Name, objSidecarName)
		}

		// Check for container mount path conflict
		for _, objC := range obj.Spec.Containers {
			for _, existC := range existingCMS.Spec.Containers {
				if objC.MountPath == existC.MountPath {
					if objC.Name != "" && objC.Name == existC.Name {
						return fmt.Errorf("container mount path conflict: both %s and %s mount to '%s' on container '%s'", obj.Name, existingCMS.Name, objC.MountPath, objC.Name)
					}
					if objC.NameFrom != nil && existC.NameFrom != nil && reflect.DeepEqual(objC.NameFrom, existC.NameFrom) {
						return fmt.Errorf("container mount path conflict: both %s and %s mount to '%s' using the same nameFrom reference", obj.Name, existingCMS.Name, objC.MountPath)
					}
				}
			}
		}
	}

	return nil
}

func (h *ConfigMapSetCreateUpdateHandler) getReloadSidecarName(ctx context.Context, cms *appsv1alpha1.ConfigMapSet) (string, error) {
	if cms.Spec.ReloadSidecarConfig == nil {
		return "", nil
	}
	config := cms.Spec.ReloadSidecarConfig.Config
	if config == nil {
		return "", nil
	}

	switch cms.Spec.ReloadSidecarConfig.Type {
	case appsv1alpha1.ReloadSidecarTypeK8s:
		return config.Name, nil
	case appsv1alpha1.ReloadSidecarTypeSidecarSet:
		if config.SidecarSetRef != nil {
			return config.SidecarSetRef.ContainerName, nil
		}
	case appsv1alpha1.ReloadSidecarTypeCustom:
		if config.ConfigMapRef != nil {
			cmNamespace := config.ConfigMapRef.Namespace
			if cmNamespace == "" {
				cmNamespace = cms.Namespace
			}
			customerCM := &corev1.ConfigMap{}
			err := h.Client.Get(ctx, types.NamespacedName{Name: config.ConfigMapRef.Name, Namespace: cmNamespace}, customerCM)
			if err != nil {
				return "", err
			}
			if containerData, exists := customerCM.Data["reload-sidecar"]; exists {
				var container corev1.Container
				if err := json.Unmarshal([]byte(containerData), &container); err == nil {
					if container.Name != "" {
						return container.Name, nil
					}
					return "reload-sidecar", nil
				}
			}
		}
	}
	return "", nil
}

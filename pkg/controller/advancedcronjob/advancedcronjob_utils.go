package advancedcronjob

import appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"

func FindTemplateKind(spec appsv1alpha1.AdvancedCronJobSpec) appsv1alpha1.TemplateKind {
	if spec.Template.JobTemplate != nil {
		return appsv1alpha1.JobTemplate
	}

	return appsv1alpha1.BroadcastJobTemplate
}

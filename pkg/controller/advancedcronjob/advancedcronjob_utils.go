package advancedcronjob

import (
	"fmt"
	"strings"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"k8s.io/klog/v2"
)

func FindTemplateKind(spec appsv1alpha1.AdvancedCronJobSpec) appsv1alpha1.TemplateKind {
	if spec.Template.JobTemplate != nil {
		return appsv1alpha1.JobTemplate
	}

	return appsv1alpha1.BroadcastJobTemplate
}

func formatSchedule(acj *appsv1alpha1.AdvancedCronJob) string {
	if strings.Contains(acj.Spec.Schedule, "TZ") {
		return acj.Spec.Schedule
	}
	if acj.Spec.TimeZone != nil {
		if _, err := time.LoadLocation(*acj.Spec.TimeZone); err != nil {
			klog.ErrorS(err, "Failed to load location for advancedCronJob", "location", *acj.Spec.TimeZone, "advancedCronJob", klog.KObj(acj))
			return acj.Spec.Schedule
		}
		return fmt.Sprintf("TZ=%s %s", *acj.Spec.TimeZone, acj.Spec.Schedule)
	}
	return acj.Spec.Schedule
}

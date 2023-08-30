package advancedcronjob

import (
	"fmt"
	"strings"
	"time"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"k8s.io/klog/v2"
)

func FindTemplateKind(spec appsv1beta1.AdvancedCronJobSpec) appsv1beta1.TemplateKind {
	if spec.Template.JobTemplate != nil {
		return appsv1beta1.JobTemplate
	}

	return appsv1beta1.BroadcastJobTemplate
}

func formatSchedule(acj *appsv1beta1.AdvancedCronJob) string {
	if strings.Contains(acj.Spec.Schedule, "TZ") {
		return acj.Spec.Schedule
	}
	if acj.Spec.TimeZone != nil {
		if _, err := time.LoadLocation(*acj.Spec.TimeZone); err != nil {
			klog.Errorf("Failed to load location %s for %s/%s: %v", *acj.Spec.TimeZone, acj.Namespace, acj.Name, err)
			return acj.Spec.Schedule
		}
		return fmt.Sprintf("TZ=%s %s", *acj.Spec.TimeZone, acj.Spec.Schedule)
	}
	return acj.Spec.Schedule
}

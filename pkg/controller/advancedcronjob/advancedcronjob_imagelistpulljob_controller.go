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

package advancedcronjob

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/robfig/cron/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/reference"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

func watchImageListPullJob(mgr manager.Manager, c controller.Controller) error {
	if err := c.Watch(source.Kind(mgr.GetCache(), &appsv1alpha1.ImageListPullJob{}, handler.TypedEnqueueRequestForOwner[*appsv1alpha1.ImageListPullJob](
		mgr.GetScheme(), mgr.GetRESTMapper(), &appsv1alpha1.AdvancedCronJob{}, handler.OnlyControllerOwner()))); err != nil {
		return err
	}

	return nil
}

func (r *ReconcileAdvancedCronJob) reconcileImageListPullJob(ctx context.Context, req ctrl.Request, advancedCronJob appsv1alpha1.AdvancedCronJob) (ctrl.Result, error) {
	klog.V(1).InfoS("Starting to reconcile ImageListPullJob", "advancedCronJob", req)

	advancedCronJob.Status.Type = appsv1alpha1.ImageListPullJobTemplate

	// list all the jobs
	childJobs := &appsv1alpha1.ImageListPullJobList{}

	if err := r.List(ctx, childJobs, client.InNamespace(req.Namespace), client.MatchingFields{jobOwnerKey: advancedCronJob.Name}); err != nil {
		klog.ErrorS(err, "Unable to list child ImageListPullJobs", "advancedCronJob", req)
		return ctrl.Result{}, err
	}

	// find the active list of jobs
	var activeJobs []*appsv1alpha1.ImageListPullJob
	var successfulJobs []*appsv1alpha1.ImageListPullJob
	var failedJobs []*appsv1alpha1.ImageListPullJob

	// helper function to check if an ImageListPullJob is finished
	isImageListPullJobFinished := func(job *appsv1alpha1.ImageListPullJob) (bool, string) {
		if job.Status.CompletionTime != nil {
			if job.Status.Succeeded == job.Status.Desired {
				return true, "Complete"
			} else {
				return true, "Failed"
			}
		}
		return false, ""
	}

	// helper function to get scheduled time for ImageListPullJob
	getScheduledTimeForImageListPullJob := func(job *appsv1alpha1.ImageListPullJob) (*time.Time, error) {
		timeRaw := job.Annotations[scheduledTimeAnnotation]
		if len(timeRaw) == 0 {
			return nil, nil
		}

		timeParsed, err := time.Parse(time.RFC3339, timeRaw)
		if err != nil {
			return nil, err
		}
		return &timeParsed, nil
	}

	var mostRecentTime *time.Time // find the last run so we can update the status
	for i, job := range childJobs.Items {
		_, finishedType := isImageListPullJobFinished(&job)
		switch finishedType {
		case "": // ongoing
			activeJobs = append(activeJobs, &childJobs.Items[i])
		case "Failed":
			failedJobs = append(failedJobs, &childJobs.Items[i])
		case "Complete":
			successfulJobs = append(successfulJobs, &childJobs.Items[i])
		}

		// We'll store the launch time in an annotation, so we'll reconstitute that from
		// the active jobs themselves.
		scheduledTimeForJob, err := getScheduledTimeForImageListPullJob(&job)
		if err != nil {
			klog.ErrorS(err, "Unable to parse schedule time for child ImageListPullJob", "imageListPullJob", klog.KObj(&job), "advancedCronJob", req)
			continue
		}
		if scheduledTimeForJob != nil {
			if mostRecentTime == nil {
				mostRecentTime = scheduledTimeForJob
			} else if mostRecentTime.Before(*scheduledTimeForJob) {
				mostRecentTime = scheduledTimeForJob
			}
		}
	}

	if mostRecentTime != nil {
		advancedCronJob.Status.LastScheduleTime = &metav1.Time{Time: *mostRecentTime}
	} else {
		advancedCronJob.Status.LastScheduleTime = nil
	}

	advancedCronJob.Status.Active = nil
	for _, activeJob := range activeJobs {
		jobRef, err := reference.GetReference(r.scheme, activeJob)
		if err != nil {
			klog.ErrorS(err, "Unable to make reference to active job", "job", klog.KObj(activeJob))
			continue
		}
		advancedCronJob.Status.Active = append(advancedCronJob.Status.Active, *jobRef)
	}

	klog.V(1).InfoS("Job count", "active jobs", len(activeJobs), "successful jobs", len(successfulJobs), "failed jobs", len(failedJobs), "advancedCronJob", req)

	if err := r.updateAdvancedJobStatus(req, &advancedCronJob); err != nil {
		klog.ErrorS(err, "Unable to update CronJob status", "advancedCronJob", req)
		return ctrl.Result{}, err
	}

	// Clean up old jobs according to the history limit
	if advancedCronJob.Spec.FailedJobsHistoryLimit != nil {
		sort.Slice(failedJobs, func(i, j int) bool {
			if failedJobs[i].Status.StartTime == nil {
				return failedJobs[j].Status.StartTime != nil
			}
			return failedJobs[i].Status.StartTime.Before(failedJobs[j].Status.StartTime)
		})
		for i, job := range failedJobs {
			if int32(i) >= int32(len(failedJobs))-*advancedCronJob.Spec.FailedJobsHistoryLimit {
				break
			}
			klog.V(1).InfoS("Cleaning up old failed job", "job", klog.KObj(job), "advancedCronJob", req)
			if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
				klog.ErrorS(err, "Unable to delete old failed job", "job", klog.KObj(job), "advancedCronJob", req)
			} else {
				r.recorder.Eventf(&advancedCronJob, "Normal", "SuccessfulDelete", "Deleted failed job %v", job.Name)
				klog.InfoS("Deleted old failed job", "job", klog.KObj(job), "advancedCronJob", req)
			}
		}
	}

	if advancedCronJob.Spec.SuccessfulJobsHistoryLimit != nil {
		sort.Slice(successfulJobs, func(i, j int) bool {
			if successfulJobs[i].Status.StartTime == nil {
				return successfulJobs[j].Status.StartTime != nil
			}
			return successfulJobs[i].Status.StartTime.Before(successfulJobs[j].Status.StartTime)
		})
		for i, job := range successfulJobs {
			if int32(i) >= int32(len(successfulJobs))-*advancedCronJob.Spec.SuccessfulJobsHistoryLimit {
				break
			}
			klog.V(1).InfoS("Cleaning up old successful job", "job", klog.KObj(job), "advancedCronJob", req)
			if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
				klog.ErrorS(err, "Unable to delete old successful job", "job", klog.KObj(job), "advancedCronJob", req)
			} else {
				r.recorder.Eventf(&advancedCronJob, "Normal", "SuccessfulDelete", "Deleted job %v", job.Name)
				klog.InfoS("Deleted old successful job", "job", klog.KObj(job), "advancedCronJob", req)
			}
		}
	}

	/* We'll start building our next run, by figuring out the next times that we need to create jobs */

	if advancedCronJob.Spec.Paused != nil && *advancedCronJob.Spec.Paused {
		klog.V(1).InfoS("CronJob paused, skipping", "advancedCronJob", req)
		return ctrl.Result{}, nil
	}

	// figure out the next times that we need to create jobs
	getNextSchedule := func(cronJob *appsv1alpha1.AdvancedCronJob, now time.Time) (lastMissed time.Time, next time.Time, err error) {
		sched, err := cron.ParseStandard(formatSchedule(cronJob))
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("unparsable schedule %q: %v", cronJob.Spec.Schedule, err)
		}

		// for optimization purposes, cheat a bit and start from our last observed run time
		// we could reconstitute this here, but there's not much point, since we've
		// just updated it.
		var earliestTime time.Time
		if cronJob.Status.LastScheduleTime != nil {
			earliestTime = cronJob.Status.LastScheduleTime.Time
		} else {
			earliestTime = cronJob.ObjectMeta.CreationTimestamp.Time
		}
		if cronJob.Spec.StartingDeadlineSeconds != nil {
			// controller is not going to schedule anything below this point
			schedulingDeadline := now.Add(-time.Second * time.Duration(*cronJob.Spec.StartingDeadlineSeconds))

			if schedulingDeadline.After(earliestTime) {
				earliestTime = schedulingDeadline
			}
		}
		if earliestTime.After(now) {
			return time.Time{}, sched.Next(now), nil
		}

		starts := 0
		for t := sched.Next(earliestTime); !t.After(now); t = sched.Next(t) {
			lastMissed = t
			// An object might miss several starts. For example, if
			// controller gets wedged on Friday at 5:01pm when everyone has
			// gone home, and someone comes in on Tuesday AM and discovers
			// the problem and restarts the controller, then all the hourly
			// jobs, more than 80 of them for one hourly scheduledJob, should
			// all start running with no further intervention (if the scheduledJob
			// allows concurrency and late starts).
			//
			// However, if there is a bug somewhere, or incorrect clock
			// on controller's server or apiservers (for setting creationTimestamp)
			// then there could be so many missed start times (it could be off
			// by decades or more), that it would eat up all the CPU and memory
			// of this controller. In that case, we want to not try to list
			// all the missed start times.
			starts++
			if starts > 100 {
				// We can't get the most recent times so just return an empty slice
				return time.Time{}, time.Time{}, fmt.Errorf("too many missed start times (> 100). Set or decrease .spec.startingDeadlineSeconds or check clock skew")
			}
		}
		return lastMissed, sched.Next(now), nil
	}

	// figure out the next times that we need to create jobs
	missedRun, nextRun, err := getNextSchedule(&advancedCronJob, r.Now())
	if err != nil {
		klog.ErrorS(err, "Unable to figure out CronJob schedule", "advancedCronJob", req)
		// we don't really care about requeuing until we get an update that
		// fixes the schedule, so don't return an error
		return ctrl.Result{}, nil
	}

	scheduledResult := ctrl.Result{RequeueAfter: nextRun.Sub(r.Now())} // save this so we can re-use it elsewhere
	klog.V(1).InfoS("Time info", "now", r.Now(), "next run", nextRun, "last run", advancedCronJob.Status.LastScheduleTime, "advancedCronJob", req)

	if missedRun.IsZero() {
		klog.V(1).InfoS("No upcoming scheduled times, sleeping until next run", "now", r.Now(), "nextRun", nextRun, "advancedCronJob", req)
		return scheduledResult, nil
	}

	// make sure we're not too late to start the run
	tooLate := false
	if advancedCronJob.Spec.StartingDeadlineSeconds != nil {
		tooLate = missedRun.Add(time.Duration(*advancedCronJob.Spec.StartingDeadlineSeconds) * time.Second).Before(r.Now())
	}
	if tooLate {
		klog.V(1).InfoS("Missed starting deadline for last run, sleeping till next run", "missedRun", missedRun, "advancedCronJob", req)
		return scheduledResult, nil
	}

	// figure out how to run this job -- concurrency policy might forbid us from running
	// multiple at the same time...
	if advancedCronJob.Spec.ConcurrencyPolicy == appsv1alpha1.ForbidConcurrent && len(activeJobs) > 0 {
		klog.V(1).InfoS("Concurrency policy blocks concurrent runs, skipping", "active jobs", len(activeJobs), "advancedCronJob", req)
		return scheduledResult, nil
	}

	// ...or instruct us to replace existing ones.
	if advancedCronJob.Spec.ConcurrencyPolicy == appsv1alpha1.ReplaceConcurrent {
		for _, activeJob := range activeJobs {
			klog.V(1).InfoS("Replacing currently running job", "job", klog.KObj(activeJob), "advancedCronJob", req)
			// we don't care if the job was already deleted
			if err := r.Delete(ctx, activeJob, client.PropagationPolicy(metav1.DeletePropagationBackground)); client.IgnoreNotFound(err) != nil {
				klog.ErrorS(err, "Unable to delete active job", "job", klog.KObj(activeJob), "advancedCronJob", req)
				return ctrl.Result{}, err
			}
		}
	}

	constructImageListPullJobForCronJob := func(advancedCronJob *appsv1alpha1.AdvancedCronJob, scheduledTime time.Time) (*appsv1alpha1.ImageListPullJob, error) {
		// We want job names for a given nominal start time to have a deterministic name to avoid the same job being created twice
		name := fmt.Sprintf("%s-%d", advancedCronJob.Name, scheduledTime.Unix())

		job := &appsv1alpha1.ImageListPullJob{
			ObjectMeta: metav1.ObjectMeta{
				Labels:      make(map[string]string),
				Annotations: make(map[string]string),
				Name:        name,
				Namespace:   advancedCronJob.Namespace,
			},
			Spec: *advancedCronJob.Spec.Template.ImageListPullJobTemplate.Spec.DeepCopy(),
		}
		for k, v := range advancedCronJob.Spec.Template.ImageListPullJobTemplate.Annotations {
			job.Annotations[k] = v
		}
		job.Annotations[scheduledTimeAnnotation] = scheduledTime.Format(time.RFC3339)
		for k, v := range advancedCronJob.Spec.Template.ImageListPullJobTemplate.Labels {
			job.Labels[k] = v
		}
		if err := ctrl.SetControllerReference(advancedCronJob, job, r.scheme); err != nil {
			return nil, err
		}

		return job, nil
	}

	// actually make the job...
	job, err := constructImageListPullJobForCronJob(&advancedCronJob, missedRun)
	if err != nil {
		klog.ErrorS(err, "Unable to construct job from template", "advancedCronJob", req)
		// don't bother requeuing until we get a change to the spec
		return scheduledResult, nil
	}

	// ...and create it on the cluster
	if err := r.Create(ctx, job); err != nil {
		klog.ErrorS(err, "Unable to create ImageListPullJob for CronJob", "job", klog.KObj(job), "advancedCronJob", req)
		return ctrl.Result{}, err
	}

	klog.V(1).InfoS("Created ImageListPullJob for CronJob run", "job", klog.KObj(job), "advancedCronJob", req)
	r.recorder.Eventf(&advancedCronJob, "Normal", "SuccessfulCreate", "Created job %v", job.Name)

	// we'll requeue once we see the running job, and update our status
	return scheduledResult, nil
}

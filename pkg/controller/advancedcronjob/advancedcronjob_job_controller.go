/*
Copyright 2020 The Kruise Authors.
Copyright 2016 The Kubernetes Authors.
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
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ref "k8s.io/client-go/tools/reference"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

func watchJob(mgr manager.Manager, c controller.Controller) error {
	if err := c.Watch(source.Kind(mgr.GetCache(), &batchv1.Job{}, handler.TypedEnqueueRequestForOwner[*batchv1.Job](
		mgr.GetScheme(), mgr.GetRESTMapper(), &appsv1alpha1.BroadcastJob{}, handler.OnlyControllerOwner()))); err != nil {
		return err
	}

	return nil
}

// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs/status,verbs=get;update;patch

func (r *ReconcileAdvancedCronJob) reconcileJob(ctx context.Context, req ctrl.Request, advancedCronJob appsv1alpha1.AdvancedCronJob) (ctrl.Result, error) {
	advancedCronJob.Status.Type = appsv1alpha1.JobTemplate
	var childJobs batchv1.JobList
	if err := r.List(ctx, &childJobs, client.InNamespace(advancedCronJob.Namespace), client.MatchingFields{jobOwnerKey: advancedCronJob.Name}); err != nil {
		klog.ErrorS(err, "Unable to list child Jobs", "advancedCronJob", req)
		return ctrl.Result{}, err
	}

	var activeJobs []*batchv1.Job
	var successfulJobs []*batchv1.Job
	var failedJobs []*batchv1.Job
	var mostRecentTime *time.Time
	isJobFinished := func(job *batchv1.Job) (bool, batchv1.JobConditionType) {
		for _, c := range job.Status.Conditions {
			if (c.Type == batchv1.JobComplete || c.Type == batchv1.JobFailed) && c.Status == corev1.ConditionTrue {
				return true, c.Type
			}
		}

		return false, ""
	}

	// +kubebuilder:docs-gen:collapse=isJobFinished
	getScheduledTimeForJob := func(job *batchv1.Job) (*time.Time, error) {
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

	// +kubebuilder:docs-gen:collapse=getScheduledTimeForJob

	for i, job := range childJobs.Items {
		_, finishedType := isJobFinished(&job)
		switch finishedType {
		case "": // ongoing
			activeJobs = append(activeJobs, &childJobs.Items[i])
		case batchv1.JobFailed:
			failedJobs = append(failedJobs, &childJobs.Items[i])
		case batchv1.JobComplete:
			successfulJobs = append(successfulJobs, &childJobs.Items[i])
		}

		// We'll store the launch time in an annotation, so we'll reconstitute that from
		// the active jobs themselves.
		scheduledTimeForJob, err := getScheduledTimeForJob(&job)
		if err != nil {
			klog.ErrorS(err, "Unable to parse schedule time for child job", "job", klog.KObj(&job), "advancedCronJob", req)
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
		jobRef, err := ref.GetReference(r.scheme, activeJob)
		if err != nil {
			klog.ErrorS(err, "Unable to make reference to active job", "job", klog.KObj(activeJob), "advancedCronJob", req)
			continue
		}
		advancedCronJob.Status.Active = append(advancedCronJob.Status.Active, *jobRef)
	}

	klog.V(1).InfoS("Job count", "activeJobCount", len(activeJobs), "successfulJobCount", len(successfulJobs), "failedJobCount", len(failedJobs), "advancedCronJob", req)
	if err := r.updateAdvancedJobStatus(req, &advancedCronJob); err != nil {
		klog.ErrorS(err, "Unable to update AdvancedCronJob status", "advancedCronJob", req)
		return ctrl.Result{}, err
	}

	/*
		Once we've updated our status, we can move on to ensuring that the status of
		the world matches what we want in our spec.
		### 3: Clean up old jobs according to the history limit
		First, we'll try to clean up old jobs, so that we don't leave too many lying
		around.
	*/

	// NB: deleting these is "best effort" -- if we fail on a particular one,
	// we won't requeue just to finish the deleting.
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
			if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); client.IgnoreNotFound(err) != nil {
				klog.ErrorS(err, "Unable to delete old failed job", "job", klog.KObj(job), "advancedCronJob", req)
			} else {
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
			if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); (err) != nil {
				klog.ErrorS(err, "Unable to delete old successful job", "job", klog.KObj(job), "advancedCronJob", req)
			} else {
				klog.InfoS("Deleted old successful job", "job", klog.KObj(job), "advancedCronJob", req)
			}
		}
	}

	/* ### 4: Check if we're suspended
	If this object is suspended, we don't want to run any jobs, so we'll stop now.
	This is useful if something's broken with the job we're running and we want to
	pause runs to investigate or putz with the cluster, without deleting the object.
	*/

	if advancedCronJob.Spec.Paused != nil && *advancedCronJob.Spec.Paused {
		klog.V(1).InfoS("CronJob paused, skipping", "advancedCronJob", req)
		return ctrl.Result{}, nil
	}

	/*
		### 5: Get the next scheduled run
		If we're not paused, we'll need to calculate the next scheduled run, and whether
		or not we've got a run that we haven't processed yet.
	*/

	/*
		We'll calculate the next scheduled time using our helpful cron library.
		We'll start calculating appropriate times from our last run, or the creation
		of the CronJob if we can't find a last run.
		If there are too many missed runs and we don't have any deadlines set, we'll
		bail so that we don't cause issues on controller restarts or wedges.
		Otherwise, we'll just return the missed runs (of which we'll just use the latest),
		and the next run, so that we can know when it's time to reconcile again.
	*/
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
	// +kubebuilder:docs-gen:collapse=getNextSchedule

	// figure out the next times that we need to create
	// jobs at (or anything we missed).
	now := realClock{}.Now()
	missedRun, nextRun, err := getNextSchedule(&advancedCronJob, now)
	if err != nil {
		klog.ErrorS(err, "Unable to figure out CronJob schedule", "advancedCronJob", req)
		// we don't really care about requeuing until we get an update that
		// fixes the schedule, so don't return an error
		return ctrl.Result{}, nil
	}

	/*
		We'll prep our eventual request to requeue until the next job, and then figure
		out if we actually need to run.
	*/
	scheduledResult := ctrl.Result{RequeueAfter: nextRun.Sub(now)} // save this so we can re-use it elsewhere

	/*
		### 6: Run a new job if it's on schedule, not past the deadline, and not blocked by our concurrency policy
		If we've missed a run, and we're still within the deadline to start it, we'll need to run a job.
	*/
	if missedRun.IsZero() {
		klog.V(1).InfoS("No upcoming scheduled times, sleeping until next run", "now", now, "nextRun", nextRun, "advancedCronJob", req)
		return scheduledResult, nil
	}

	// make sure we're not too late to start the run
	tooLate := false
	if advancedCronJob.Spec.StartingDeadlineSeconds != nil {
		tooLate = missedRun.Add(time.Duration(*advancedCronJob.Spec.StartingDeadlineSeconds) * time.Second).Before(now)
	}
	if tooLate {
		klog.V(1).InfoS("Missed starting deadline for last run, sleeping till next run", "missedRun", missedRun, "advancedCronJob", req)
		return scheduledResult, nil
	}

	/*
		If we actually have to run a job, we'll need to either wait till existing ones finish,
		replace the existing ones, or just add new ones.  If our information is out of date due
		to cache delay, we'll get a requeue when we get up-to-date information.
	*/
	// figure out how to run this job -- concurrency policy might forbid us from running
	// multiple at the same time...
	if advancedCronJob.Spec.ConcurrencyPolicy == appsv1alpha1.ForbidConcurrent && len(activeJobs) > 0 {
		klog.V(1).InfoS("Concurrency policy blocks concurrent runs, skipping", "activeJobCount", len(activeJobs), "advancedCronJob", req)
		return scheduledResult, nil
	}

	// ...or instruct us to replace existing ones...
	if advancedCronJob.Spec.ConcurrencyPolicy == appsv1alpha1.ReplaceConcurrent {
		for _, activeJob := range activeJobs {
			// we don't care if the job was already deleted
			if err := r.Delete(ctx, activeJob, client.PropagationPolicy(metav1.DeletePropagationBackground)); client.IgnoreNotFound(err) != nil {
				klog.ErrorS(err, "Unable to delete active job", "job", klog.KObj(activeJob), "advancedCronJob", req)
				return ctrl.Result{}, err
			}
		}
	}

	/*
		Once we've figured out what to do with existing jobs, we'll actually create our desired job
		We need to construct a job based on our AdvancedCronJob's template.  We'll copy over the spec
		from the template and copy some basic object meta.
		Then, we'll set the "scheduled time" annotation so that we can reconstitute our
		`LastScheduleTime` field each reconcile.
		Finally, we'll need to set an owner reference.  This allows the Kubernetes garbage collector
		to clean up jobs when we delete the CronJob, and allows controller-runtime to figure out
		which cronjob needs to be reconciled when a given job changes (is added, deleted, completes, etc).
	*/
	constructJobForCronJob := func(advancedCronJob *appsv1alpha1.AdvancedCronJob, scheduledTime time.Time) (*batchv1.Job, error) {
		// We want job names for a given nominal start time to have a deterministic name to avoid the same job being created twice
		name := fmt.Sprintf("%s-%d", advancedCronJob.Name, scheduledTime.Unix())

		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Labels:      make(map[string]string),
				Annotations: make(map[string]string),
				Name:        name,
				Namespace:   advancedCronJob.Namespace,
			},
			Spec: *advancedCronJob.Spec.Template.JobTemplate.Spec.DeepCopy(),
		}
		for k, v := range advancedCronJob.Spec.Template.JobTemplate.Annotations {
			job.Annotations[k] = v
		}
		job.Annotations[scheduledTimeAnnotation] = scheduledTime.Format(time.RFC3339)
		for k, v := range advancedCronJob.Spec.Template.JobTemplate.Labels {
			job.Labels[k] = v
		}
		if err := ctrl.SetControllerReference(advancedCronJob, job, r.scheme); err != nil {
			return nil, err
		}

		return job, nil
	}
	// +kubebuilder:docs-gen:collapse=constructJobForCronJob

	// actually make the job...
	job, err := constructJobForCronJob(&advancedCronJob, missedRun)
	if err != nil {
		klog.ErrorS(err, "Unable to construct job from template", "advancedCronJob", req)
		// don't bother requeuing until we get a change to the spec
		return scheduledResult, nil
	}

	// ...and create it on the cluster
	if err := r.Create(ctx, job); err != nil {
		klog.ErrorS(err, "Unable to create Job for AdvancedCronJob", "job", klog.KObj(job), "advancedCronJob", req)
		return ctrl.Result{}, err
	}

	klog.V(1).InfoS("Created Job for AdvancedCronJob run", "job", klog.KObj(job), "advancedCronJob", req)

	/*
		### 7: Requeue when we either see a running job or it's time for the next scheduled run
		Finally, we'll return the result that we prepped above, that says we want to requeue
		when our next run would need to occur.  This is taken as a maximum deadline -- if something
		else changes in between, like our job starts or finishes, we get modified, etc, we might
		reconcile again sooner.
	*/
	// we'll requeue once we see the running job, and update our status
	return scheduledResult, nil
}

---
title: AdvancedCronJob Crd and Controller
authors:
  - "@rishi-anand"
reviewers:
  - "@Fei-Guo"
  - "@FillZpp"
  - "@jzhoucliqr"
creation-date: 2020-10-25
last-updated: 2020-10-25
status: implementable
---

# Implementing AdvancedCronJob Crd and Controller
- Implementing AdvancedCronJob to support Job/BroadcastJob or any other future CRD and to run it periodically at a given schedule. 

## Table of Contents

A table of contents is helpful for quickly jumping to sections of a proposal and for highlighting
any additional information provided beyond the standard proposal template.
[Tools for generating](https://github.com/ekalinin/github-markdown-toc) a table of contents from markdown are available.

- [Title](#title)
  - [Table of Contents](#table-of-contents)
  - [Summary](#summary)
  - [Motivation](#motivation)
    - [Goals](#goals)
  - [Proposal](#proposal)
    - [User Stories](#user-stories)
      - [Story 1](#story-1)
      - [Story 2](#story-2)
  - [Implementation History](#implementation-history)

## Summary

This controller will be very generic and will have implementations to help developers to run a job or any CRD in specific schedule.

## Motivation

- Developer may come across a use-case when some job needs to be executed on at a specific schedule.
- Found same use-case requirement in Issues #251 and I got motivated to implement it

### Goals

- Implementing a custom controller for AdvancedCronJob which acts like CronJob but it schedules Job/BroadcastJob or other CRD

## Proposal

- Adding a new CRD and controller for AdvancedCronJob

### User Stories

- Implement a CRD which contains below fields and a controller which honors all the fields and reconciles accordingly.
```
apiVersion: apps.kruise.io/v1alpha1
kind: AdvancedCronJob
metadata:
  name: AdvancedCronJob-sample
spec:
  schedule: "* * * * *"
  concurrencyPolicy: Replace
  paused: false
  successfulJobsHistoryLimit: 3
  failedJobsHistoryLimit: 3
  jobTemplate: #user can provide only one template at a time
  broadcastJobTemplate:
    spec:
      template:
        spec:
          containers:
            - name: hello
              image: busybox
              args:
                - /bin/sh
                - -c
                - date; echo "Hello from the AdvancedCronJob - SpectroCloud"
          restartPolicy: Never
      completionPolicy:
        type: Always
        activeDeadlineSeconds: 200
```

#### Story 1
Create a above CRD and implement the controller

#### Story 2
Add unit and integration test cases

## Implementation History

- [ ] 10/24/2020: Proposal discussion in an issue <a href="https://github.com/openkruise/kruise/issues/215#issuecomment-715506813">#215</a>
- [ ] 11/04/2020: Proposal submission

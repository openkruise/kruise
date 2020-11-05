---
title: BroadcastCronJob Crd and Controller
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

# Implementing BroadcastCronJob Crd and Controller
- Running BroadcastJob periodically at a given schedule. 

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

This controller will help developers to run a job in specific schedule and on to all nodes in the cluster.

## Motivation

- Developer may come across a use-case when some job needs to be executed on all the nodes and with some specific schedule.
- Found same use-case requirement in Issues #251 and I got motivated to implement it

### Goals

- Implementing a custom controller for BroadcastCronJob which acts like CronJob but it schedules BroadcastJob

## Proposal

- Adding a new CRD and controller for BroadcastCronJob

### User Stories

- Implement a CRD which contains below fields and a controller which honors all the fields and reconciles accordingly.
```
apiVersion: apps.kruise.io/v1alpha1
kind: BroadcastCronJob
metadata:
  name: broadcastcronjob-sample
spec:
  schedule: "* * * * *"
  concurrencyPolicy: Replace
  paused: false
  successfulJobsHistoryLimit: 3
  failedJobsHistoryLimit: 3
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
                - date; echo "Hello from the BroadcastCronJob - SpectroCloud"
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

- [ ] 10/24/2020: Proposed idea in an issue <a href="https://github.com/openkruise/kruise/issues/215#issuecomment-715506813">#215</a>
- [ ] 10/25/2020: Created PR for CRD and implementation of controller <a href="https://github.com/spectrocloud/kruise/pull/1">spectrocloud/kruise/pull/1</a>


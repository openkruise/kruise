---
title: Apply SidecarSet Update Strategy In The Injection Phase
authors:
  - "@lovecrazy"
reviewers:
  - ""
creation-date: 2021-10-31
last-updated: 2021-10-31
status: provisional
see-also:
  - "https://github.com/openkruise/kruise/issues/783"
  - "https://github.com/openkruise/kruise/pull/715"
# replaces:
#   - "/docs/proposals/20181231-replaced-proposal.md"
# superseded-by:
#   - "/docs/proposals/20190104-superceding-proposal.md"
---

# SidecarSet Update Strategy

- Apply SidecarSet update strategy in the injection phase

## Table of Contents

A table of contents is helpful for quickly jumping to sections of a proposal and for highlighting
any additional information provided beyond the standard proposal template.
[Tools for generating](https://github.com/ekalinin/github-markdown-toc) a table of contents from markdown are available.

* [SidecarSet Update Strategy](#sidecarset-update-strategy)
   * [Table of Contents](#table-of-contents)
   * [Glossary](#glossary)
   * [Motivation](#motivation)
      * [Goals](#goals)
   * [Proposal](#proposal)
      * [execUpdateStrategy](#execupdatestrategy)
      * [Implementation Details/Notes/Constraints](#implementation-detailsnotesconstraints)
   * [Alternatives](#alternatives)
      * [Add a new updateStrategy.type](#add-a-new-updatestrategytype)
      * [Add a new field in updateStrategy](#add-a-new-field-in-updatestrategy)
   * [Implementation History](#implementation-history)

## Glossary

Refer to the [Cluster API Book Glossary](https://cluster-api.sigs.k8s.io/reference/glossary.html).

If this proposal adds new terms, or defines some, make the changes to the book's glossary when in PR stage.

## Motivation

If we use a Deployment to deploy our application. At the same time, use SidecarSet to inject a container which collect application's log.

Someday, we want to update or replace our SidecarSet. We are very careful, only update 10% SidecarSet per step. It will take a long time to complete the upgrade. Suddenly, we find a big problem in our application. So, we need to fix and update application immediately.

Unfortunately, we cann't do this. Because this will cause all SidecarSet to be upgraded.

In a large enterprise, it is usually divided into basic platform and business platform. One maintain SidecarSet, Another maintain application. This makes the above situation even more serious.

### Goals

- Apply SidecarSet's upgrade strategy in the injection phase

## Proposal

As shown below:

```yaml
# sidecarset.yaml
apiVersion: apps.kruise.io/v1alpha1
kind: SidecarSet
metadata:
  name: test-sidecarset
spec:
  selector:
    matchLabels:
      sidecar: test-sidecarset
  updateStrategy:
    type: RollingUpdate
    maxUnavailable: 1
  injectionStrategy:
    execUpdateStrategy: true
  containers:
  - name: sidecar1
    image: centos:6.7
    command: ["sleep", "999d"] # do nothing at all
```

### execUpdateStrategy

* `true` means that we will apply update strategy in the injection phase.
* `false` is for compatible with the old version, no behavior will be changed.

> default is `false`.

When execUpdateStrategy is `true`, injection phase will use `updateStrategy.selector` and `updateStrategy.partition` to decide which version container will injected.

> `updateStrategy.scatterStrategy` may support in the future.

__How to decide the version of SidecarSet?__

We use `https://github.com/openkruise/kruise/pull/715` to decide is there an old version of SidecarSet.

If old version exists, we inject current and old container following by `updateStrategy`'s definition.

### Implementation Details/Notes/Constraints

When we set `injectionStrategy.execUpdateStrategy` == `true`, we can not make all the properties in the `updateStrategy` take effect.

* `updateStrategy.type` and `updateStrategy.maxUnavailable` will be decided by another workload, like CloneSet or Deployment.
* `updateStrategy.paused` is a little conflict with `injectionStrategy.paused`, so we decided to make `injectionStrategy.paused` take effect. Because that sound more reasonable.

## Alternatives

### Make the above proposal the default implementation

Do not add any new field, just make the above behavior the default behavior.

The problem is that it will be inconsistent with the behavior of the previous version.

### Add a new `updateStrategy.type`

Add a new `updateStrategy.type` sound more reasonable. But it is difficult to decide what behavior to take, and can make other value more confused.

The current situation is as follows:

* `NotUpdate` means not update SidecarSet, even if yaml is changed. But will continue inject when a new pod is created.
* `RollingUpdate` means update SidecarSet immediately. Of course, if we set `maxUnavailable` will use the strategy. But it still inject current version of SidecarSet when a new pod is created.

So, we can't find a orthogonal behavior with above value.

### Add a new field in `updateStrategy`

Add a new field in `updateStrategy` like `injectionEffect`, value can be `InjectionOnly`/`UpdateOnly`/`InjectionAndUpdate`, and default is `UpdateOnly`.

The behavior of this new field is confusing when used together with the `updateStrategy.type`, and too complex.

## Implementation History

- [x] 10/31/2021: Proposal submission

# Release Process

The release process of a new version of Kruise involves the following:

*(Currently only the [maintainers](https://github.com/openkruise/community/blob/master/MAINTAINERS.md) are eligible to release a new version)*

## 0. Prerequisites

Look at [the last release](https://github.com/openkruise/kruise/releases/latest) in the releases page:

- For example, at the time of writing, it was v1.2.0
- The next version will thus be v1.3.0

## 1. Changelog

Add a new section in [CHANGELOG.md](./CHANGELOG.md) for the new version that is being released along with the new features, patches and deprecations it introduces.

It should not include every single change but solely what matters to our customers, for example issue template that has changed is not important.

## 2. Publish documentation for new version

Publish documentation for new version on [https://openkruise.io](https://openkruise.io).

Fork [openkruise/openkruise.io](https://github.com/openkruise/openkruise.io), create a version from current using `yarn run docusaurus docs:version <VERSION>`.

## 3. Create Kruise release on GitHub

Creating a new release in the [releases page](https://github.com/openkruise/kruise/releases) will trigger a GitHub workflow which will create a new image with the latest code and tagged with the next version (in this example v1.3.0).

## 4. Release template

Every release should use the template provided below to create the GitHub release.

Here's the template:

```
#### To install or upgrade to the old version, see [installation doc](https://openkruise.io/docs/installation/).

## Changes since v1.2.0

### New CRD and Controller: XXX

### CloneSet

### XXX

### Others

Thanks to all our contributors! 😊
```

## 5. Prepare our Helm Chart

Before we can release our new Helm chart version, we need to prepare it:

1. Create a new chart version with the updated version and appVersion in our [chart repository](https://github.com/openkruise/charts/tree/master/versions/kruise).
2. Update the CRDs & Kubernetes resources based on the release artifact (YAML)

## 6. Ship new Helm chart

Submit a PR to merge the new release,
and then the [Publish action](https://github.com/openkruise/charts/actions/workflows/publish.yaml) will automatically package and publish it.

## 7. Prepare next release

As per our [release governance](./RELEASES.md), we need to create a new shipping cycle in our project settings with a target date in 2 to 3 months after the last cycle.

Lastly, a new [milestone](https://github.com/openkruise/kruise/milestones) should be created to maintain the changes of next release.

## 8. Announcement

Announce the new release in Slack channel, DingTalk and WeChat groups.

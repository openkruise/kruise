# Security Policy

## Supported Versions

Kruise commits to supporting the n-2 version minor version of the current major release;
as well as the last minor version of the previous major release.

Here's an overview:

| Version | Supported           |
| ------- | ------------------- |
| 1.16.x   | :white_check_mark: |
| 1.15.x   | :white_check_mark:  |
| 1.14.x   | :white_check_mark:  |
| < 1.14   | :x:                 |

## Prevention

Container images are scanned in every pull request (PR) with [Snyk](https://snyk.io/) to detect new vulnerabilities.

Kruise maintainers are working to improve our prevention by adding additional measures:

- Scan code in master/nightly build and PR/master/nightly for Go.
- Scan published container images on GitHub Container Registry.

## Disclosures

We strive to ship secure software, but we need the community to help us find security breaches.

In case of a confirmed breach, reporters will get full credit and can be keep in the loop, if preferred.

DO NOT CREATE AN ISSUE to report a security problem. Instead, please send an email to kubernetes-security@service.aliyun.com

### Compensation

We do not provide compensations for reporting vulnerabilities except for eternal gratitude.

## Communication

[GitHub Security Advisory](https://github.com/openkruise/kruise/security/advisories) will be used to communicate during the process of identifying, fixing & shipping the mitigation of the vulnerability.

The advisory will only be made public when the patched version is released to inform the community of the breach and its potential security impact.

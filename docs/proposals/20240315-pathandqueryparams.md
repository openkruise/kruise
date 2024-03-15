---
title: Add Path and QueryParams support to trafficRoutings
authors:
  - "@lujiajing1126"
reviewers:
  - "@YYY"
creation-date: 2024-03-15
last-updated: 2024-03-15
status: implementable
---

# Add Path and QueryParams support to trafficRoutings

Support Path and QueryParams as HttpMatch conditions.

## Table of Contents

- [Add Path and QueryParams support to trafficRoutings](#add-path-and-queryparams-support-to-trafficroutings)
  - [Table of Contents](#table-of-contents)
  - [Motivation](#motivation)
    - [Goals](#goals)
    - [Non-Goals/Future Work](#non-goalsfuture-work)
  - [Proposal](#proposal)
    - [User Stories](#user-stories)
    - [Implementation Details/Notes/Constraints](#implementation-detailsnotesconstraints)
	  - [MSE Ingress](#mse-ingress)
	  - [Gateway API](#gateway-api)
  - [Additional Details](#additional-details)
    - [Test Plan [optional]](#test-plan-optional)
  - [Implementation History](#implementation-history)

## Motivation

So far, OpenKruise Rollouts simply supports `Header` as the matchers for traffic routing. Path and QueryParams are also commonly used for traffic routing in A/B testing scenarios,

- Path can be used for A/B testing at API/RPC level,
- QueryParams can be used in A/B testing for HTMLs and other cases where Header is not under control

### Goals

- Support Path and/or QueryParams for Gateway API, Istio Mesh and Ingress (Specfically, MSE and Gateway API)

### Non-Goals/Future Work

- `SourceLabels` and `SourceNamespace` are also commonly used, but only applicable to interservice communication. It can be left in the future.

## Proposal

The following fields are necessary in the `HttpRouteMatch` struct,

| FieldName | Type | Description |
| --------- | ---- | ----------- |
| Path | *gatewayv1beta1.HTTPPathMatch | Path specifies a HTTP request path matcher. |
| QueryParams | []gatewayv1beta1.HTTPQueryParamMatch | QueryParams specifies HTTP query parameter matchers. Multiple match values are ANDed together, meaning, a request must match all the specified query parameters to select the route. |

```go
type HttpRouteMatch struct {
	// New

	// Path specifies a HTTP request path matcher.
	// Supported list:
	// - Istio: https://istio.io/latest/docs/reference/config/networking/virtual-service/#HTTPMatchRequest
	//
	// +optional
	Path *gatewayv1beta1.HTTPPathMatch `json:"path,omitempty"`

	// QueryParams specifies HTTP query parameter matchers. Multiple match
	// values are ANDed together, meaning, a request must match all the
	// specified query parameters to select the route.
	// Supported list:
	// - Istio: https://istio.io/latest/docs/reference/config/networking/virtual-service/#HTTPMatchRequest
	// - MSE Ingress: https://help.aliyun.com/zh/ack/ack-managed-and-ack-dedicated/user-guide/annotations-supported-by-mse-ingress-gateways-1
	//   Header/Cookie > QueryParams
	// - Gateway API
	//
	// +listType=map
	// +listMapKey=name
	// +optional
	// +kubebuilder:validation:MaxItems=16
	QueryParams []gatewayv1beta1.HTTPQueryParamMatch `json:"queryParams,omitempty"`

	// Existing

  // Headers specifies HTTP request header matchers. Multiple match values are
	// ANDed together, meaning, a request must match all the specified headers
	// to select the route.
	//
	// +listType=map
	// +listMapKey=name
	// +optional
	// +kubebuilder:validation:MaxItems=16
	Headers []gatewayv1beta1.HTTPHeaderMatch `json:"headers,omitempty"`
}
```

Matches define conditions used to match incoming HTTP requests to the canary service. Each match condition may contain serveral conditions as children which are independent of each other.

For Gateway API, only a single match rule will be applied since Gateway API use ANDed rules if multiple ones are defined, i.e. the match will evaluate to be true only if all conditions are satisfied. Priority: Header > QueryParams, for backwards-compatibility.

Only one of the `weight` and `matches` will come into effect. If both fields are configured, then matches takes precedence.

### User Stories

Currently, only one trafficRouting provider is supported. Then we may consider them case by case,

- MSE Ingress: User can define queryParams, but it **SHOULD NOT** be used together with Headers due to priority and backward-compatibility.
- Istio: User can define path and/or queryParams and use without any issue.
- Gateway API: User can define path and/or queryParams. But one of them can take effect if and only if Headers are not defined.
- 

### Implementation Details/Notes/Constraints

#### MSE Ingress

Due to backward-compatibility, if both Headers and QueryParams are defined, Headers should be used.

Then according to the documentation of [MSE Ingress](https://help.aliyun.com/zh/ack/ack-managed-and-ack-dedicated/user-guide/advanced-usage-of-mse-ingress?spm=a2c4g.11186623.0.0.58a26dc4q4GCmn#p-qar-ac2-lvw), if Header and Query Parameter are combined, both conditions MUST be fulfilled.

So the priority in Rollouts is Header(Cookie) > QueryParams.

#### Gateway API

Since Gateway API uses logical AND for all matches, only a single match condition will be selected.

For backwards-comptability, the priority in Rollouts is Header(Cookie) > Path > QueryParams.

## Additional Details

### Test Plan [optional]

## Implementation History

- [ ] MM/DD/YYYY: Proposed idea in an issue or [community meeting]
- [ ] MM/DD/YYYY: Compile a Google Doc following the CAEP template (link here)
- [ ] MM/DD/YYYY: First round of feedback from community
- [ ] MM/DD/YYYY: Present proposal at a [community meeting]
- [ ] MM/DD/YYYY: Open proposal PR


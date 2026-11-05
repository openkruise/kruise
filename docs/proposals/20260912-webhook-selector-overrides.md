---
title: Webhook Selector Overrides
authors:
  - "@CharlesQQ"
creation-date: 2026-09-12
last-updated: 2026-09-14
status: implementable
---

# Webhook Selector Overrides

## Summary

Keep the current admission webhook selectors by default. Allow administrators to replace
the namespaceSelector and/or objectSelector of a named webhook explicitly. Overrides
modify existing entries; they do not append additional entries or retain a parallel
default entry that still matches ordinary business Pods.

## Motivation and user case

An operator uses ADS and SidecarSet to hot-upgrade dind-service in kube-system.
Only Pods labeled app=dind-service and dind-service-hot-upgrade=true need this
injection. The default mpod.kb.io scope excludes kube-system but admits Pods in
ordinary namespaces. With failurePolicy=Fail, a webhook outage can therefore
prevent ordinary business Pods from being created.

Appending a second, narrowly scoped webhook for dind-service does not solve this
problem: the original broad entry is still invoked independently. Filtering only
inside the SidecarSet handler also cannot help when that handler is unavailable.
The operator needs to replace the original entry's selectors so the API server
skips this webhook for nonmatching Pods before attempting a network call.

## Goals

- Preserve the default selectors when no override is configured.
- Replace explicitly supplied selector fields on named mutating or validating webhooks.
- Keep webhook names, order, number, rules, client configuration, failure policy,
  timeout, admission review versions and other fields unchanged by the override.
- Restore template selectors when the corresponding override is removed.
- Reconcile overrides independently of the controller-owned template annotation.

## Non-goals

- Appending or cloning webhook entries.
- Arbitrary changes to rules, URLs, failure policies or timeouts.
- Automatically deriving admission scopes from SidecarSets.
- Guaranteeing availability for requests matched by other webhook entries.
- Making Pod labels a security boundary: this is an administrator-configured opt-in scope.

## Configuration

Enable the alpha feature gate on kruise-manager:

```text
--feature-gates=WebhookSelectorOverrides=true
```

The gate defaults to false. Without the annotation, existing behavior is unchanged
regardless of the gate. A nonempty annotation with the gate disabled is rejected
during reconciliation instead of silently applying a broader default scope.

Set webhook.kruise.io/selector-overrides on the relevant
MutatingWebhookConfiguration or ValidatingWebhookConfiguration:

```yaml
metadata:
  annotations:
    webhook.kruise.io/selector-overrides: |
      [
        {
          "targetWebhook": "mpod.kb.io",
          "namespaceSelector": {
            "matchLabels": {
              "kubernetes.io/metadata.name": "kube-system"
            }
          },
          "objectSelector": {
            "matchLabels": {
              "app": "dind-service",
              "dind-service-hot-upgrade": "true"
            }
          }
        }
      ]
```

Put the Pod labels in ADS spec.template.metadata.labels so they are present
when the API server evaluates the Pod CREATE request.

### Field semantics

| Input | Behavior |
| --- | --- |
| Annotation absent or blank, or an empty array | Use selectors from the existing template |
| Webhook not listed | Preserve that webhook's template selectors |
| namespaceSelector supplied | Replace that entire selector; do not merge expressions |
| objectSelector supplied | Replace that entire selector; do not merge labels |
| One selector omitted or null | Preserve that field from the template |
| Explicit {} | Match all for that dimension, using standard Kubernetes semantics |
| Both selectors omitted or null in an entry | Reject the entry |
| Override removed | Restore the corresponding template selectors |

Each target may appear only once. A configuration can override several different
webhooks in one annotation. Namespace and object selectors are combined with AND.
Explicit {} is allowed for general selector customization; operators choosing a
Pod allowlist should supply positive matchLabels or In expressions.

### Before the extension

Relevant fields of the original webhook:

```yaml
webhooks:
  - name: mpod.kb.io
    failurePolicy: Fail
    clientConfig:
      service:
        path: /mutate-pod
    namespaceSelector:
      matchExpressions:
        - key: kubernetes.io/metadata.name
          operator: NotIn
          values: [kube-system]
    objectSelector: {}
    rules:
      - operations: [CREATE]
        apiGroups: [""]
        apiVersions: [v1]
        resources: [pods]
```

### After applying the override

There is still exactly one mpod.kb.io entry:

```yaml
webhooks:
  - name: mpod.kb.io
    failurePolicy: Fail
    clientConfig:
      service:
        path: /mutate-pod
    namespaceSelector:
      matchLabels:
        kubernetes.io/metadata.name: kube-system
    objectSelector:
      matchLabels:
        app: dind-service
        dind-service-hot-upgrade: "true"
    rules:
      - operations: [CREATE]
        apiGroups: [""]
        apiVersions: [v1]
        resources: [pods]
```

The snippets show relevant fields, not complete installable webhook configurations.

| Pod CREATE request | Default mpod.kb.io | Overridden mpod.kb.io | During an outage after override |
| --- | --- | --- | --- |
| Ordinary Pod in default namespace | Called | Skipped | Not blocked by this entry |
| Unlabeled Pod in kube-system | Skipped | Skipped | Not blocked by this entry |
| Pod in kube-system with both required labels | Skipped | Called | Rejected under Fail |
| Pod with both labels in another namespace | Called | Skipped | Not blocked by this entry |

Fail is retained for matching dind-service Pods: allowing creation without injecting
the proxy slots would leave an incomplete service. Unmatched Pods also stop receiving
other mutations implemented by /mutate-pod, so operators must account for other Kruise
features they use. For update/delete/eviction isolation, inspect and configure the
corresponding validating entries separately.

## Reconciliation and validation

The controller always starts from the template annotation. It validates overrides,
deep-copies the template entries, and replaces only the supplied fields. The template
remains unchanged. Existing certificate and endpoint reconciliation still applies.

Unknown targets, duplicate targets, malformed JSON, unknown fields and invalid label
selectors cause reconciliation to return an error before publishing either configuration.
An already published configuration is retained; there is no fallback to a broad scope
on invalid input. This does not make first-time invalid input safe: the previous
configuration may still be the default, so operators must verify effective selectors.

External-certificate installations use the same overrides and continue validating
their CA bundles. A controller-owned selector-overrides-managed annotation records
that restoration is needed after an override annotation is removed. Override processing
does not generate or replace external certificates.

To restore defaults, remove the override while the controller is running and verify
the effective selectors. Removing the annotation intentionally restores the default
scope, which may be broader. Keep the feature gate enabled while using overrides.

## Test plan

- Verify default behavior and feature-gate rejection.
- Verify replacement preserves entry count, order, identity and unrelated fields.
- Verify both namespace and Pod labels must match the dind-service allowlist.
- Verify partial overrides and explicit empty selectors for both webhook kinds.
- Reject malformed JSON, unknown fields, invalid selectors and duplicate/unknown targets.
- Verify reconciliation is idempotent and does not mutate the saved template.
- Verify invalid updates retain effective selectors and removing overrides restores defaults.
- Exercise generated and external certificate paths.
- Cluster acceptance: in an isolated test cluster, make the webhook endpoint unavailable
  after verifying effective selectors. Confirm ordinary Pod CREATE succeeds while a
  matching dind-service Pod CREATE fails. Restore the endpoint afterward. Unit selector
  matching tests alone do not demonstrate network-outage behavior.

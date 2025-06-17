# Troubleshooting Guide

This document provides guidance on troubleshooting common issues with OpenKruise. For proactive monitoring, please also refer to the [Prometheus Metrics](./prometheus-metrics.md) documentation.

## Table of Contents

- [General Troubleshooting Steps](#general-troubleshooting-steps)
- [Kruise Controller Manager Issues](#kruise-controller-manager-issues)
  - [Controller Manager Not Starting or Crashing](#controller-manager-not-starting-or-crashing)
  - [Webhook Issues](#webhook-issues)
- [CloneSet Troubleshooting](#cloneset-troubleshooting)
  - [Pods Not Scaling Up/Down or Not Updating](#pods-not-scaling-updown-or-not-updating)
  - [In-place Update Issues](#in-place-update-issues)
- [Advanced StatefulSet Troubleshooting](#advanced-statefulset-troubleshooting)
  - [In-place Update Not Working](#in-place-update-not-working)
  - [PVC Issues](#pvc-issues)
- [SidecarSet Troubleshooting](#sidecarset-troubleshooting)
  - [Sidecars Not Injected](#sidecars-not-injected)
  - [Sidecar Update Issues](#sidecar-update-issues)
- [Other Kruise Workloads](#other-kruise-workloads)
- [Common Error Messages](#common-error-messages)
- [Diagnostic Commands](#diagnostic-commands)
- [Getting Support](#getting-support)

## General Troubleshooting Steps

1.  **Check Kruise Controller Manager Logs:** This is often the first place to look. The container name is typically `manager`.
    ```bash
    kubectl logs -n kruise-system -l control-plane=controller-manager -c manager --tail=200 # Add -f to follow
    ```

2.  **Examine Kubernetes Events:** Events can provide clues about issues with resource creation, scheduling, or webhooks.
    ```bash
    # For events in the workload's namespace
    kubectl get events -n <namespace-of-your-workload> --sort-by='.lastTimestamp'
    # For events in the kruise-system namespace
    kubectl get events -n kruise-system --sort-by='.lastTimestamp'
    ```

3.  **Inspect Metrics:** Review metrics exposed by OpenKruise. High error rates, reconciliation latencies, or work queue depth can indicate problems. See [Prometheus Metrics](./prometheus-metrics.md).

4.  **Verify CRD Installation:** Ensure all OpenKruise CRDs are correctly installed and up-to-date.
    ```bash
    kubectl get crd | grep 'kruise.io'
    ```

5.  **Describe the Problematic Resource:** This shows the resource's spec, status, and recent events.
    ```bash
    kubectl describe <kruise-workload-kind> <resource-name> -n <namespace>
    # e.g., kubectl describe cloneset my-cloneset -n default
    ```

## Kruise Controller Manager Issues

### Controller Manager Not Starting or Crashing

**Symptoms:**
- `kruise-controller-manager` pod(s) in `kruise-system` namespace are in `CrashLoopBackOff`, `Error`, or `Pending` state.
- Controllers are not reconciling workloads (e.g., no new pods created for a CloneSet).

**Troubleshooting Steps:**

1.  Check pod status and logs:
    ```bash
    kubectl get pods -n kruise-system -l control-plane=controller-manager
    # Describe the failing pod
    kubectl describe pod -n kruise-system <manager-pod-name>
    # Get logs from the manager container within the pod
    kubectl logs -n kruise-system <manager-pod-name> -c manager
    # If there's an init container or sidecar, check its logs too
    # kubectl logs -n kruise-system <manager-pod-name> -c <other-container-name>
    ```
    Look for OOMKilled (Out Of Memory), permission issues, or connection problems to the Kubernetes API server.

2.  Check resource requests/limits defined for the manager deployment and ensure nodes have capacity.

3.  Verify RBAC permissions: The controller manager needs appropriate ClusterRoles and RoleBindings. Issues here usually manifest as "forbidden" errors in logs.

### Webhook Issues

OpenKruise relies on Mutating and Validating Admission Webhooks for many of its features (e.g., sidecar injection, in-place updates, resource validation).

**Symptoms:**
- Failure to create, update, or delete OpenKruise CRDs or standard resources (Pods, Deployments) that OpenKruise might interact with.
- Error messages like:
  - `"Failed calling webhook ... connection refused"`
  - `"Internal error occurred: failed calling webhook ...: Post ... x509: certificate signed by unknown authority"`
  - `"admission webhook ... denied the request"`
  - Timeout errors related to webhooks.

**Troubleshooting Steps:**

1.  **Check Webhook Configurations:**
    ```bash
    kubectl get mutatingwebhookconfiguration | grep kruise
    kubectl get validatingwebhookconfiguration | grep kruise
    ```
    Inspect their YAML (`-o yaml`) paying attention to:
    - `clientConfig.service`: Must correctly point to the webhook service (e.g., `kruise-webhook-service` or `kruise-manager`) in `kruise-system` on the correct port (e.g., 443, 9443).
    - `clientConfig.caBundle`: Must contain the CA that signed the webhook server's certificate.
    - `failurePolicy`: `Fail` means issues block requests; `Ignore` allows requests but features might not work as expected.
    - `rules.operations` and `rules.resources`: Ensure they cover the intended operations and resources.
    - `timeoutSeconds`: Default is 10s. If webhooks are slow, requests might time out.

2.  **Check Webhook Service and Endpoints:**
    ```bash
    # Identify the webhook service name from the WebhookConfiguration
    kubectl get svc -n kruise-system | grep kruise # Look for services related to webhook or manager
    kubectl get endpoints -n kruise-system <webhook-service-name>
    ```
    Ensure the service exists, has a `ClusterIP`, and the Endpoints object has at least one ready IP address (matching a running `kruise-controller-manager` pod).

3.  **Check Webhook Pod Logs:** Webhook handling logic runs within the `kruise-controller-manager` pod.
    ```bash
    kubectl logs -n kruise-system -l control-plane=controller-manager -c manager | grep -iE "webhook|admission"
    ```

4.  **Certificate Issues (`x509` errors):**
    - OpenKruise typically uses a self-signed certificate managed by its controller or a tool like `cert-manager`.
    - Check the secret storing the webhook certificate (often named `kruise-webhook-certs`, `kruise-manager-webhook-cert` or similar in `kruise-system`).
      ```bash
      kubectl get secret -n kruise-system | grep webhook
      kubectl get secret -n kruise-system <webhook-cert-secret-name> -o yaml
      ```
    - The `caBundle` in webhook configurations must match the CA from this secret.
    - If certificates have expired or are mismatched, restarting the `kruise-controller-manager` deployment often triggers regeneration (if certificates are self-managed by Kruise):
      ```bash
      kubectl rollout restart deployment -n kruise-system kruise-controller-manager
      ```
    - Ensure the Kubernetes API server can reach the webhook pod on its webhook port (check NetworkPolicies or firewalls).

5.  **Check Metrics:** Review webhook metrics like `kruise_webhook_admission_requests_total` and `kruise_webhook_admission_duration_seconds` (see [Prometheus Metrics](./prometheus-metrics.md)). High latencies or error rates are indicators.

## CloneSet Troubleshooting

### Pods Not Scaling Up/Down or Not Updating

**Symptoms:**
- CloneSet `spec.replicas` count doesn't match `status.replicas` or `status.readyReplicas`.
- Pods are not created/deleted/updated as expected when the CloneSet spec changes.

**Troubleshooting Steps:**

1.  Describe the CloneSet and check its status and events:
    ```bash
    kubectl describe cloneset <cloneset-name> -n <namespace>
    kubectl get events --field-selector involvedObject.kind=CloneSet,involvedObject.name=<cloneset-name>,involvedObject.namespace=<namespace> --sort-by='.lastTimestamp'
    ```
    Look for conditions like `Progressing=False` or error messages in events.

2.  Check controller logs, filtering for the CloneSet name or UID:
    ```bash
    # Get UID of the CloneSet
    # kubectl get cloneset <cloneset-name> -n <namespace> -o jsonpath='{.metadata.uid}'
    kubectl logs -n kruise-system -l control-plane=controller-manager -c manager | grep "<cloneset-name-or-uid>"
    ```

3.  Verify Pod template validity: If the pod template is invalid (e.g., image not found, bad configuration, failing readiness/liveness probes), new pods might fail to start or become ready. Check `kubectl describe pod <new-pod-name>` for newly created pods.

4.  Check `spec.updateStrategy.partition` and `spec.updateStrategy.maxUnavailable`/`maxSurge`. A high partition number can prevent updates to pods with lower ordinal numbers.

5.  Review metrics for the `cloneset-controller` (see [Prometheus Metrics](./prometheus-metrics.md)).

### In-place Update Issues

**Symptoms:**
- Pods are recreated instead of updated in-place when only container images or specific annotations/labels change.
- In-place update seems stuck.

**Troubleshooting Steps:**

1.  Ensure `spec.updateStrategy.type` is `InPlaceIfPossible` or `InPlaceOnly`.
    ```bash
    kubectl get cloneset <cloneset-name> -n <namespace> -o jsonpath="{.spec.updateStrategy.type}"
    ```

2.  Verify that the change made qualifies for in-place update (typically only `spec.template.spec.containers[*].image` and certain metadata). Other changes will force pod recreation. Consult OpenKruise documentation for exact fields supporting in-place update.

3.  Examine pod annotations related to in-place update:
    ```bash
    kubectl get pod <pod-name> -n <namespace> -o jsonpath="{.metadata.annotations.apps\.kruise\.io/in-place-update-state}"
    kubectl get pod <pod-name> -n <namespace> -o yaml # Look for other kruise annotations like 'apps.kruise.io/inplace-update-grace-period'
    ```

4.  Check pod events and container statuses. In-place update might be blocked by readiness/liveness probes failing post-update or by preStop/postStart hooks.

## Advanced StatefulSet Troubleshooting

(Troubleshooting steps are often similar to CloneSet, but with StatefulSet-specific considerations like persistent identity and storage.)

### In-place Update Not Working

**Symptoms & Steps:** Similar to CloneSet in-place update issues. Verify `spec.updateStrategy.type`. Check the `podUpdatePolicy` field in `rollingUpdate` strategy.

### PVC Issues

**Symptoms:**
- Pods are stuck in `Pending` due to PVC binding issues.
- Events show errors like "Failed to provision volume" or "PVC not found".

**Troubleshooting Steps:**

1.  Check PVC status:
    ```bash
    kubectl get pvc -n <namespace> | grep <statefulset-name>
    kubectl describe pvc <pvc-name> -n <namespace>
    ```

2.  Verify `volumeClaimTemplates` in the AdvancedStatefulSet spec.
3.  Check `StorageClass` configuration and persistent volume provisioner logs.
4.  Ensure that if `spec.reserveOrdinals` is used, it's not conflicting with PVC creation for active ordinals.

## SidecarSet Troubleshooting

### Sidecars Not Injected

**Symptoms:**
- Expected sidecar containers are missing from target pods after they are created or restarted.

**Troubleshooting Steps:**

1.  Verify SidecarSet `spec.selector` correctly matches labels of target pods.
    ```bash
    kubectl get sidecarset <sidecarset-name> -o jsonpath="{.spec.selector}"
    kubectl get pods -n <target-namespace> --show-labels # Check if pods have matching labels
    ```

2.  Ensure the SidecarSet is in the correct scope. Namespace-scoped SidecarSets only affect pods in their own namespace unless `spec.namespaceSelector` is used to target other namespaces.
    ```bash
    kubectl get sidecarset <sidecarset-name> -o jsonpath="{.spec.namespace}" # if defined
    kubectl get sidecarset <sidecarset-name> -o jsonpath="{.spec.namespaceSelector}" # if defined
    ```

3.  Check webhook functionality (see [Webhook Issues](#webhook-issues)). Sidecar injection relies on a mutating admission webhook.

4.  Inspect SidecarSet status and events:
    ```bash
    kubectl describe sidecarset <sidecarset-name>
    kubectl get sidecarset <sidecarset-name> -o jsonpath='{.status}'
    ```
    Look at `status.matchedPods`, `status.updatedPods`.

### Sidecar Update Issues

**Symptoms:**
- Sidecar containers in existing pods are not updated when the SidecarSet spec (e.g., sidecar image) changes.

**Troubleshooting Steps:**

1.  Verify `spec.updateStrategy.type` (e.g., `RollingUpdate`).
2.  Check if pods have the necessary annotations indicating SidecarSet control and hash, e.g., `apps.kruise.io/sidecarset-hash`.
3.  The update process might be gradual, depending on the `rollingUpdate` strategy (e.g., `maxUnavailable`). Monitor `status.updatedPods` and `status.updatedReadyPods`.
4.  For changes to take effect on existing pods, they typically need to be restarted or recreated if the update strategy doesn't automatically trigger it (e.g., for non-image changes, or if `injectionStrategy.policy` is not `Always`).

## Other Kruise Workloads

(UnitedDeployment, BroadcastJob, AdvancedDaemonSet, etc.)
Follow similar patterns:
1.  `kubectl describe <kind> <name> -n <namespace>`
2.  Check controller logs, filtering for the resource name or UID.
3.  Check events.
4.  Verify webhook functionality if applicable.
5.  Consult specific documentation for that workload type.

## Common Error Messages

### "admission webhook ... denied the request because ..."

**Possible Causes:**
- Invalid resource spec according to OpenKruise validation rules.
- An external policy controller (like OPA Gatekeeper, Kyverno) is denying the request due to a policy violation.

**Resolution:**
1.  Carefully read the denial message; it usually explains the reason.
2.  Correct the resource YAML definition.
3.  If it's an external policy controller, check its logs and policies.

### "controller-runtime manager error" / "leader election lost" / "context deadline exceeded"

**Possible Causes:**
- Controller manager pod crashed or was restarted.
- Network issues between manager pods or with the Kubernetes API server.
- Resource contention (CPU/memory) on the manager pod or its node.
- API server is overloaded or unresponsive.

**Resolution:**
1.  Check controller manager logs (see [General Troubleshooting Steps](#general-troubleshooting-steps)).
2.  Ensure multiple replicas of `kruise-controller-manager` (if configured) can communicate for leader election (check Kubernetes events for leader election messages).
3.  Check for resource exhaustion on nodes running the controller manager or on the API server itself.
4.  Monitor API server latency and error rates.

## Diagnostic Commands

### Collecting Logs and Information for a Bug Report

```bash
# Controller manager deployment and pod details
kubectl get deployment -n kruise-system kruise-controller-manager -o yaml > kruise-manager-deployment.yaml
kubectl get pods -n kruise-system -l control-plane=controller-manager -o wide > kruise-manager-pods.txt
kubectl describe pods -n kruise-system -l control-plane=controller-manager > kruise-manager-pods-describe.txt

# Controller manager logs (collect a significant amount, e.g., last hour or since issue started if possible)
kubectl logs -n kruise-system deploy/kruise-controller-manager -c manager --since=1h > kruise-manager.log

# Describe problematic Kruise resource
kubectl get <kind> <name> -n <namespace> -o yaml > my-resource.yaml
kubectl describe <kind> <name> -n <namespace> > my-resource-describe.txt

# Relevant events (from workload namespace and kruise-system)
kubectl get events -n <namespace> --sort-by='.lastTimestamp' > workload-ns-events.txt
kubectl get events -n kruise-system --sort-by='.lastTimestamp' > kruise-system-events.txt

# Webhook configurations (adjust labels if your Kruise installation uses different ones)
kubectl get mutatingwebhookconfiguration -l app.kubernetes.io/part-of=openkruise -o yaml > mutatingwebhooks.yaml
kubectl get validatingwebhookconfiguration -l app.kubernetes.io/part-of=openkruise -o yaml > validatingwebhooks.yaml

# CRD definitions for relevant Kruise kinds
# kubectl get crd <crd-name.kruise.io> -o yaml > crd-<kind>.yaml
# Manage Pod labels dynamically according to different rules

## Motivation

Many advanced K8s workload management features require the use of labels. In some cases, labels are needed after Pods are created so that adding them to Pod template does not help. For example,

- A canary release plan may want to upgrade X number of Pods out of Y number of total replica. We can add a label to the chosen Pod so that the rollout controller can upgrade these Pods first.

- Pod container may provides a query service. We can add label based on query result so that we can rollout Pods based on container running status.

## Goals

- Manage(Add/Update/Delete) pod's labels according to various rules automatically

## Proposal

### Define a PodMarker CRD

```yaml
# podMarker.yaml
apiVersion: apps.kruise.io/v1alpha1
kind: PodMarker
metadata:
  name: test-podmarker
  namespace: default
spec:
  markLabels: # labels you want to patch to matched pods
    upgdateStrategy: rollingUpdate
    status: running
  replicas: 2 # the number of pod you want tp patch labels to
  selector: # For label selector, you can follow the official doc to set: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/
    matchLabels:
      app: nginx
      role: loadbalancer
    # matchExpressions:
    #  - {key: tier, operator: In, values: [cache]}
status:
    matchedPods: 2
```

### DataStructure

```go
type PodMarker struct {
    metav1.TypeMeta       `json:",inline"`
    metav1.ObjectMeta     `json:"metadata,omitempty"`
    Spec   PodMarkerSpec   `json:"spec,omitempty"`
    Status PodMarkerStatus `json:"status,omitempty"`
}

type PodMarkerSpec struct {
    MarkLabels MarkLabels           `json:"markLabels, omitempty"`
    Replicas int                    `json:"replicas, omitempty"`
    Selector *metav1.LabelSelector  `json:"selector,omitempty"`
}

type MarkLabels map[string]string

type PodMarkerStatus struct {
    MatchedPods int      `json:"matchedPods, omitempty"`
}

```

### How to implement

#### Patch labels to the Pod

- Define a CRD instance called PodMarker. Users can describe the desired status of labels.
- Implement a controller called PodMarkerController to manage the PodMarker Instance.
  - Watch changes of PodMarker instances
  - Collect labels users want to add/update/remove
  - Choose candidate pods according to the labelSelector
  - Call the Kubernetes API to modify labels of choosing pods
  - Update the podMarker.Status

#### Save history of markLabels && Rollback markLabels for the Pod

Actions related to labels usually include four parts:

- Add: Create the PodMarker resource object to patch labels to specific Pods.
- Update: Update `markLabels` field of existed PodMarker object and patch the newest labels to specific Pods.
- Delete: Delete patched labels of Pod through deleting the PodMarker object.
- RollBack: Revert labels of Pod to the last or next version.

For saving the history of markLabels or rolling back to old version, I reference the `revision history` mechanism of Deployment. When I used the Deployment workload to orchestrate my
applications, I can perform command `kubectl rollout history <Workload Type> <Workload Object Name>` to detect historial versions if I modified that Deployment workload like below.

```bash
REVISION    CHANGE-CAUSE
1           kubectl apply --filename=https://k8s.io/examples/controllers/nginx-deployment.yaml --record=true
2           kubectl set image deployment.v1.apps/nginx-deployment nginx=nginx:1.9.1 --record=true
3           kubectl set image deployment.v1.apps/nginx-deployment nginx=nginx:1.91 --record=true
```

I can choose one of old versions to rollback my Deployment workload like below

```bash
kubectl rollout history <Workload Type> <Workload Object Name> --revision=<historial revision>
```

Not only that, we can also adjust the number of historical revisions we saved.

Related documents:

- <https://kubernetes.io/docs/concepts/workloads/controllers/deployment/#rolling-back-a-deployment>

## RoadMap

- [ ] [Phase1] For a specific workload, mark x number of replicas with specified label key:val

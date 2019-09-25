# SidecarSet

This controller leverages the mutating webhook admission controllers to automatically
inject a sidecar container for every selected Pod when the Pod is created. The Sidecar
injection process is similar to the automatic sidecar injection mechanism used in
[istio](https://istio.io/docs/setup/kubernetes/additional-setup/sidecar-injection/).

Besides deployment, SidecarSet controller also provides
additional capabilities such as in-place Sidecar container image upgrade, mounting Sidecar volumes, etc.
Basically, SidecarSet decouples the Sidecar container lifecycle
management from the main container lifecycle management.

The SidecarSet is preferable for managing stateless sidecar containers such as
monitoring tools or operation agents. Its API spec is listed below:

```go
type SidecarSetSpec struct {
    // selector is a label query over pods that should be injected
    Selector *metav1.LabelSelector `json:"selector,omitempty"`

    // containers specifies the list of containers to be injected into the pod
    Containers []SidecarContainer `json:"containers,omitempty"`

    // List of volumes that can be mounted by sidecar containers
    Volumes []corev1.Volume `json:"volumes,omitempty"`

    // Paused indicates that the sidecarset is paused and will not be processed by the sidecarset controller.
    Paused bool `json:"paused,omitempty"`

    // The sidecarset strategy to use to replace existing pods with new ones.
    Strategy SidecarSetUpdateStrategy `json:"strategy,omitempty"`
}

type SidecarContainer struct {
    corev1.Container
}
```

Note that the injection happens at Pod creation time and only Pod spec is updated.
The workload template spec will not be updated.

## Example

### Create a SidecarSet

The `sidecarset.yaml` file below describes a SidecarSet that contains a sidecar container named `sidecar1`

```
# sidecarset.yaml
apiVersion: apps.kruise.io/v1alpha1
kind: SidecarSet
metadata:
  name: test-sidecarset
spec:
  selector:
    matchLabels:
      app: nginx
  strategy:
    rollingUpdate:
      maxUnavailable: 2
  containers:
  - name: sidecar1
    image: centos:6.7
    command: ["sleep", "999d"] # do nothing at all
    volumeMounts:
    - name: log-volume
      mountPath: /var/log
  volumes: # this field will be merged into pod.spec.volumes
  - name: log-volume
    emptyDir: {}
```

Create a SidecarSet based on the YAML file:

```
kubectl apply -f sidecarset.yaml
```

### Create a Pod

Create a pod that matches the sidecarset's selector:

```
apiVersion: v1
kind: Pod
metadata:
  labels:
    app: nginx # matches the SidecarSet's selector
  name: test-pod
spec:
  containers:
  - name: app
    image: nginx:1.15.1
```

Create this pod and now you will find it's injected with `sidecar1`:

```
# kubectl get pod
NAME       READY   STATUS    RESTARTS   AGE
test-pod   2/2     Running   0          118s
```

In the meantime, the SidecarSet status updated:

```

# kubectl get sidecarset test-sidecarset -o yaml | grep -A4 status

status:
  matchedPods: 1
  observedGeneration: 1
  readyPods: 1
  updatedPods: 1
```

### Update a SidecarSet

Using ```kubectl edit sidecarset test-sidecarset``` to modify SidecarSet image from `centos:6.7` to `centos:6.8`, You should find that the matched pods will be updated in-place sequentially.
`.spec.strategy.rollingUpdate.maxUnavailable` is an optional field that specifies the maximum number of Pods that can be unavailable during the update process. The default value is 1. The value can be an absolute number or a percentage of desired pods. For example, 10% means 10% * `matched pods` number of pods can be upgraded simultaneously. The calculated value is rounded down to the nearest integer.

You could use ```kubectl patch sidecarset test-sidecarset --type merge -p '{"spec":{"paused":true}}'``` to pause the update procedure.

If user modifies fields other than image in SidecarSet Spec, the sidecar container in the pod won't get updated until the pod is recreated by workload (e.g., Deployment).
This behavior is also referred to as **lazy update** mode.

## Tutorial

A detailed tutorial is provided:

- [Use SidecarSet to inject a sidecar container into the Guestbook application](../../tutorial/sidecarset.md)

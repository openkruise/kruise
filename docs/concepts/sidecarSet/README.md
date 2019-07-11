# SidecarSet

This controller leverages the mutating webhook admission controllers to automatically
inject a sidecar container for every selected Pod when the Pod is created. The Sidecar
injection process is similar to the automatic sidecar injection mechanism used in
[istio](https://istio.io/docs/setup/kubernetes/additional-setup/sidecar-injection/).

Besides deployment, in the future release, SidecarSet controller will provide
additional capabilities such as in-place Sidecar container image upgrade
and Sidecar container clean up if it is no longer needed.
Basically, SidecarSet decouples the Sidecar container lifecycle
management from the main container lifecycle management.
The upgrade capability is under development.

The SidecarSet is preferable for managing stateless sidecar containers such as
monitoring tools or operation agents. Its API spec is listed below:

```go
type SidecarSetSpec struct {
    // selector is a label query over pods that should be injected
    Selector *metav1.LabelSelector `json:"selector,omitempty"`

    // containers specifies the list of containers to be injected into the pod
    Containers []SidecarContainer `json:"containers,omitempty"`
}

type SidecarContainer struct {
    corev1.Container
}
```

The `Selector` selects which pods to inject the sidecar container using the label
selector and the `Containers` specifies the list of containers to be injected.

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
  containers:
  - name: sidecar1
    image: centos:7
    command: ["sleep", "999d"] # do nothing at all
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

## Tutorial

A more sophisticated tutorial is provided:

- [Use SidecarSet to inject a sidecar container into the Guestbook application](../../tutorial/sidecarset.md)
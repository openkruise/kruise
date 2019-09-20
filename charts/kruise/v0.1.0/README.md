# Kruise

## Install

```bash
# git clone https://github.com/openkruise/kruise
# cd charts/kruise
# helm install kruise ./
```

you will see follow:

```
[root@k8s-master submitted]# helm install kruise ./
NAME:   kruise
LAST DEPLOYED: Fri Aug 23 10:37:52 2019
NAMESPACE: default
STATUS: DEPLOYED

RESOURCES:
==> v1/ClusterRole
NAME                 AGE
kruise-manager-role  0s

==> v1/ClusterRoleBinding
NAME                        AGE
kruise-manager-rolebinding  0s

==> v1/Namespace
NAME           STATUS  AGE
kruise-system  Active  0s

==> v1/Pod(related)
NAME                         READY  STATUS             RESTARTS  AGE
kruise-controller-manager-0  0/1    ContainerCreating  0         0s

==> v1/Secret
NAME                          TYPE    DATA  AGE
kruise-webhook-server-secret  Opaque  0     0s

==> v1/Service
NAME                               TYPE       CLUSTER-IP    EXTERNAL-IP  PORT(S)  AGE
kruise-controller-manager-service  ClusterIP  10.98.202.12  <none>       443/TCP  0s

==> v1/StatefulSet
NAME                       READY  AGE
kruise-controller-manager  0/1    0s

==> v1beta1/CustomResourceDefinition
NAME                          AGE
broadcastjobs.apps.kruise.io  0s
sidecarsets.apps.kruise.io    0s
statefulsets.apps.kruise.io   0s

==> v1beta1/MutatingWebhookConfiguration
NAME                                   AGE
kruise-mutating-webhook-configuration  0s

==> v1beta1/ValidatingWebhookConfiguration
NAME                                     AGE
kruise-validating-webhook-configuration  0s

```

## Uninstall

```bash
# helm delete kruise
```

## Configuration

The following table lists the configurable parameters of the kruise chart and their default values.

| Parameter                                 | Description                                                        | Default                             |
|-------------------------------------------|--------------------------------------------------------------------|-------------------------------------|
| `log.level`                               | Log level that kruise-manager printed                              | `4`                                 |
| `revisionHistoryLimit`                    | Limit of revision history                                          | `3`                                 |
| `manager.resources.limits.cpu`            | CPU resource limit of kruise-manager container                     |                                     |
| `manager.resources.limits.memory`         | Memory resource limit of kruise-manager container                  |                                     |
| `manager.resources.requests.cpu`          | CPU resource request of kruise-manager container                   |                                     |
| `manager.resources.requests.memory`       | Memory resource request of kruise-manager container                |                                     |
| `manager.metrics.addr`                    | Addr of metrics served                                             | `localhost`                         |
| `manager.metrics.port`                    | Port of metrics served                                             | `8080`                              |
| `spec.nodeAffinity`                       | Node affinity policy for kruise-manager pod                        | `{}`                                |
| `spec.nodeSelector`                       | Node labels for kruise-manager pod                                 | `{}`                                |
| `spec.tolerations`                        | Tolerations for kruise-manager pod                                 | `[]`

Specify each parameter using the `--set key=value[,key=value]` argument to `helm install`. For example,

```bash
# helm install kruise ./  --set manager.log.level=5
```
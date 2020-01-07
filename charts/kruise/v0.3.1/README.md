# Kruise

## Install

```bash
helm install kruise https://github.com/openkruise/kruise/releases/download/v0.3.1/kruise-chart.tgz
```

you will see follow:

```
NAME: kruise
LAST DEPLOYED: Mon Jan  6 14:47:48 2020
NAMESPACE: default
STATUS: deployed
REVISION: 1
TEST SUITE: None
```

## Uninstall

```bash
$ helm delete kruise
release "kruise" uninstalled
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
# helm install kruise https://github.com/openkruise/kruise/releases/download/v0.3.1/kruise-chart.tgz --set manager.log.level=5
```
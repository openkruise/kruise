# Kruise

## Install

Install with Helm 3:

OpenKruise only supports Kubernetes version >= `1.13+` because of CRD conversion.
Note that for Kubernetes 1.13 and 1.14, users must enable `CustomResourceWebhookConversion` feature-gate in kube-apiserver before install or upgrade Kruise.

```bash
# Kubernetes 1.13 and 1.14
helm install kruise https://github.com/openkruise/kruise/releases/download/v0.8.1/kruise-chart.tgz --disable-openapi-validation
# Kubernetes 1.15 and newer versions
helm install kruise https://github.com/openkruise/kruise/releases/download/v0.8.1/kruise-chart.tgz
```

you will see follow:

```
NAME: kruise
LAST DEPLOYED: Tue Mar  2 12:03:51 2021
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

| Parameter                                 | Description                                                  | Default                       |
| ----------------------------------------- | ------------------------------------------------------------ | ----------------------------- |
| `featureGates`                            | Feature gates for Kruise, empty string means all enabled     | ``                            |
| `installation.namespace`                  | namespace for kruise installation                            | `kruise-system`               |
| `manager.log.level`                       | Log level that kruise-manager printed                        | `4`                           |
| `manager.replicas`                        | Replicas of kruise-controller-manager deployment             | `2`                           |
| `manager.image.repository`                | Repository for kruise-manager image                          | `openkruise/kruise-manager`   |
| `manager.image.tag`                       | Tag for kruise-manager image                                 | `v0.8.1`                      |
| `manager.resources.limits.cpu`            | CPU resource limit of kruise-manager container               | `100m`                        |
| `manager.resources.limits.memory`         | Memory resource limit of kruise-manager container            | `256Mi`                       |
| `manager.resources.requests.cpu`          | CPU resource request of kruise-manager container             | `100m`                        |
| `manager.resources.requests.memory`       | Memory resource request of kruise-manager container          | `256Mi`                       |
| `manager.metrics.port`                    | Port of metrics served                                       | `8080`                        |
| `manager.webhook.port`                    | Port of webhook served                                       | `9443`                        |
| `manager.nodeAffinity`                    | Node affinity policy for kruise-manager pod                  | `{}`                          |
| `manager.nodeSelector`                    | Node labels for kruise-manager pod                           | `{}`                          |
| `manager.tolerations`                     | Tolerations for kruise-manager pod                           | `[]`                          |
| `daemon.log.level`                        | Log level that kruise-daemon printed                         | `4`                           |
| `daemon.port`                             | Port of metrics and healthz that kruise-daemon served        | `10221`                       |
| `daemon.resources.limits.cpu`             | CPU resource limit of kruise-daemon container                | `50m`                         |
| `daemon.resources.limits.memory`          | Memory resource limit of kruise-daemon container             | `64Mi`                        |
| `daemon.resources.requests.cpu`           | CPU resource request of kruise-daemon container              | `0`                           |
| `daemon.resources.requests.memory`        | Memory resource request of kruise-daemon container           | `0`                           |
| `daemon.affinity`                         | Affinity policy for kruise-daemon pod                        | `{}`                          |
| `daemon.socketLocation`                   | Location of the container manager control socket             | `/var/run`                    |
| `webhookConfiguration.failurePolicy.pods` | The failurePolicy for pods in mutating webhook configuration | `Ignore`                      |
| `webhookConfiguration.timeoutSeconds`     | The timeoutSeconds for all webhook configuration             | `30`                          |
| `crds.managed`                            | Kruise will not install CRDs with chart if this is false     | `true`                        |

Specify each parameter using the `--set key=value[,key=value]` argument to `helm install`. For example,

```bash
$ helm install kruise https://... --set manager.log.level=5
...
```

### Optional: feature-gate

Feature-gate controls some influential features in Kruise:

| Name                   | Description                                                  | Default | Side effect (if closed)              |
| ---------------------- | ------------------------------------------------------------ | ------- | --------------------------------------
| `PodWebhook`           | Whether to open a webhook for Pod **create**                 | `true`  | SidecarSet disabled                  |
| `KruiseDaemon`         | Whether to deploy `kruise-daemon` DaemonSet                  | `true`  | Image pulling disabled               |

If you want to configure the feature-gate, just set the parameter when install or upgrade:

```bash
# one
$ helm install kruise https://... --set featureGates="PodWebhook=false"
...

# or more
$ helm install kruise https://... --set featureGates="PodWebhook=false\,KruiseDaemon=false"
...
```

### Optional: the local image for China

If you are in China and have problem to pull image from official DockerHub, you can use the registry hosted on Alibaba Cloud:

```bash
$ helm install kruise https://... --set  manager.image.repository=openkruise-registry.cn-hangzhou.cr.aliyuncs.com/openkruise/kruise-manager
...
```

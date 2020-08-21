# Use Advanced DaemonSet to deploy daemons

This tutorial goes through an example of deploying and updating daemons using Advanced DaemonSet.

## Prepare nodes

In this example, the testing cluster has five available nodes.
In order to verify node selector update, we add a `node-type=canary` label to one of them.

```bash
$ kubectl label node cn-hangzhou.192.168.0.10 node-type=canary --overwrite
node/cn-hangzhou.192.168.0.10 labeled

$ kubectl get nodes -L node-type
NAME                       STATUS   ROLES    AGE   VERSION            NODE-TYPE
cn-hangzhou.192.168.0.10   Ready    <none>   89d   v1.16.9-aliyun.1   canary
cn-hangzhou.192.168.0.11   Ready    <none>   89d   v1.16.9-aliyun.1
cn-hangzhou.192.168.0.12   Ready    <none>   89d   v1.16.9-aliyun.1
cn-hangzhou.192.168.0.23   Ready    <none>   58d   v1.16.9-aliyun.1
cn-hangzhou.192.168.0.24   Ready    <none>   58d   v1.16.9-aliyun.1
```

## Install guestbook DaemonSet

```bash
$ kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/docs/tutorial/v1/daemonset-guestbook.yaml
daemonset.apps.kruise.io/guestbook-ds created
```

Check to make sure all Pods are running and ready.

```bash
$ kubectl get pod -L controller-revision-hash -l app=guestbook-daemon -o wide
NAME                 READY   STATUS    RESTARTS   AGE   IP             NODE                       CONTROLLER-REVISION-HASH
guestbook-ds-bwtl4   1/1     Running   0          8s    172.27.0.186   cn-hangzhou.192.168.0.11   864fbf5949
guestbook-ds-fbhnn   1/1     Running   0          8s    172.27.1.31    cn-hangzhou.192.168.0.24   864fbf5949
guestbook-ds-fvq49   1/1     Running   0          8s    172.27.0.62    cn-hangzhou.192.168.0.12   864fbf5949
guestbook-ds-mbd56   1/1     Running   0          8s    172.27.0.110   cn-hangzhou.192.168.0.10   864fbf5949
guestbook-ds-t8dd8   1/1     Running   0          8s    172.27.0.217   cn-hangzhou.192.168.0.23   864fbf5949
```

## Update guestbook with Standard methodology

### Using selector

Edit the DaemonSet, update image to `openkruise/guestbook:v2` and set node selector `node-type: canary` in the `rollingUpdate` spec:

```yaml
$ kubectl edit daemonset.apps.kruise.io guestbook-ds
...

  updateStrategy:
    rollingUpdate:
      selector:
        matchLabels:
          node-type: canary
      maxUnavailable: 1
      partition: 0
      rollingUpdateType: Standard
    type: RollingUpdate
```

Then we will find the Pod on `cn-hangzhou.192.168.0.10`, which has been added the `canary` label, has been recreated using the new version.

```bash
$ kubectl get pod -L controller-revision-hash -l app=guestbook-daemon -o wide
NAME                 READY   STATUS    RESTARTS   AGE   IP             NODE                       CONTROLLER-REVISION-HASH
guestbook-ds-bwtl4   1/1     Running   0          73s   172.27.0.186   cn-hangzhou.192.168.0.11   864fbf5949
guestbook-ds-fbhnn   1/1     Running   0          73s   172.27.1.31    cn-hangzhou.192.168.0.24   864fbf5949
guestbook-ds-fvq49   1/1     Running   0          73s   172.27.0.62    cn-hangzhou.192.168.0.12   864fbf5949
guestbook-ds-t8dd8   1/1     Running   0          73s   172.27.0.217   cn-hangzhou.192.168.0.23   864fbf5949
guestbook-ds-xnld8   1/1     Running   0          3s    172.27.0.111   cn-hangzhou.192.168.0.10   cdf6d4478
```

### Using partition

Edit the DaemonSet, remove the node selector and set partition to 2.

```yaml
$ kubectl edit daemonset.apps.kruise.io guestbook-ds
...

  updateStrategy:
    rollingUpdate:
      maxUnavailable: 1
      partition: 2
      rollingUpdateType: Standard
    type: RollingUpdate
```

Then the controller will update two more Pods one by one,
to reach the expected `5 - 2 = 3` Pods to use the new version in addition to the one that has been updated in previous step.

```bash
$ kubectl get pod -L controller-revision-hash -l app=guestbook-daemon -o wide
NAME                 READY   STATUS    RESTARTS   AGE    IP             NODE                       CONTROLLER-REVISION-HASH
guestbook-ds-bwtl4   1/1     Running   0          2m5s   172.27.0.186   cn-hangzhou.192.168.0.11   864fbf5949
guestbook-ds-fvq49   1/1     Running   0          2m5s   172.27.0.62    cn-hangzhou.192.168.0.12   864fbf5949
guestbook-ds-pxhn4   1/1     Running   0          11s    172.27.1.32    cn-hangzhou.192.168.0.24   cdf6d4478
guestbook-ds-txqjl   1/1     Running   0          2s     172.27.0.218   cn-hangzhou.192.168.0.23   cdf6d4478
guestbook-ds-xnld8   1/1     Running   0          55s    172.27.0.111   cn-hangzhou.192.168.0.10   cdf6d4478
```

### Updating the rest of Pods

Edit the DaemonSet and set partition to 0.

```yaml
$ kubectl edit daemonset.apps.kruise.io guestbook-ds
...

  updateStrategy:
    rollingUpdate:
      maxUnavailable: 1
      partition: 0
      rollingUpdateType: Standard
    type: RollingUpdate
```

All Pods will be updated.

```bash
$ kubectl get pod -L controller-revision-hash -l app=guestbook-daemon -o wide
NAME                 READY   STATUS    RESTARTS   AGE     IP             NODE                       CONTROLLER-REVISION-HASH
guestbook-ds-j76db   1/1     Running   0          3s      172.27.0.7     cn-hangzhou.192.168.0.12   cdf6d4478
guestbook-ds-lcbnj   1/1     Running   0          14s     172.27.0.187   cn-hangzhou.192.168.0.11   cdf6d4478
guestbook-ds-pxhn4   1/1     Running   0          7m34s   172.27.1.32    cn-hangzhou.192.168.0.24   cdf6d4478
guestbook-ds-txqjl   1/1     Running   0          7m25s   172.27.0.218   cn-hangzhou.192.168.0.23   cdf6d4478
guestbook-ds-xnld8   1/1     Running   0          8m18s   172.27.0.111   cn-hangzhou.192.168.0.10   cdf6d4478
```

## Update guestbook with Surging methodology

The rollingUpdateType could be **Standard** or **Surging**.

`maxUnavailable` is valid for **Standard** type and `maxSurge` is valid for **Surging** type.

```yaml
  updateStrategy:
    rollingUpdate:
      maxSurge: 1
      partition: 0
      rollingUpdateType: Surging
    type: RollingUpdate
```

When rollingUpdateType is **Surging**:

1. For each node, the controller will first create a new Pod, and then delete the old one when the new one has been ready.
2. `maxSurge` controls how many nodes could be updated at one time.

Go and try **Surging** by yourself to understand how it works.

## Uninstall guestbook DaemonSet

```bash
$ kubectl delete -f https://raw.githubusercontent.com/kruiseio/kruise/master/docs/tutorial/v1/daemonset-guestbook.yaml
daemonset.apps.kruise.io "guestbook-ds" deleted
```

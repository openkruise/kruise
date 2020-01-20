# Tutorial

These tutorials walk through several examples to demonstrate how to use the Kruise Workloads to deploy and manage applications

## Install Kruise

Install Kruise with helm v3, which is a simple command-line tool and you can get it from [here](https://github.com/helm/helm/releases).

```bash
helm install kruise https://github.com/openkruise/kruise/releases/download/v0.3.1/kruise-chart.tgz
```

Verify Kruise-manager is running:

```bash
$ kubectl get pods -n kruise-system
NAME                          READY   STATUS    RESTARTS   AGE
kruise-controller-manager-0   1/1     Running   0          4m11s
```

## Add AppHub as your helm repo

Add the AppHub repository to your Helm, for some of the tutorial charts are held in it:

```bash
helm repo add apphub https://apphub.aliyuncs.com
```

## Try tutorials

- [Use advanced StatefulSet to install Guestbook app](./advanced-statefulset.md)
- [Use SidecarSet to inject a sidecar container](./sidecarset.md)
- [Use Broadcast Job to pre-download image](./broadcastjob.md)
- [Run a UnitedDeployment in a multi-domain cluster](./uniteddeployment.md)
- [Deploy Guestbook using CloneSet](./cloneset.md)

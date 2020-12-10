# Tutorial

These tutorials walk through several examples to demonstrate how to use the Kruise Workloads to deploy and manage applications

[Install Kruise](../../README.md)

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
- [Use advanced DaemonSet to deploy daemons](./advanced-daemonset.md)

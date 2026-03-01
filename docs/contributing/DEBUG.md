# How to Debug Your Kruise

## Way 1: Debug your kruise with Pod (Recommended)
### First Step: Start your kubernetes cluster
#### Requirements:
- Linux system, MacOS, or Windows Subsystem for Linux 2.0 (WSL 2)
- Docker installed (follow the [official docker installation guide](https://docs.docker.com/get-docker/) to install if need be)
- Kubernetes cluster >= v1.16.0
- Kubectl installed and configured

Kruise relies on Kubernetes as control plane. The control plane could be any managed Kubernetes offering or your own cluster.

For local deployment and test, you could use [kind](https://kind.sigs.k8s.io/) or [minikube](https://minikube.sigs.k8s.io/docs/start/). For production usage, you could use Kubernetes services provided by cloud providers.
#### Option 1: Start your kubernetes cluster with kind (Recommended)
Follow [this guide](https://kind.sigs.k8s.io/docs/user/quick-start/#installation) to install kind.

Then spins up a kind cluster:
```shell
cat <<EOF | kind create cluster --image=kindest/node:v1.22.15 --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
- role: worker
- role: worker
- role: worker
EOF
```
#### Option 2: Start your kubernetes cluster with minikube
Follow the [minikube installation guide](https://minikube.sigs.k8s.io/docs/start/).

Then spins up a MiniKube cluster:
```shell
minikube start
```

### Second Step: Deploy your kruise with a deployment
#### 1. Generate code and manifests in your branch
Make your own code changes and validate the build by running `make generate manifests` in Kruise directory.
#### 2. Deploy customized controller manager
The deployment can be done by following steps.
- Prerequisites: prepare a `kruise` repository in an image registry, it can be [docker hub](https://hub.docker.com/) or your private hub.
- step 1: `export IMG=<image_name>` to specify the target image name. e.g., `export IMG=$DOCKERID/kruise:test`;
- step 2: `make docker-build` to build the image locally and `make docker-push` to push the image to registry;
- step 3: `export KUBECONFIG=<your_k8s_config>` to specify the k8s cluster config. e.g., `export KUBECONFIG=$~/.kube/config`;
- step 4:
  - 4.1: `make deploy` to deploy Kruise to the k8s cluster with the `IMG` you have packaged, if the cluster has not installed Kruise or has installed via `make deploy`;
  - 4.2: if the cluster has installed Kruise via helm chart, we suggest you just update your `IMG` into it with `kubectl set image -n kruise-system deployment kruise-controller-manager manager=${IMG}`;

Tips:
- You have to run `./scripts/uninstall.sh` to uninstall Kruise if you installed it using `make deploy`.

#### 3.View logs of your kruise
You can perform manual tests and use `kubectl logs -n kruise-system <kruise-controller-manager-pod-name>` to check controller logs for debugging, and you can see your `<kruise-controller-manager-pod-name>` by applying `kubectl get pod -n kruise-system`.

## Way 2: Debug your kruise locally (NOT Recommended)
Kubebuilder default `make run` does not work for webhooks since its scaffolding code starts webhook server
using kubernetes service and the service usually does not work in local dev environment.

We workarounds this problem by allowing to start webbook server in local host directly.
With this fix, one can start/debug kruise-manager process locally
which connects to a local or remote Kubernetes cluster. Several extra steps are needed to make it work:

**Setup host and run your kruise**

First, make sure your kubernetes cluster is running.

Second, **make sure `kube-apiserver` could connect to your local machine.**

Then, run kruise locally with `WEBHOOK_HOST` env:

```bash
export KUBECONFIG=${PATH_TO_CONFIG}
export WEBHOOK_HOST=${YOUR_LOCAL_IP}

make install
make run
```

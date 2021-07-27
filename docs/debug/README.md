# How to DEBUG

## DEBUG: start with process locally

Kubebuilder default `make run` does not work for webhooks since its scaffolding code starts webhook server
using kubernetes service and the service usually does not work in local dev environment.

We workarounds this problem by allowing to start webbook server in local host directly.
With this fix, one can start/debug kruise-manager process locally
which connects to a local or remote Kubernetes cluster. Several extra steps are needed to make it work:

**Setup host**

First, make sure `kube-apiserver` could connect to your local machine.

Then, run kruise locally with `WEBHOOK_HOST` env:

```bash
export KUBECONFIG=${PATH_TO_CONFIG}
export WEBHOOK_HOST=${YOUR_LOCAL_IP}

make install
make run
```

## DEBUG: start with Pod

The followings are the steps to debug Kruise controller manager locally using Pod. Note that the
`WEBHOOK_HOST` env variable should be unset if you have tried [above](#debug-start-with-process-locally)
debugging methodology before.

**Install docker**

Following the [official docker installation guide](https://docs.docker.com/get-docker/).

**Install minikube**

Follow the [official minikube installation guide](https://kubernetes.io/docs/tasks/tools/install-minikube/).

**Develop locally**

Make your own code changes and validate the build by running `make manager` and `make manifests` in Kruise directory.

**Deploy customized controller manager**

The new controller manager will be deployed via a `Deployment` to replace the default Kruise controller manager.
The deployment can be done by following steps assuming a fresh environment:

* Prerequisites: create new/use existing [dock hub](https://hub.docker.com/) account ($DOCKERID), and create a `kruise` repository in it;
* step 1: `docker login` with the $DOCKERID account;
* step 2: `export IMG=<image_name>` to specify the target image name. e.g., `export IMG=$DOCKERID/kruise:test`;
* step 3: `make docker-build` to build the image locally;
* step 4: `make docker-push` to push the image to dock hub under the `kruise` repository;
* step 5: `export KUBECONFIG=<your_k8s_config>` to specify the k8s cluster config. e.g., `export KUBECONFIG=$~/.kube/config`;
* step 6: `make deploy IMG=${IMG}` to deploy your kruise-controller-manager to the k8s cluster;

Tips:
* If you need to update `mutatingwebhookconfiguration` or `validatingwebhookconfiguration` of kruise, please run `./scripts/uninstall.sh` in Kruise directory to uninstall them `before step 6`.
* You can perform manual tests and use `kubectl logs <kruise-pod-name> -n kruise-system` to check controller logs for debugging, and you can see your `<kruise-pod-name>` by applying `kubectl get pod -n kruise-system`.
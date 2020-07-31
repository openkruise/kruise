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

Following the [official docker installation guide](https://docs.docker.com/install/).

**Install minikube**

Follow the [official minikube installation guide](https://kubernetes.io/docs/tasks/tools/install-minikube/).

**Develop locally**

Make your own code changes and validate the build by running `make manager` in Kruise directory.

**Deploy customized controller manager**

The new controller manager will be deployed via a statefulset to replace the default Kruise controller manager.
The deployment can be done by following steps assuming a fresh environment:

* Prerequisites: create new/use existing [dock hub](https://hub.docker.com/) account ($DOCKERID), and create a `kruise` repository in it. Also, [install Kruise CRDs](../../README.md#install-crds);
* step 1: `docker login` with the $DOCKERID account;
* step 2: `export IMG=<image_name>` to specify the target image name. e.g., `export IMG=$DOCKERID/kruise:test`;
* step 3: `make docker-build` to build the image locally;
* step 4: `make docker-push` to push the image to dock hub under the `kruise` repository;
* step 5: change the `config/manager/all_in_one.yaml` and replace the container image of the controller manager statefulset to `$DOCKERID/kruise:test`

```yaml
spec:
      containers:
        - command:
            - /manager
          args:
            - "--metrics-addr=127.0.0.1:8080"
            - "--logtostderr=true"
            - "--v=4"
          env:
            - name: POD_NAMESPACE
              valueFrom:
                fieldRef:
                  fieldPath: metadata.namespace
            - name: SECRET_NAME
              value: kruise-webhook-server-secret
          image: $DOCKERID/kruise:test
          imagePullPolicy: Always
          name: manager
```

* step 6: `kubectl delete sts kruise-controller-manager -n kruise-system` to remove the old statefulset if any;
* step 7: `kubectl apply -f config/manager/all_in_one.yaml` to install the new statefulset with the customized controller manager image;

Then one can perform manual tests and use `kubectl logs kruise-controller-manager-0 -n kruise-system` to check controller logs for debugging.


The followings are the steps to debug Kruise controller manager locally using Pod.

**Install docker**

Following the [official docker installation guide](https://docs.docker.com/install/).

**Install minikube**

Follow the [official minikube installation guide](https://kubernetes.io/docs/tasks/tools/install-minikube/).

**Develop locally**

Make your own code changes and validate the build by running `make manager` in Kruise directory.

**Deploy customized controller manager**

The new controller manager will be deployed via a statefulset to replace the default Kruise controller manager.
The deployment can be done by following steps assuming a fresh environment:

* Prerequisites: [install Kruise CRDs](../../README.md#install-crds);
* step 1: `eval $(minikube docker-env)` configure your local environment to re-use the Docker daemon inside the minikube instance;
* step 2: `export NO_PROXY=${your minikube virtual ip}` neglect ip proxy;
* step 3: `export IMG=<image_name>` to specify the target image name. e.g., `export IMG=openkruise/kruise:test`;
* step 4: `make docker-build` to build the image locally;
* step 5: change the `config/manager/all_in_one.yaml` and replace the container image of the controller manager statefulset to `openkruise/kruise:test`

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
          image: openkruise/kruise:test
          imagePullPolicy: Always
          name: manager
```

* step 6: `kubectl delete sts kruise-controller-manager -n kruise-system` to remove the old statefulset if any;
* step 7: `kubectl apply -f config/manager/all_in_one.yaml` to install the new statefulset with the customized controller manager image;

Then one can perform manual tests and use `kubectl logs kruise-controller-manager-0 -n kruise-system` to check controller logs for debugging.

Notes:

* Step 1, 2, 3 are one-time efforts.
* Kubebuilder default `make run` does not work for webhooks since kubernetes services usually do not work in local dev environment. Hence, it is recommended to debug controller manager in Pod.


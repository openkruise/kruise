# Run a BroadcastJob to pre-download a new image

This tutorial walks you through an example to pre-download an image on nodes with broadcastjob.

## Verify nodes do not have images present

Below command should output nothing.

```
kubectl get nodes -o yaml | grep "openkruise/guestbook:v3"
```

## Run a broadcastjob to download the images

`kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/docs/tutorial/v1/broadcastjob.yaml`

Check the broadcastjob is completed. `bcj` is short for `broadcastjob`

```
$ kubectl get bcj
NAME             DESIRED   ACTIVE   SUCCEEDED   FAILED   AGE
download-image   3         0        3           0        7s
```

Check the pods are completed.

```
$ kubectl get pods
NAME                            READY   STATUS      RESTARTS   AGE
download-image-v99xz            0/1     Completed   0          61s
download-image-xmpkt            0/1     Completed   0          61s
download-image-zc4t4            0/1     Completed   0          61s
```

## Verify images are downloaded on nodes

Now run the same command and check that the images have been downloaded. The testing cluster has 3 nodes. So below command
will output three entries.

```
$ kubectl get nodes -o yaml | grep "openkruise/guestbook:v3"
      - openkruise/guestbook:v3
      - openkruise/guestbook:v3
      - openkruise/guestbook:v3
```

The broadcastjob is configured with `ttlSecondsAfterFinished` to `60`, meaning the job and its associated pods will be deleted
in `60` seconds after the job is finished.

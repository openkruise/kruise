# Apply VolumeSnapshot CRDs
kubectl apply -f snapshot/snapshot.storage.k8s.io_volumesnapshotclasses.yaml
kubectl apply -f snapshot/snapshot.storage.k8s.io_volumesnapshotcontents.yaml
kubectl apply -f snapshot/snapshot.storage.k8s.io_volumesnapshots.yaml

# Create snapshot controller
kubectl apply -f snapshot/rbac-snapshot-controller.yaml
kubectl apply -f snapshot/setup-snapshot-controller.yaml

# Deploy fake csi
bash ./csi_driver/deploy-hostpath.sh

# Deploy simple storage class
kubectl apply -f allow-expansion-sc.yaml
kubectl apply -f disallow-expansion-sc.yaml


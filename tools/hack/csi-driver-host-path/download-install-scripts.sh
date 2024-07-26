# Change to the latest supported snapshotter release branch
SNAPSHOTTER_BRANCH=release-6.3
mkdir -p snapshot;
# Apply VolumeSnapshot CRDs
wget https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/${SNAPSHOTTER_BRANCH}/client/config/crd/snapshot.storage.k8s.io_volumesnapshotclasses.yaml -O ./snapshot/snapshot.storage.k8s.io_volumesnapshotclasses.yaml
wget https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/${SNAPSHOTTER_BRANCH}/client/config/crd/snapshot.storage.k8s.io_volumesnapshotcontents.yaml -O ./snapshot/snapshot.storage.k8s.io_volumesnapshotcontents.yaml
wget https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/${SNAPSHOTTER_BRANCH}/client/config/crd/snapshot.storage.k8s.io_volumesnapshots.yaml -O ./snapshot/snapshot.storage.k8s.io_volumesnapshots.yaml

# Change to the latest supported snapshotter version
SNAPSHOTTER_VERSION=v6.3.3

# Create snapshot controller
wget https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/${SNAPSHOTTER_VERSION}/deploy/kubernetes/snapshot-controller/rbac-snapshot-controller.yaml -O ./snapshot/rbac-snapshot-controller.yaml
wget https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/${SNAPSHOTTER_VERSION}/deploy/kubernetes/snapshot-controller/setup-snapshot-controller.yaml -O ./snapshot/setup-snapshot-controller.yaml

mkdir -p ./csi_driver/hostpath

CSI_DRIVER_VERSION=master
CSI_DRIVER_DEPLOY_DIR=kubernetes-1.27
wget https://raw.githubusercontent.com/kubernetes-csi/csi-driver-host-path/${CSI_DRIVER_VERSION}/deploy/util/deploy-hostpath.sh -O ./csi_driver/deploy-hostpath.sh
wget https://raw.githubusercontent.com/kubernetes-csi/csi-driver-host-path/${CSI_DRIVER_VERSION}/deploy/util/destroy-hostpath.sh -O ./csi_driver/destroy-hostpath.sh
wget https://raw.githubusercontent.com/kubernetes-csi/csi-driver-host-path/${CSI_DRIVER_VERSION}/deploy/${CSI_DRIVER_DEPLOY_DIR}/hostpath/csi-hostpath-driverinfo.yaml -O ./csi_driver/hostpath/csi-hostpath-driverinfo.yaml
wget https://raw.githubusercontent.com/kubernetes-csi/csi-driver-host-path/${CSI_DRIVER_VERSION}/deploy/${CSI_DRIVER_DEPLOY_DIR}/hostpath/csi-hostpath-plugin.yaml -O ./csi_driver/hostpath/csi-hostpath-plugin.yaml
wget https://raw.githubusercontent.com/kubernetes-csi/csi-driver-host-path/${CSI_DRIVER_VERSION}/deploy/${CSI_DRIVER_DEPLOY_DIR}/hostpath/csi-hostpath-snapshotclass.yaml -O ./csi_driver/hostpath/csi-hostpath-snapshotclass.yaml
wget https://raw.githubusercontent.com/kubernetes-csi/csi-driver-host-path/${CSI_DRIVER_VERSION}/deploy/${CSI_DRIVER_DEPLOY_DIR}/hostpath/csi-hostpath-testing.yaml -O ./csi_driver/hostpath/csi-hostpath-testing.yaml

#wget https://raw.githubusercontent.com/kubernetes-csi/csi-driver-host-path/master/examples/csi-storageclass.yaml

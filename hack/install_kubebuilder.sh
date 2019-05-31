version=1.0.8 # latest stable version
arch=amd64

# download the release
curl -L -O "https://github.com/kubernetes-sigs/kubebuilder/releases/download/v${version}/kubebuilder_${version}_darwin_${arch}.tar.gz"

# extract the archive
tar -zxvf kubebuilder_${version}_darwin_${arch}.tar.gz
mv kubebuilder_${version}_darwin_${arch} kubebuilder && sudo mv kubebuilder /usr/local/

# update your PATH to include /usr/local/kubebuilder/bin
export PATH=$PATH:/usr/local/kubebuilder/bin
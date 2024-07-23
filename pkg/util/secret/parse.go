package secret

import (
	"context"
	"fmt"

	daemonutil "github.com/openkruise/kruise/pkg/daemon/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/credentialprovider"
	credentialprovidersecrets "k8s.io/kubernetes/pkg/credentialprovider/secrets"
)

func AuthInfos(ctx context.Context, imageName, tag string, pullSecrets []corev1.Secret) []daemonutil.AuthInfo {
	imageRef := fmt.Sprintf("%s:%s", imageName, tag)
	ref, err := daemonutil.NormalizeImageRef(imageRef)
	if err != nil {
		return nil
	}
	var registry = daemonutil.ParseRegistry(ref.String())
	authInfos, _ := ConvertToRegistryAuths(pullSecrets, registry)
	return authInfos
}

var (
	keyring credentialprovider.DockerKeyring
)

// make and set new docker keyring
func MakeAndSetKeyring() {
	klog.Info("make and set new docker keyring")
	keyring = credentialprovider.NewDockerKeyring()
}

func ConvertToRegistryAuths(pullSecrets []corev1.Secret, repo string) (infos []daemonutil.AuthInfo, err error) {
	if keyring == nil {
		MakeAndSetKeyring()
	}
	keyring, err := credentialprovidersecrets.MakeDockerKeyring(pullSecrets, keyring)
	if err != nil {
		return nil, err
	}
	creds, withCredentials := keyring.Lookup(repo)
	if !withCredentials {
		return nil, nil
	}
	for _, c := range creds {
		infos = append(infos, daemonutil.AuthInfo{
			Username: c.Username,
			Password: c.Password,
		})
	}
	return infos, nil
}

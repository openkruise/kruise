package secret

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/credentialprovider"
	"k8s.io/kubernetes/pkg/credentialprovider/plugin"
	credentialprovidersecrets "k8s.io/kubernetes/pkg/credentialprovider/secrets"

	daemonutil "github.com/openkruise/kruise/pkg/daemon/util"
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
	// Use NewExternalCredentialProviderDockerKeyring to include any registered
	// credential provider plugins. Pass empty strings for pod info since this
	// is used for general image pulling, not per-pod credentials.
	keyring = plugin.NewExternalCredentialProviderDockerKeyring("", "", "", "")
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

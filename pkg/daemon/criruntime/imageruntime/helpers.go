/*
Copyright 2021 The Kruise Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package imageruntime

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	dockermessage "github.com/docker/docker/pkg/jsonmessage"
	daemonutil "github.com/openkruise/kruise/pkg/daemon/util"
	"github.com/openkruise/kruise/pkg/util"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/credentialprovider"
	credentialprovidersecrets "k8s.io/kubernetes/pkg/credentialprovider/secrets"
)

var (
	keyring = credentialprovider.NewDockerKeyring()
)

func convertToRegistryAuths(pullSecrets []v1.Secret, repo string) (infos []daemonutil.AuthInfo, err error) {
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

//// Auths struct contains an embedded RegistriesStruct of name auths
//type Auths struct {
//	Registries RegistriesStruct `json:"auths"`
//}
//
//// RegistriesStruct is a map of registries
//type RegistriesStruct map[string]struct {
//	Username string `json:"username"`
//	Password string `json:"password"`
//	Email    string `json:"email"`
//	Auth     string `json:"auth"`
//}
//
//func convertToRegistryAuthInfo(secret v1.Secret, registry string) (*daemonutil.AuthInfo, error) {
//	auths := Auths{}
//	if secret.Type == v1.SecretTypeOpaque {
//		return &daemonutil.AuthInfo{
//			Username: string(secret.Data["username"]),
//			Password: string(secret.Data["password"]),
//		}, nil
//	}
//
//	if secretData, ok := secret.Data[".dockerconfigjson"]; ok && secret.Type == v1.SecretTypeDockerConfigJson {
//		if err := json.Unmarshal(secretData, &auths); err != nil {
//			klog.Errorf("Error unmarshalling .dockerconfigjson from %s/%s: %v", secret.Namespace, secret.Name, err)
//			return nil, err
//		}
//
//	} else if dockerCfgData, ok := secret.Data[".dockercfg"]; ok && secret.Type == v1.SecretTypeDockercfg {
//		registries := RegistriesStruct{}
//		if err := json.Unmarshal(dockerCfgData, &registries); err != nil {
//			klog.Errorf("Error unmarshalling .dockercfg from %s/%s: %v", secret.Namespace, secret.Name, err)
//			return nil, err
//		}
//		auths.Registries = registries
//	}
//
//	if au, ok := auths.Registries[registry]; ok {
//		return &daemonutil.AuthInfo{
//			Username: au.Username,
//			Password: au.Password,
//		}, nil
//	}
//	return nil, fmt.Errorf("imagePullSecret %s/%s contains neither .dockercfg nor .dockerconfigjson", secret.Namespace, secret.Name)
//}

//func containsImage(c []ImageInfo, name string, tag string) bool {
//	for _, info := range c {
//		for _, repoTag := range info.RepoTags {
//			imageRepo, imageTag := daemonutil.ParseRepositoryTag(repoTag)
//			if imageRepo == name && imageTag == tag {
//				return true
//			}
//		}
//	}
//	return false
//}

type layerProgress struct {
	*dockermessage.JSONProgress
	Status string `json:"status,omitempty"` //Extracting,Pull complete,Pulling fs layer,Verifying Checksum,Downloading
}

type pullingProgress struct {
	Layers        map[string]layerProgress `json:"layers,omitempty"`
	TotalStatuses []string                 `json:"totalStatuses,omitempty"`
}

func newPullingProgress() *pullingProgress {
	return &pullingProgress{
		Layers: make(map[string]layerProgress),
	}
}

func (pp *pullingProgress) getProgressPercent() int32 {
	current := int64(0)
	total := int64(0)
	for _, layerProgress := range pp.Layers {
		if layerProgress.JSONProgress != nil {
			current = current + layerProgress.Current
			total = total + layerProgress.Total
		}
	}

	if total == int64(0) {
		return 0
	}
	return int32(current * 100 / total)
}

type imagePullStatusReader struct {
	ch     chan ImagePullStatus
	done   chan struct{}
	reader io.ReadCloser
}

// newImagePullStatusReader create a progress reader
func newImagePullStatusReader(reader io.ReadCloser) ImagePullStatusReader {
	r := &imagePullStatusReader{
		ch:     make(chan ImagePullStatus, 1),
		done:   make(chan struct{}),
		reader: reader,
	}
	go r.mainloop()
	return r
}

func (r *imagePullStatusReader) C() <-chan ImagePullStatus {
	return r.ch
}

func (r *imagePullStatusReader) Close() {
	close(r.done)
}

func (r *imagePullStatusReader) seedPullStatus(s ImagePullStatus) {
	for {
		// clean the channel
		select {
		case <-r.ch:
		default:
		}
		// send status
		select {
		case r.ch <- s:
			return
		default:
		}
	}
}

func (r *imagePullStatusReader) mainloop() {
	defer r.reader.Close()
	decoder := json.NewDecoder(r.reader)
	progress := newPullingProgress()
	// ticker := time.NewTicker(10 * time.Millisecond)
	// defer ticker.Stop()
	for {
		select {
		case <-r.done:
			return
		default:
			var jm dockermessage.JSONMessage
			err := decoder.Decode(&jm)
			if err == io.EOF {
				klog.V(5).Info("runtime read eof")
				r.seedPullStatus(ImagePullStatus{Process: 100, Finish: true})
				return
			} else if err != nil {
				klog.V(5).Infof("runtime read err %v", err)
				r.seedPullStatus(ImagePullStatus{Err: err, Finish: true})
				return
			} else if jm.Error != nil {
				klog.V(5).Infof("runtime read err %v", jm.Error)
				r.seedPullStatus(ImagePullStatus{Err: fmt.Errorf("get error in pull response: %+v", jm.Error), Finish: true})
				return
			}

			klog.V(5).Infof("runtime read progress %v", util.DumpJSON(jm))
			if jm.ID != "" {
				progress.Layers[jm.ID] = layerProgress{
					JSONProgress: jm.Progress,
					Status:       jm.Status,
				}
			} else if jm.Status != "" {
				progress.TotalStatuses = append(progress.TotalStatuses, jm.Status)
			}
			currentProgress := progress.getProgressPercent()
			r.seedPullStatus(ImagePullStatus{Process: int(currentProgress), DetailInfo: util.DumpJSON(progress)})
		}
	}
}

func (c ImageInfo) ContainsImage(name string, tag string) bool {
	for _, repoTag := range c.RepoTags {
		imageRepo, imageTag := parseRepositoryTag(repoTag)
		if imageRepo == name && imageTag == tag {
			return true
		}
	}
	return false
}

// parseRepositoryTag gets a repos name and returns the right reposName + tag|digest
// The tag can be confusing because of a port in a repository name.
//     Ex: localhost.localdomain:5000/samalba/hipache:latest
//     Digest ex: localhost:5000/foo/bar@sha256:bc8813ea7b3603864987522f02a76101c17ad122e1c46d790efc0fca78ca7bfb
func parseRepositoryTag(repos string) (string, string) {
	n := strings.Index(repos, "@")
	if n >= 0 {
		parts := strings.Split(repos, "@")
		return parts[0], parts[1]
	}
	n = strings.LastIndex(repos, ":")
	if n < 0 {
		return repos, ""
	}
	if tag := repos[n+1:]; !strings.Contains(tag, "/") {
		return repos[:n], tag
	}
	return repos, ""
}

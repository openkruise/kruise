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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	dockermessage "github.com/docker/docker/pkg/jsonmessage"
	daemonutil "github.com/openkruise/kruise/pkg/daemon/util"
	"github.com/openkruise/kruise/pkg/util"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
	runtimeapiv1alpha2 "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"k8s.io/klog/v2"
)

// // Auths struct contains an embedded RegistriesStruct of name auths
// type Auths struct {
//	Registries RegistriesStruct `json:"auths"`
// }
//
// // RegistriesStruct is a map of registries
// type RegistriesStruct map[string]struct {
//	Username string `json:"username"`
//	Password string `json:"password"`
//	Email    string `json:"email"`
//	Auth     string `json:"auth"`
// }
//
// func convertToRegistryAuthInfo(secret v1.Secret, registry string) (*daemonutil.AuthInfo, error) {
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
// }

// func containsImage(c []ImageInfo, name string, tag string) bool {
//	for _, info := range c {
//		for _, repoTag := range info.RepoTags {
//			imageRepo, imageTag := daemonutil.ParseRepositoryTag(repoTag)
//			if imageRepo == name && imageTag == tag {
//				return true
//			}
//		}
//	}
//	return false
// }

type layerProgress struct {
	*dockermessage.JSONProgress
	Status string `json:"status,omitempty"` // Extracting,Pull complete,Pulling fs layer,Verifying Checksum,Downloading
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
			}
			if err != nil {
				klog.V(5).Infof("runtime read err %v", err)
				r.seedPullStatus(ImagePullStatus{Err: err, Finish: true})
				return
			}
			if jm.Error != nil {
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
		// We should remove defaultDomain and officialRepoName in RepoTags by NormalizeImageRefToNameTag method,
		// Because if the user needs to download the image from hub.docker.com, CRI.PullImage will automatically add these when downloading the image
		// Ref: https://github.com/openkruise/kruise/issues/1273
		imageRepo, imageTag, _ := daemonutil.NormalizeImageRefToNameTag(repoTag)
		if imageRepo == name && imageTag == tag {
			return true
		}
	}
	return false
}

func determineImageClientAPIVersion(conn *grpc.ClientConn) (runtimeapi.ImageServiceClient, runtimeapiv1alpha2.ImageServiceClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	klog.V(4).InfoS("Finding the CRI API image version")
	imageClientV1 := runtimeapi.NewImageServiceClient(conn)

	_, err := imageClientV1.ImageFsInfo(ctx, &runtimeapi.ImageFsInfoRequest{})
	if err == nil {
		klog.V(2).InfoS("Using CRI v1 image API")
		return imageClientV1, nil, nil

	} else if status.Code(err) == codes.Unimplemented {
		klog.V(2).InfoS("Falling back to CRI v1alpha2 image API (deprecated in k8s 1.24)")
		return nil, runtimeapiv1alpha2.NewImageServiceClient(conn), nil
	}

	return nil, nil, fmt.Errorf("unable to determine image API version: %w", err)
}

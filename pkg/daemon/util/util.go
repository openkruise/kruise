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

package util

import (
	"fmt"
	"os"
	"strings"
)

// NodeName returns the node name of this daemon
func NodeName() (string, error) {
	nodeName := os.Getenv("NODE_NAME")
	if len(nodeName) == 0 {
		return "", fmt.Errorf("not found NODE_NAME in environments")
	}
	return nodeName, nil
}

// ParseRegistry return the registry of image
func ParseRegistry(imageName string) string {
	idx := strings.Index(imageName, "/")
	if idx == -1 {
		return imageName
	}
	return imageName[0:idx]
}

// ParseRepositoryTag gets a repos name and returns the right reposName + tag|digest
// The tag can be confusing because of a port in a repository name.
//     Ex: localhost.localdomain:5000/samalba/hipache:latest
//     Digest ex: localhost:5000/foo/bar@sha256:bc8813ea7b3603864987522f02a76101c17ad122e1c46d790efc0fca78ca7bfb
func ParseRepositoryTag(repos string) (string, string) {
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

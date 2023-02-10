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

	"github.com/docker/distribution/reference"
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
//
//	Ex: localhost.localdomain:5000/samalba/hipache:latest
//	Digest ex: localhost:5000/foo/bar@sha256:bc8813ea7b3603864987522f02a76101c17ad122e1c46d790efc0fca78ca7bfb
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

// NormalizeImageRefToNameTag normalizes the image reference to name and tag.
func NormalizeImageRefToNameTag(ref string) (string, string, error) {
	namedRef, err := NormalizeImageRef(ref)
	if err != nil {
		return "", "", err
	}
	return reference.FamiliarName(namedRef), getAPITagFromNamedRef(namedRef), nil
}

// getAPITagFromNamedRef returns a tag from the specified reference.
// This function is necessary as long as the docker "server" api expects
// digests to be sent as tags and makes a distinction between the name
// and tag/digest part of a reference.
func getAPITagFromNamedRef(ref reference.Named) string {
	if digested, ok := ref.(reference.Digested); ok {
		return digested.Digest().String()
	}
	ref = reference.TagNameOnly(ref)
	if tagged, ok := ref.(reference.Tagged); ok {
		return tagged.Tag()
	}
	return ""
}

// NormalizeImageRef normalizes the image reference.
func NormalizeImageRef(ref string) (reference.Named, error) {
	named, err := reference.ParseNormalizedNamed(ref)
	if err != nil {
		return nil, err
	}

	if _, ok := named.(reference.NamedTagged); ok {
		if canonical, ok := named.(reference.Canonical); ok {
			newNamed, err := reference.WithName(canonical.Name())
			if err != nil {
				return nil, err
			}
			newCanonical, err := reference.WithDigest(newNamed, canonical.Digest())
			if err != nil {
				return nil, err
			}
			return newCanonical, nil
		}
	}
	return reference.TagNameOnly(named), nil
}

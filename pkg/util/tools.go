/*
Copyright 2019 The Kruise Authors.
Copyright 2016 The Kubernetes Authors.

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
	"math"
	"sync"

	"github.com/docker/distribution/reference"
	intstrutil "k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
	"k8s.io/utils/integer"
)

// SlowStartBatch tries to call the provided function a total of 'count' times,
// starting slow to check for errors, then speeding up if calls succeed.
//
// It groups the calls into batches, starting with a group of initialBatchSize.
// Within each batch, it may call the function multiple times concurrently with its index.
//
// If a whole batch succeeds, the next batch may get exponentially larger.
// If there are any failures in a batch, all remaining batches are skipped
// after waiting for the current batch to complete.
//
// It returns the number of successful calls to the function.
func SlowStartBatch(count int, initialBatchSize int, fn func(index int) error) (int, error) {
	remaining := count
	successes := 0
	index := 0
	for batchSize := integer.IntMin(remaining, initialBatchSize); batchSize > 0; batchSize = integer.IntMin(2*batchSize, remaining) {
		errCh := make(chan error, batchSize)
		var wg sync.WaitGroup
		wg.Add(batchSize)
		for i := 0; i < batchSize; i++ {
			go func(idx int) {
				defer wg.Done()
				if err := fn(idx); err != nil {
					errCh <- err
				}
			}(index)
			index++
		}
		wg.Wait()
		curSuccesses := batchSize - len(errCh)
		successes += curSuccesses
		if len(errCh) > 0 {
			return successes, <-errCh
		}
		remaining -= batchSize
	}
	return successes, nil
}

// CheckDuplicate finds if there are duplicated items in a list.
func CheckDuplicate(list []string) []string {
	tmpMap := make(map[string]struct{})
	var dupList []string
	for _, name := range list {
		if _, ok := tmpMap[name]; ok {
			dupList = append(dupList, name)
		} else {
			tmpMap[name] = struct{}{}
		}
	}
	return dupList
}

func GetIntOrStrPointer(i intstrutil.IntOrString) *intstrutil.IntOrString {
	return &i
}

// IntAbs returns the abs number of the given int number
func IntAbs(i int) int {
	return int(math.Abs(float64(i)))
}

func IsIntPlusAndMinus(i, j int) bool {
	return (i < 0 && j > 0) || (i > 0 && j < 0)
}

// parse container images,
// 1. docker.io/busybox@sha256:a9286defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d
// repo=docker.io/busybox, tag="", digest=sha256:a9286defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d
// 2. docker.io/busybox:latest
// repo=docker.io/busybox, tag=latest, digest=""
func ParseImage(image string) (repo, tag, digest string, err error) {
	refer, err := reference.Parse(image)
	if err != nil {
		return "", "", "", err
	}

	if named, ok := refer.(reference.Named); ok {
		repo = named.Name()
	}
	if tagged, ok := refer.(reference.Tagged); ok {
		tag = tagged.Tag()
	}
	if digested, ok := refer.(reference.Digested); ok {
		digest = digested.Digest().String()
	}
	return
}

//whether image is digest format,
//for example: docker.io/busybox@sha256:a9286defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d
func IsImageDigest(image string) bool {
	_, _, digest, _ := ParseImage(image)
	return digest != ""
}

// 1. image1, image2 are digest image, compare repo+digest
// 2. image1, image2 are normal image, compare repo+tag
// 3. image1, image2 are digest+normal image, don't support compare it, return false
func IsContainerImageEqual(image1, image2 string) bool {
	repo1, tag1, digest1, err := ParseImage(image1)
	if err != nil {
		klog.Errorf("parse image %s failed: %s", image1, err.Error())
		return false
	}

	repo2, tag2, digest2, err := ParseImage(image2)
	if err != nil {
		klog.Errorf("parse image %s failed: %s", image2, err.Error())
		return false
	}

	if IsImageDigest(image1) && IsImageDigest(image2) {
		return repo1 == repo2 && digest1 == digest2
	}

	return repo1 == repo2 && tag1 == tag2
}

// CalculatePartitionReplicas returns absolute value of partition for workload. This func can solve some
// corner cases about percentage-type partition, such as:
// - if partition > "0%" and replicas > 0, we will ensure at least 1 old pod is reserved.
// - if partition < "100%" and replicas > 1, we will ensure at least 1 pod is upgraded.
func CalculatePartitionReplicas(partition *intstrutil.IntOrString, replicasPointer *int32) (int, error) {
	if partition == nil {
		return 0, nil
	}

	replicas := 1
	if replicasPointer != nil {
		replicas = int(*replicasPointer)
	}

	// 'roundUp=true' will ensure at least 1 old pod is reserved if partition > "0%" and replicas > 0.
	pValue, err := intstrutil.GetScaledValueFromIntOrPercent(partition, replicas, true)
	if err != nil {
		return pValue, err
	}

	// if partition < "100%" and replicas > 1, we will ensure at least 1 pod is upgraded.
	if replicas > 1 && pValue == replicas && partition.Type == intstrutil.String && partition.StrVal != "100%" {
		pValue = replicas - 1
	}

	pValue = integer.IntMax(integer.IntMin(pValue, replicas), 0)
	return pValue, nil
}

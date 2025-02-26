/*
Copyright 2025 The Kruise Authors.

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

package api

import (
	"fmt"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
)

// ParseRange parses the start and end value from a string like "1-3"
func ParseRange(s string) (start int, end int, err error) {
	split := strings.Split(s, "-")
	if len(split) != 2 {
		return 0, 0, fmt.Errorf("invalid range %s", s)
	}
	start, err = strconv.Atoi(split[0])
	if err != nil {
		return
	}
	end, err = strconv.Atoi(split[1])
	if err != nil {
		return
	}
	if start > end {
		return 0, 0, fmt.Errorf("invalid range %s", s)
	}
	return
}

// GetReserveOrdinalIntSet returns a set of ints from parsed reserveOrdinal
func GetReserveOrdinalIntSet(r []intstr.IntOrString) sets.Set[int] {
	values := sets.New[int]()
	for _, elem := range r {
		if elem.Type == intstr.Int {
			values.Insert(int(elem.IntVal))
		} else {
			start, end, err := ParseRange(elem.StrVal)
			if err != nil {
				klog.ErrorS(err, "invalid range reserveOrdinal found, an empty slice will be returned", "reserveOrdinal", elem.StrVal)
				return nil
			}
			for i := start; i <= end; i++ {
				values.Insert(i)
			}
		}
	}
	return values
}

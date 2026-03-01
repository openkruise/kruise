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

package util

import (
	"errors"
	"fmt"
)

const maxDefaultTtlsecondsForAlwaysNodeimage = 3600 * 24 * 31

var defaultTtlsecondsForAlwaysNodeimage int32

func SetDefaultTtlForAlwaysNodeimage(t int) error {
	if t > maxDefaultTtlsecondsForAlwaysNodeimage || t < 0 {
		return errors.New(fmt.Sprintf("default-ttlseconds-for-always-nodeimage must be positive and less than %d, current value is %d", maxDefaultTtlsecondsForAlwaysNodeimage, t))
	}

	defaultTtlsecondsForAlwaysNodeimage = int32(t)
	return nil
}

func GetDefaultTtlsecondsForAlwaysNodeimage() int32 {
	return defaultTtlsecondsForAlwaysNodeimage
}

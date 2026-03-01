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
	"math/rand"
	"strconv"
	"sync"
	"testing"
)

func TestKeyedMutex(t *testing.T) {
	NumberOfKey := 5
	NumberOfGoroutine := 100000
	keys := make([]string, 0)
	expectCount := make(map[string]int)
	realCount := make(map[string]*int, NumberOfKey)
	for i := 0; i < NumberOfKey; i++ {
		var x int
		key := strconv.Itoa(i)
		keys = append(keys, key)
		realCount[key] = &x
	}

	km := KeyedMutex{}
	wait := sync.WaitGroup{}
	wait.Add(NumberOfGoroutine)
	for i := 0; i < NumberOfGoroutine; i++ {
		selectedKey := keys[rand.Int()%NumberOfKey]
		expectCount[selectedKey]++
		go func(key string) {
			unlock := km.Lock(key)
			// test two types of unlock function
			defer func() {
				if key == keys[0] {
					unlock()
				} else {
					km.Unlock(key)
				}
			}()
			count := realCount[key]
			*count++
			wait.Done()
		}(selectedKey)
	}

	wait.Wait()
	for key, expected := range expectCount {
		if times, ok := realCount[key]; !ok || *times != expected {
			t.Fatalf("Expected count times: %d, but got %d", expected, *times)
		}
	}
}

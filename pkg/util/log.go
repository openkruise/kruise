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
	"context"

	"github.com/google/uuid"
	"k8s.io/klog/v2"
)

type loggerContextKey struct{}

func NewLogContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, loggerContextKey{}, uuid.New().String())
}

func NewLogContextWithId(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, loggerContextKey{}, id)
}

func FromLogContext(ctx context.Context) klog.Logger {
	value := ctx.Value(loggerContextKey{})
	if value == nil {
		return klog.Background()
	}
	return klog.Background().WithValues("context", value)
}

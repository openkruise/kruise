package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetParentNameAndOrdinal(t *testing.T) {
	tests := []struct {
		name        string
		podName     string
		wantParent  string
		wantOrdinal int32
	}{
		{
			name:        "normal pod name",
			podName:     "foo-42",
			wantParent:  "foo",
			wantOrdinal: 42,
		},
		{
			name:        "multi-segment parent name",
			podName:     "foo-bar-baz-7",
			wantParent:  "foo-bar-baz",
			wantOrdinal: 7,
		},
		{
			name:        "ordinal 0",
			podName:     "foo-0",
			wantParent:  "foo",
			wantOrdinal: 0,
		},
		{
			name:        "no trailing digits",
			podName:     "foo-bar",
			wantParent:  "",
			wantOrdinal: -1,
		},
		{
			name:        "empty string",
			podName:     "",
			wantParent:  "",
			wantOrdinal: -1,
		},
		{
			name:        "large overflow number returns parent but ordinal -1",
			podName:     "foo-9999999999999999999",
			wantParent:  "foo",
			wantOrdinal: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: tt.podName}}
			parent, ordinal := getParentNameAndOrdinal(pod)
			assert.Equal(t, tt.wantParent, parent)
			assert.Equal(t, tt.wantOrdinal, ordinal)
		})
	}
}

func TestGetOrdinal(t *testing.T) {
	tests := []struct {
		name        string
		podName     string
		wantOrdinal int32
	}{
		{
			name:        "valid ordinal",
			podName:     "web-3",
			wantOrdinal: 3,
		},
		{
			name:        "no ordinal",
			podName:     "web",
			wantOrdinal: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: tt.podName}}
			assert.Equal(t, tt.wantOrdinal, GetOrdinal(pod))
		})
	}
}

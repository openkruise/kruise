package sidecarset

import (
	"context"
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/stretchr/testify/assert"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	webhookutil "github.com/openkruise/kruise/pkg/webhook/util"
)

func TestPatchControllerRevisionLabels(t *testing.T) {
	sidecarSet := factorySidecarSet()
	sidecarSet.SetUID("1223344")
	kubeSysNs := &corev1.Namespace{}
	//Note that webhookutil.GetNamespace() return "" here
	kubeSysNs.SetName(webhookutil.GetNamespace())
	kubeSysNs.SetNamespace(webhookutil.GetNamespace())
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sidecarSet, kubeSysNs).Build()
	processor := NewSidecarSetProcessor(fakeClient, record.NewFakeRecorder(10))

	_, latestRevision, _, err := processor.registerLatestRevision(sidecarSet, nil)
	assert.Equal(t, err, nil)
	assert.Equal(t, latestRevision.Revision, int64(1))

	err = processor.patchControllerRevisionLabels(latestRevision, appsv1alpha1.ImagePreDownloadIgnoredKey, "true")
	assert.Equal(t, err, nil)

	revision := &apps.ControllerRevision{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{Namespace: latestRevision.Namespace, Name: latestRevision.Name}, revision)
	assert.Equal(t, err, nil)

	value, ok := revision.Labels[appsv1alpha1.ImagePreDownloadIgnoredKey]
	assert.Equal(t, ok, true)
	assert.Equal(t, value, "true")

}

package update

import (
	"testing"
)

func TestRecordInplaceReason(t *testing.T) {
	tests := []struct {
		name           string
		imageUpdate    bool
		metadataUpdate bool
		resourceUpdate bool
	}{
		{
			name:        "image changed",
			imageUpdate: true,
		},
		{
			name:           "metadata changed",
			metadataUpdate: true,
		},
		{
			name:           "resource changed",
			resourceUpdate: true,
		},

		{
			name:           "image&metadata changed",
			imageUpdate:    true,
			metadataUpdate: true,
		},
		{
			name:           "metadata&resource changed",
			metadataUpdate: true,
			resourceUpdate: true,
		},
		{
			name:           "image&resource changed",
			imageUpdate:    true,
			resourceUpdate: true,
		},

		{
			name:           "three changed",
			imageUpdate:    true,
			metadataUpdate: true,
			resourceUpdate: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updateTypeMetric.Reset()
			RecordInplaceReason(tt.imageUpdate, tt.metadataUpdate, tt.resourceUpdate)
		})
	}
}

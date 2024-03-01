package imageruntime

import (
	"testing"

	dockerapi "github.com/docker/docker/client"
)

func TestCreateRuntimeClientIfNecessary(t *testing.T) {
	tests := []struct {
		name          string
		clientExists  bool
		expectedError error
	}{
		{
			name:          "ClientAlreadyExists",
			clientExists:  true,
			expectedError: nil,
		},
		{
			name:          "ClientDoesNotExist",
			clientExists:  false,
			expectedError: nil, // Assuming that dockerapi.NewClientWithOpts does not return an error in this test environment
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &dockerImageService{
				runtimeURI: "unix:///hostvarrun/docker.sock",
			}
			if tt.clientExists {
				d.client = &dockerapi.Client{}
			}

			err := d.createRuntimeClientIfNecessary()

			if err != tt.expectedError {
				t.Errorf("createRuntimeClientIfNecessary() error = %v, expectedError %v", err, tt.expectedError)
			}

			if !tt.clientExists && d.client == nil {
				t.Errorf("createRuntimeClientIfNecessary() client was not created")
			}
		})
	}
}

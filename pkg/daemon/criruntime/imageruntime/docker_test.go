package imageruntime

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/image"
	dockerapi "github.com/docker/docker/client"
	"github.com/onsi/gomega"
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

func Test_docker_ListImages(t *testing.T) {
	d := &dockerImageService{
		//runtimeURI: "unix:///var/run/docker.sock",
		runtimeURI: "unix:///hostvarrun/docker.sock",
	}
	g := gomega.NewGomegaWithT(t)

	err := d.createRuntimeClientIfNecessary()
	g.Expect(err).NotTo(gomega.HaveOccurred())

	got, err := d.ListImages(context.TODO())
	if err != nil {
		if strings.Contains(err.Error(), "Cannot connect to the Docker daemon") {
			// ignore this test
			t.Log("Cannot connect to the Docker daemon, ignore this test")
			return
		}
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	t.Log(got)

	_, err = d.PullImage(context.TODO(), "busybox", "latest", nil, nil)
	g.Expect(err).NotTo(gomega.HaveOccurred())
}

func Test_newImageCollectionDocker(t *testing.T) {
	type args struct {
		infos []image.Summary
	}
	tests := []struct {
		name string
		args args
		want []ImageInfo
	}{
		{
			name: "test single",
			args: args{
				infos: []image.Summary{
					{
						ID: "sha256:a3ed95caeb02ffe68cdd9fd84406680ae93d633cb16422d00e8a7c22955b46d4",
						RepoTags: []string{
							"busybox:latest",
						},
						RepoDigests: []string{
							"busybox@sha256:a3ed95caeb02ffe68cdd9fd844066",
						},
						Size: 111,
					},
				},
			},
			want: []ImageInfo{
				{
					ID: "sha256:a3ed95caeb02ffe68cdd9fd84406680ae93d633cb16422d00e8a7c22955b46d4",
					RepoTags: []string{
						"busybox:latest",
					},
					RepoDigests: []string{
						"busybox@sha256:a3ed95caeb02ffe68cdd9fd844066",
					},
					Size: 111,
				},
			},
		},
		{
			name: "empty test",
			args: args{
				infos: []image.Summary{},
			},
			want: []ImageInfo{},
		},
		{
			name: "empty element test",
			args: args{
				infos: []image.Summary{
					{},
				},
			},
			want: []ImageInfo{
				{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := newImageCollectionDocker(tt.args.infos); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("newImageCollectionDocker() = %v, want %v", got, tt.want)
			}
		})
	}
}

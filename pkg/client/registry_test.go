package client

import (
	"testing"

	"k8s.io/apimachinery/pkg/version"
)

func TestShouldUpdateResourceByResizeWithKubectlVersionOutput(t *testing.T) {
	originalVersion := curVersion
	defer func() {
		curVersion = originalVersion
	}()

	curVersion = &version.Info{
		Major:      "1",
		Minor:      "19+",
		GitVersion: "v1.19.16-vke.2",
		GitCommit:  "fd3942548f7530629133e9fd9e7e86e0276ae834",
		Platform:   "linux/amd64",
	}

	if ShouldUpdateResourceByResize() {
		t.Fatalf("expected resize subresource to be disabled for kubernetes %s.%s", curVersion.Major, curVersion.Minor)
	}
}

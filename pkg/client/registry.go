package client

import (
	"fmt"
	"regexp"

	"github.com/coreos/go-semver/semver"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/rest"
)

// nonDigitSuffix matches any trailing non-numeric characters in a version component,
// e.g. "28+" or "19+" produced by some private Kubernetes distributions.
var nonDigitSuffix = regexp.MustCompile(`[^0-9].*$`)

var (
	cfg *rest.Config

	defaultGenericClient *GenericClientset

	curVersion = &version.Info{Major: "1", Minor: "30"}
)

// NewRegistry creates clientset by client-go
func NewRegistry(c *rest.Config) error {
	var err error
	defaultGenericClient, err = newForConfig(c)
	if err != nil {
		return err
	}
	curVersion, err = defaultGenericClient.DiscoveryClient.ServerVersion()
	if err != nil {
		return err
	}
	cfgCopy := *c
	cfg = &cfgCopy
	return nil
}

// GetGenericClient returns default clientset
func GetGenericClient() *GenericClientset {
	return defaultGenericClient
}

// GetGenericClientWithName returns clientset with given name as user-agent
func GetGenericClientWithName(name string) *GenericClientset {
	if cfg == nil {
		return nil
	}
	newCfg := *cfg
	newCfg.UserAgent = fmt.Sprintf("%s/%s", cfg.UserAgent, name)
	return newForConfigOrDie(&newCfg)
}

// GetCurrentServerVersion returns current k8s version
func GetCurrentServerVersion() *version.Info {
	return curVersion
}

// ShouldUpdateResourceByResize returns whether should update resource by resize
// The resize sub-resource was introduced in version 1.32, https://github.com/kubernetes/kubernetes/pull/128266
func ShouldUpdateResourceByResize() bool {
	// Some private/custom Kubernetes distributions report Minor versions with a
	// non-numeric suffix (e.g. "28+" or "19+"). Strip it before semver parsing
	// to avoid a panic in semver.New.
	minor := nonDigitSuffix.ReplaceAllString(curVersion.Minor, "")
	if minor == "" {
		minor = "0"
	}
	return semver.New(fmt.Sprintf("%s.%s.0", curVersion.Major, minor)).Compare(*semver.New("1.32.0")) >= 0
}

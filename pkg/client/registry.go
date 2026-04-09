package client

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/coreos/go-semver/semver"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

var (
	cfg *rest.Config

	defaultGenericClient *GenericClientset

	curVersion = &version.Info{Major: "1", Minor: "30"}

	resizeSubresourceVersion = semver.New("1.32.0")
	leadingDigitsRegexp     = regexp.MustCompile(`^(\d+)`)
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
	major := sanitizeVersion(curVersion.Major)
	minor := sanitizeVersion(curVersion.Minor)

	versionStr := fmt.Sprintf("%s.%s.0", major, minor)
	currentSemver, err := semver.NewVersion(versionStr)
	if err != nil {
		klog.ErrorS(err, "Failed to parse current k8s version", "version", versionStr, "originalMajor", curVersion.Major, "originalMinor", curVersion.Minor)
		return false
	}

	return currentSemver.Compare(*resizeSubresourceVersion) >= 0
}

func sanitizeVersion(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "0"
	}

	matches := leadingDigitsRegexp.FindStringSubmatch(v)
	if len(matches) > 1 {
		return matches[1]
	}

	return "0"
}

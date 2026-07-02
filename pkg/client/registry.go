package client

import (
	"fmt"
	"strings"

	"github.com/coreos/go-semver/semver"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/rest"
)

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
func ShouldUpdateResourceByResize() (result bool) {
	defer func() {
		if r := recover(); r != nil {
			// semver.New panics on strings that are not in dotted-tri format
			// (e.g. unexpected non-numeric content from custom build metadata).
			// Treat that as "not >= 1.32" so the caller falls back to the
			// pre-1.32 update path.
			_ = r
			result = false
		}
	}()
	// coreos/go-semver does not strip the leading 'v' that k8s prefixes on
	// GitVersion (e.g. "v1.32.0"). Remove it before parsing.
	v := strings.TrimPrefix(curVersion.GitVersion, "v")
	// digitsOnly cleans each dotted component so cloud-provider build metadata
	// (e.g. "1.32+gke.1" → "1.32.1") does not cause a parse panic. SplitN
	// keeps at most three parts (major/minor/patch); any extra segments are
	// folded into the last part and then trimmed by digitsOnly. Any
	// remaining format exception is caught by the deferred recover above.
	parts := strings.SplitN(v, ".", 3)
	for i, p := range parts {
		parts[i] = digitsOnly(p)
	}
	cur := semver.New(strings.Join(parts, "."))
	return cur.Compare(*semver.New("1.32.0")) >= 0
}

// digitsOnly returns the leading numeric prefix of a version component.
// This handles GKE/EKS-style build metadata embedded in a component
// (e.g. "32+gke" → "32"). If the input has no leading digit, an empty
// string is returned.
func digitsOnly(s string) string {
	for i, r := range s {
		if r < '0' || r > '9' {
			return s[:i]
		}
	}
	return s
}

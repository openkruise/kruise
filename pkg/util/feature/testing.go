package feature

import (
	"fmt"
	"testing"

	"k8s.io/component-base/featuregate"
)

// SetFeatureGateDuringTest sets the specified gate to the specified value, and returns a function that restores the original value.
// Failures to set or restore cause the test to fail.
//
// Example use:
//
// defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.<FeatureName>, true)()
func SetFeatureGateDuringTest(tb testing.TB, gate featuregate.FeatureGate, f featuregate.Feature, value bool) func() {
	originalValue := gate.Enabled(f)

	if err := gate.(featuregate.MutableFeatureGate).Set(fmt.Sprintf("%s=%v", f, value)); err != nil {
		tb.Errorf("error setting %s=%v: %v", f, value, err)
	}

	return func() {
		if err := gate.(featuregate.MutableFeatureGate).Set(fmt.Sprintf("%s=%v", f, originalValue)); err != nil {
			tb.Errorf("error restoring %s=%v: %v", f, originalValue, err)
		}
	}
}

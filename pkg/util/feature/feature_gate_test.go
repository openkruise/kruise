package feature

import (
	"testing"

	"k8s.io/component-base/featuregate"
)

func TestDefaultMutableFeatureGate(t *testing.T) {
	if DefaultMutableFeatureGate == nil {
		t.Error("DefaultMutableFeatureGate should not be nil")
	}

	var _ featuregate.MutableFeatureGate = DefaultMutableFeatureGate

	testFeature := featuregate.Feature("TestFeature")
	err := DefaultMutableFeatureGate.Add(map[featuregate.Feature]featuregate.FeatureSpec{
		testFeature: {Default: false, PreRelease: featuregate.Alpha},
	})
	if err != nil {
		t.Errorf("Failed to add test feature gate: %v", err)
	}
	err = DefaultMutableFeatureGate.Set("TestFeature=true")
	if err != nil {
		t.Errorf("Failed to set test feature gate: %v", err)
	}

	if !DefaultMutableFeatureGate.Enabled(testFeature) {
		t.Error("TestFeature should be enabled after setting it to true")
	}
}

func TestDefaultFeatureGate(t *testing.T) {
	if DefaultFeatureGate == nil {
		t.Error("DefaultFeatureGate should not be nil")
	}
	var _ featuregate.FeatureGate = DefaultFeatureGate

	if DefaultFeatureGate != DefaultMutableFeatureGate {
		t.Error("DefaultFeatureGate should be the same instance as DefaultMutableFeatureGate")
	}
}

func TestFeatureGateInterfaces(t *testing.T) {
	var gate featuregate.FeatureGate = DefaultMutableFeatureGate
	if gate == nil {
		t.Error("DefaultMutableFeatureGate should be usable as FeatureGate")
	}
	var mutableGate featuregate.MutableFeatureGate = DefaultMutableFeatureGate
	if mutableGate == nil {
		t.Error("DefaultMutableFeatureGate should be usable as MutableFeatureGate")
	}
}

func TestFeatureGateBasicOperations(t *testing.T) {
	testFeature := featuregate.Feature("TestBasicOps")

	err := DefaultMutableFeatureGate.Add(map[featuregate.Feature]featuregate.FeatureSpec{
		testFeature: {Default: false, PreRelease: featuregate.Alpha},
	})
	if err != nil {
		t.Errorf("Failed to add test feature: %v", err)
	}
	if DefaultFeatureGate.Enabled(testFeature) {
		t.Error("TestBasicOps should be disabled by default")
	}
	err = DefaultMutableFeatureGate.Set("TestBasicOps=true")
	if err != nil {
		t.Errorf("Failed to enable TestBasicOps: %v", err)
	}
	if !DefaultFeatureGate.Enabled(testFeature) {
		t.Error("TestBasicOps should be enabled after setting to true")
	}
	err = DefaultMutableFeatureGate.Set("TestBasicOps=false")
	if err != nil {
		t.Errorf("Failed to disable TestBasicOps: %v", err)
	}
	if DefaultFeatureGate.Enabled(testFeature) {
		t.Error("TestBasicOps should be disabled after setting to false")
	}
}

func TestFeatureGateWithDefaultTrue(t *testing.T) {
	// Create a test feature that defaults to true
	testFeature := featuregate.Feature("TestDefaultTrue")
	
	err := DefaultMutableFeatureGate.Add(map[featuregate.Feature]featuregate.FeatureSpec{
		testFeature: {Default: true, PreRelease: featuregate.Beta},
	})
	if err != nil {
		t.Errorf("Failed to add test feature: %v", err)
	}
	if !DefaultFeatureGate.Enabled(testFeature) {
		t.Error("TestDefaultTrue should be enabled by default")
	}
	err = DefaultMutableFeatureGate.Set("TestDefaultTrue=false")
	if err != nil {
		t.Errorf("Failed to disable TestDefaultTrue: %v", err)
	}
	if DefaultFeatureGate.Enabled(testFeature) {
		t.Error("TestDefaultTrue should be disabled after setting to false")
	}
}

func TestFeatureGateErrorHandling(t *testing.T) {
	// Test setting an invalid feature gate format
	err := DefaultMutableFeatureGate.Set("InvalidFormat")
	if err == nil {
		t.Error("Setting invalid format should return an error")
	}
	// Test setting a non-existent feature gate
	err = DefaultMutableFeatureGate.Set("NonExistentFeature=true")
	if err == nil {
		t.Error("Setting non-existent feature should return an error")
	}

	// Test adding duplicate feature
	testFeature := featuregate.Feature("TestDuplicate")
	err = DefaultMutableFeatureGate.Add(map[featuregate.Feature]featuregate.FeatureSpec{
		testFeature: {Default: false, PreRelease: featuregate.Alpha},
	})
	if err != nil {
		t.Errorf("Failed to add test feature: %v", err)
	}
	err = DefaultMutableFeatureGate.Add(map[featuregate.Feature]featuregate.FeatureSpec{
		testFeature: {Default: true, PreRelease: featuregate.Beta},
	})
	if err == nil {
		t.Error("Adding duplicate feature should return an error")
	}
}

func TestFeatureGateKnownFeatures(t *testing.T) {
	// Test that we can get the list of known features
	knownFeatures := DefaultFeatureGate.KnownFeatures()
	if len(knownFeatures) == 0 {
		t.Error("Should have some known features")
	}
	for _, featureDesc := range knownFeatures {
		if featureDesc == "" {
			t.Error("Feature description should not be empty")
		}
	}
}

func TestFeatureGateString(t *testing.T) {
	// Test that we can get string representation of known features
	knownFeatures := DefaultFeatureGate.KnownFeatures()
	if len(knownFeatures) == 0 {
		t.Error("Should have some known features to test string representation")
	}
	// Test that known features are strings
	for _, featureStr := range knownFeatures {
		if featureStr == "" {
			t.Error("Feature string should not be empty")
		}
	}
}

func TestFeatureGateDeepCopy(t *testing.T) {
	// Test that we can create a deep copy of the feature gate
	copy := DefaultFeatureGate.DeepCopy()
	if copy == nil {
		t.Error("DeepCopy should not return nil")
	}
	if copy == DefaultFeatureGate {
		t.Error("DeepCopy should return a different instance")
	}

	originalFeatures := DefaultFeatureGate.KnownFeatures()
	copyFeatures := copy.KnownFeatures()
	
	if len(originalFeatures) != len(copyFeatures) {
		t.Error("DeepCopy should have the same number of features")
	}
}

func TestSetFeatureGateDuringTest(t *testing.T) {
	// Create a separate feature gate for testing to avoid conflicts
	testGate := featuregate.NewFeatureGate()
	testFeature := featuregate.Feature("TestSetDuringTest")

	err := testGate.Add(map[featuregate.Feature]featuregate.FeatureSpec{
		testFeature: {Default: false, PreRelease: featuregate.Alpha},
	})
	if err != nil {
		t.Errorf("Failed to add test feature: %v", err)
	}
	initialState := testGate.Enabled(testFeature)
	restore := SetFeatureGateDuringTest(t, testGate, testFeature, !initialState)

	if testGate.Enabled(testFeature) == initialState {
		t.Error("SetFeatureGateDuringTest should have changed the feature gate value")
	}
	restore()

	if testGate.Enabled(testFeature) != initialState {
		t.Error("restore function should have restored the original value")
	}
}

func TestSetFeatureGateDuringTestWithDefer(t *testing.T) {
	testGate := featuregate.NewFeatureGate()
	testFeature := featuregate.Feature("TestSetWithDefer")

	err := testGate.Add(map[featuregate.Feature]featuregate.FeatureSpec{
		testFeature: {Default: true, PreRelease: featuregate.Beta},
	})
	if err != nil {
		t.Errorf("Failed to add test feature: %v", err)
	}
	initialState := testGate.Enabled(testFeature)

	func() {
		defer SetFeatureGateDuringTest(t, testGate, testFeature, false)()

		if testGate.Enabled(testFeature) != false {
			t.Error("Feature should be disabled within the function scope")
		}
	}()

	if testGate.Enabled(testFeature) != initialState {
		t.Error("Feature should be restored to original value after defer")
	}
}

func TestSetFeatureGateDuringTestErrorHandling(t *testing.T) {
	// Test that SetFeatureGateDuringTest works correctly with valid features
	// Create a separate feature gate for testing
	testGate := featuregate.NewFeatureGate()
	testFeature := featuregate.Feature("TestErrorHandling")

	err := testGate.Add(map[featuregate.Feature]featuregate.FeatureSpec{
		testFeature: {Default: false, PreRelease: featuregate.Alpha},
	})
	if err != nil {
		t.Errorf("Failed to add test feature: %v", err)
	}

	restore := SetFeatureGateDuringTest(t, testGate, testFeature, true)
	if restore == nil {
		t.Error("SetFeatureGateDuringTest should return a restore function")
	}

	restore()
}

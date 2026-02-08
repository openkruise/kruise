package v1beta1

const (
	// AnnotationUsingEnhancedLiveness indicates that the enhanced liveness probe of pod is enabled.
	AnnotationUsingEnhancedLiveness = "apps.kruise.io/using-enhanced-liveness"
	// AnnotationUsingEnhancedLiveness indicates the backup probe (json types) of the pod native container livenessProbe configuration.
	AnnotationNativeContainerProbeContext = "apps.kruise.io/container-probe-context"
)

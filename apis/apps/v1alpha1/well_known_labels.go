package v1alpha1

const (
	// ControllerRevisionHashLabelKey is used to record the controller revision of current resource.
	ControllerRevisionHashLabelKey = "apps.kruise.io/controller-revision-hash"

	// SubSetNameLabelKey is used to record the name of current subset.
	SubSetNameLabelKey = "apps.kruise.io/subset-name"

	// SpecifiedDeleteKey indicates this object should be deleted, and the value could be the deletion option.
	SpecifiedDeleteKey = "apps.kruise.io/specified-delete"

	// ImagePreDownloadCreatedKey indicates the images of this revision have been pre-downloaded
	ImagePreDownloadCreatedKey = "apps.kruise.io/pre-predownload-created"

	// ImagePreDownloadIgnoredKey indicates the images of this revision have been ignored to pre-download
	ImagePreDownloadIgnoredKey = "apps.kruise.io/image-predownload-ignored"
	// AnnotationSubsetPatchKey indicates the patch for every subset
	AnnotationSubsetPatchKey = "apps.kruise.io/subset-patch"
)

// Sidecar container environment variable definitions which are used to enable SidecarTerminator to take effect on the sidecar container.
const (
	// KruiseTerminateSidecarEnv is an env name, which represents a switch to enable sidecar terminator.
	// The corresponding value is "true", which means apply a crr to kill sidecar using kruise-daemon.
	KruiseTerminateSidecarEnv = "KRUISE_TERMINATE_SIDECAR_WHEN_JOB_EXIT"

	// KruiseTerminateSidecarWithImageEnv is an env name, which refers to an image that will replace the original image
	// using in-place update strategy to kill sidecar. This image must be given if you want to use in-place update
	// strategy to terminate sidecar containers.
	KruiseTerminateSidecarWithImageEnv = "KRUISE_TERMINATE_SIDECAR_WHEN_JOB_EXIT_WITH_IMAGE"
)

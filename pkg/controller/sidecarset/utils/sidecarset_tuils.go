package utils

import (
	"github.com/openkruise/kruise/pkg/control/sidecarcontrol"
	"github.com/openkruise/kruise/pkg/util/expectations"
)

var (
	UpdateExpectations = expectations.NewUpdateExpectations(sidecarcontrol.RevisionAdapterImpl)
)

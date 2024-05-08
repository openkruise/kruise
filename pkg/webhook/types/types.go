package types

import (
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type HandlerGetter = func(manager.Manager) admission.Handler

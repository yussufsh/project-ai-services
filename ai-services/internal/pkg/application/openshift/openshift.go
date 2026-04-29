package openshift

import (
	"github.com/project-ai-services/ai-services/internal/pkg/cli/templates"
	"github.com/project-ai-services/ai-services/internal/pkg/runtime"
	"github.com/project-ai-services/ai-services/internal/pkg/runtime/types"
)

// OpenshiftApplication implements the Application interface for Openshift runtime.
type OpenshiftApplication struct {
	runtime          runtime.Runtime
	templateProvider templates.Template
}

// NewOpenshiftApplication creates a new OpenshiftApplication instance.
func NewOpenshiftApplication(runtimeClient runtime.Runtime) *OpenshiftApplication {
	return &OpenshiftApplication{
		runtime:          runtimeClient,
		templateProvider: nil, // Will be set during deployment
	}
}

// Type returns the runtime type.
func (o *OpenshiftApplication) Type() types.RuntimeType {
	return types.RuntimeTypeOpenShift
}

func (p *OpenshiftApplication) SetTemplateProvider(tp templates.Template) {
	p.templateProvider = tp
}

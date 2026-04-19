package podman

import (
	"github.com/project-ai-services/ai-services/internal/pkg/cli/templates"
	"github.com/project-ai-services/ai-services/internal/pkg/runtime"
	"github.com/project-ai-services/ai-services/internal/pkg/runtime/types"
)

// PodmanApplication implements the Application interface for Podman runtime.
type PodmanApplication struct {
	runtime          runtime.Runtime
	templateProvider templates.Template
}

// NewPodmanApplication creates a new PodmanApplication instance.
func NewPodmanApplication(runtimeClient runtime.Runtime) *PodmanApplication {
	return &PodmanApplication{
		runtime:          runtimeClient,
		templateProvider: nil, // Will be set during deployment
	}
}

// Type returns the runtime type.
func (p *PodmanApplication) Type() types.RuntimeType {
	return types.RuntimeTypePodman
}

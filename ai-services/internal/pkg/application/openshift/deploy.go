package openshift

import (
	"context"
	"fmt"

	"github.com/project-ai-services/ai-services/internal/pkg/application/types"
)

// Deploy deploys either an architecture or a service based on the template name
func (o *OpenshiftApplication) Deploy(ctx context.Context, opts types.CreateOptions) error {
	return fmt.Errorf("deployment for OpenShift is not supported yet  (use --legacy for old applications)")
}

// Made with Bob

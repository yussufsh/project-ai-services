package image

import (
	"fmt"

	"github.com/project-ai-services/ai-services/internal/pkg/logger"
	"github.com/project-ai-services/ai-services/internal/pkg/runtime/podman"
	runtimeTypes "github.com/project-ai-services/ai-services/internal/pkg/runtime/types"
	"github.com/project-ai-services/ai-services/internal/pkg/vars"
	"github.com/spf13/cobra"
)

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Pulls all container images for a given application, architecture, or service template",
	Long: `Pull all container images required for the specified template.
	
	Supports three modes:
	1. Architecture Mode: Pulls images for all services in the architecture
	2. Service Mode: Pulls images for the service and its dependencies
	3. Legacy Mode: Pulls images for legacy application templates (use --legacy flag)`,
	Args: cobra.MaximumNArgs(0),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Once precheck passes, silence usage for any *later* internal errors.
		cmd.SilenceUsage = true

		return pull(templateName)
	},
}

func pull(template string) error {
	if vars.RuntimeFactory.GetRuntimeType() == runtimeTypes.RuntimeTypeOpenShift {
		// Since we do not have templates in OpenShift marking it as unsupported for now
		logger.Warningln("Not supported for openshift runtime")

		return nil
	}

	images, err := listImages(template)
	if err != nil {
		return fmt.Errorf("error listing images: %w", err)
	}

	logger.Infof("Downloading the images for the application... ")
	runtimeClient, err := podman.NewPodmanClient()
	if err != nil {
		return fmt.Errorf("failed to connect to podman: %w", err)
	}

	for _, image := range images {
		if err := runtimeClient.PullImage(image); err != nil {
			return fmt.Errorf("failed to pull the image: %w", err)
		}
	}

	return nil
}

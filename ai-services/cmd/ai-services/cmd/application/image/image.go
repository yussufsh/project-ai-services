package image

import (
	"fmt"

	"github.com/project-ai-services/ai-services/assets"
	"github.com/project-ai-services/ai-services/internal/pkg/catalog"
	"github.com/project-ai-services/ai-services/internal/pkg/cli/templates"
	"github.com/project-ai-services/ai-services/internal/pkg/image"
	"github.com/project-ai-services/ai-services/internal/pkg/logger"
	"github.com/project-ai-services/ai-services/internal/pkg/runtime/types"
	"github.com/spf13/cobra"
)

var (
	templateName string
	useLegacy    bool
)

var ImageCmd = &cobra.Command{
	Use:   "image",
	Short: "Manage application images",
	Long:  ``,
	Args:  cobra.MaximumNArgs(0),
	RunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}

func init() {
	ImageCmd.AddCommand(listCmd)
	ImageCmd.AddCommand(pullCmd)
	ImageCmd.PersistentFlags().StringVarP(&templateName, "template", "t", "", "Application template name (Required)")
	_ = ImageCmd.MarkPersistentFlagRequired("template")
	ImageCmd.PersistentFlags().BoolVar(&useLegacy, "legacy", false, "Use legacy application template mode")
}

// listImages returns all images for a given template
func listImages(template string) ([]string, error) {
	if useLegacy {
		// Legacy mode: use ApplicationFS
		logger.Infof("Using legacy mode for template '%s'\n", template)
		tp := templates.NewEmbedTemplateProvider(templates.EmbedOptions{})
		return image.ListImages(template, "", tp)
	}

	// New mode: try architecture first, then service
	var services []string
	var err error

	if arch, archErr := catalog.LoadArchitecture(template); archErr == nil {
		// Architecture mode - convert ServiceReferences to interface{} slice
		serviceRefs := make([]interface{}, len(arch.Services))
		for i, svc := range arch.Services {
			serviceRefs[i] = svc
		}
		services, err = catalog.ResolveServiceDependencies(serviceRefs...)
	} else if _, svcErr := catalog.LoadService(template); svcErr == nil {
		// Service mode
		services, err = catalog.ResolveServiceDependencies(template)
	} else {
		return nil, fmt.Errorf("template '%s' is neither a valid architecture nor service (use --legacy for old applications)", template)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to resolve services: %w", err)
	}

	tp := templates.NewEmbedTemplateProvider(templates.EmbedOptions{
		FS:      &assets.CatalogFS,
		Root:    "services",
		Runtime: types.RuntimeTypePodman,
	})

	// Collect images from all services
	imageSet := make(map[string]bool)
	for _, serviceID := range services {
		images, err := image.ListImages(serviceID, "", tp)
		if err != nil {
			logger.Infof("Warning: failed to list images for service '%s': %v\n", serviceID, err)
			continue
		}

		for _, img := range images {
			imageSet[img] = true
		}
	}

	// Convert set to slice
	allImages := make([]string, 0, len(imageSet))
	for img := range imageSet {
		allImages = append(allImages, img)
	}

	return allImages, nil
}

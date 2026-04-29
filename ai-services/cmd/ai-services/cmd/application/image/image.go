package image

import (
	"fmt"

	"github.com/project-ai-services/ai-services/assets"
	"github.com/project-ai-services/ai-services/internal/pkg/catalog"
	"github.com/project-ai-services/ai-services/internal/pkg/cli/templates"
	"github.com/project-ai-services/ai-services/internal/pkg/image"
	"github.com/project-ai-services/ai-services/internal/pkg/logger"
	"github.com/project-ai-services/ai-services/internal/pkg/utils"
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

		img := &image.Images{
			AppTemplate:      template,
			TemplateProvider: templates.NewEmbedTemplateProvider(&assets.ApplicationFS),
		}

		return img.ListImages()
	}

	// New mode: resolve template to services
	services, err := catalog.ResolveTemplateToServices(template)
	if err != nil {
		return nil, fmt.Errorf("%w (use --legacy for old applications)", err)
	}

	tp := templates.NewEmbedTemplateProvider(&assets.CatalogFS, "services")

	// Collect images from all services
	imageSet := make(map[string]bool)
	for _, serviceID := range services {
		img := &image.Images{
			AppTemplate:      serviceID,
			TemplateProvider: tp,
		}
		images, err := img.ListImages()
		if err != nil {
			logger.Infof("Warning: failed to list images for service '%s': %v\n", serviceID, err)
			continue
		}

		for _, img := range images {
			imageSet[img] = true
		}
	}

	// Convert set to slice
	allImages := utils.ExtractMapKeys(imageSet)

	return allImages, nil
}

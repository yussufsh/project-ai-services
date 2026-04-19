package model

import (
	"fmt"
	"slices"

	"github.com/project-ai-services/ai-services/assets"
	"github.com/project-ai-services/ai-services/internal/pkg/catalog"
	"github.com/project-ai-services/ai-services/internal/pkg/cli/helpers"
	"github.com/project-ai-services/ai-services/internal/pkg/cli/templates"
	"github.com/project-ai-services/ai-services/internal/pkg/logger"
	runtimeTypes "github.com/project-ai-services/ai-services/internal/pkg/runtime/types"
	"github.com/spf13/cobra"
)

var (
	templateName    string
	hiddenTemplates bool
	useLegacy       bool
)

var ModelCmd = &cobra.Command{
	Use:   "model",
	Short: "Manage application models",
	Long:  ``,
	Args:  cobra.MaximumNArgs(0),
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

func init() {
	ModelCmd.AddCommand(listCmd)
	ModelCmd.AddCommand(downloadCmd)
	ModelCmd.PersistentFlags().BoolVar(&useLegacy, "legacy", false, "Use legacy application template mode")
}

// getModels returns all models for a given template (legacy, architecture, or service)
func getModels(template string) ([]string, error) {
	if useLegacy {
		logger.Infof("Using legacy mode for template '%s'\n", templateName)

		tp := templates.NewEmbedTemplateProvider(templates.EmbedOptions{})

		apps, err := tp.ListApplications(hiddenTemplates)
		if err != nil {
			return nil, fmt.Errorf("failed to list the applications, err: %w", err)
		}

		if !slices.Contains(apps, template) {
			return nil, fmt.Errorf("application template %s does not exist", template)
		}
		return helpers.ListModels(template, "", tp)
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
		Runtime: runtimeTypes.RuntimeTypePodman,
	})

	// Collect models from all services
	modelSet := make(map[string]bool)
	for _, serviceID := range services {

		models, err := helpers.ListModels(serviceID, "", tp)
		if err != nil {
			// Skip services without models
			continue
		}

		for _, model := range models {
			modelSet[model] = true
		}
	}

	// Convert set to slice
	allModels := make([]string, 0, len(modelSet))
	for model := range modelSet {
		allModels = append(allModels, model)
	}

	return allModels, nil
}

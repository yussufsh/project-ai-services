package application

import (
	"errors"
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/project-ai-services/ai-services/assets"
	"github.com/project-ai-services/ai-services/internal/pkg/catalog"
	"github.com/project-ai-services/ai-services/internal/pkg/cli/templates"
	"github.com/project-ai-services/ai-services/internal/pkg/logger"
	"github.com/project-ai-services/ai-services/internal/pkg/vars"
)

var (
	useLegacyTemplate bool
)

var templatesCmd = &cobra.Command{
	Use:   "templates",
	Short: "Lists the offered application templates and their supported parameters",
	Long:  `Retrieves information about the offered application templates and their supported parameters`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Once precheck passes, silence usage for any *later* internal errors.
		cmd.SilenceUsage = true

		if useLegacyTemplate {
			return listLegacyTemplates(cmd)
		}

		return listCatalogTemplates(cmd)
	},
}

func init() {
	templatesCmd.Flags().BoolVar(&useLegacyTemplate, "legacy", false, "List legacy application templates")
}

// listLegacyTemplates lists legacy application templates
func listLegacyTemplates(cmd *cobra.Command) error {
	tp := templates.NewEmbedTemplateProvider(&assets.ApplicationFS)

	appTemplateNames, err := tp.ListApplications(hiddenTemplates)
	if err != nil {
		return fmt.Errorf("failed to list application templates: %w", err)
	}

	if len(appTemplateNames) == 0 {
		logger.Infoln("No application templates found.")

		return nil
	}

	// sort appTemplateNames alphabetically
	sort.Strings(appTemplateNames)

	logger.Infoln("Available application templates:")
	for _, name := range appTemplateNames {
		appTemplatesParametersWithDescription, err := tp.ListApplicationTemplateValues(name)
		if err != nil {
			// Skip applications that don't support the current runtime (silently)
			if errors.Is(err, templates.ErrRuntimeNotSupported) {
				continue
			}
			// Log other errors
			logger.Errorf("failed to list application template values: %v", err)

			continue
		}

		logger.Infof("- %s\n", name)
		var metadata templates.AppMetadata
		if err := tp.LoadMetadata(name, false, &metadata); err != nil {
			logger.Errorf("failed to load application metadata: %v", err)

			continue
		}
		if metadata.Description != "" {
			logger.Infof("  Description: %s", metadata.Description)
		}

		logger.Infoln("\n  Supported Parameters:")
		if len(appTemplatesParametersWithDescription) == 0 {
			logger.Infoln("\t" + "NONE")
		}

		for k, v := range appTemplatesParametersWithDescription {
			logger.Infoln("\t" + k + ":  " + v)
		}
		cmd.Println()
	}

	return nil
}

// listCatalogTemplates lists architectures and services from the catalog
func listCatalogTemplates(cmd *cobra.Command) error {
	// List architectures
	architectures, err := catalog.ListArchitectures()
	if err != nil {
		return fmt.Errorf("failed to list architectures: %w", err)
	}

	// List services
	services, err := catalog.ListServices()
	if err != nil {
		return fmt.Errorf("failed to list services: %w", err)
	}

	if len(architectures) == 0 && len(services) == 0 {
		logger.Infoln("No templates found.")

		return nil
	}

	// Display architectures
	if len(architectures) > 0 {
		// Sort by ID
		sort.Slice(architectures, func(i, j int) bool {
			return architectures[i].ID < architectures[j].ID
		})

		logger.Infoln("Available Architectures:")
		for _, arch := range architectures {
			logger.Infof("- %s\n", arch.ID)
			if arch.Description != "" {
				logger.Infof("  Description: %s\n", arch.Description)
			}
			if len(arch.Services) > 0 {
				logger.Infoln("  Services:")
				for _, svc := range arch.Services {
					if svc.Version != "" {
						optionalStr := ""
						if svc.Optional {
							optionalStr = " (optional)"
						}
						logger.Infof("    - %s (version: %s)%s\n", svc.ID, svc.Version, optionalStr)
					} else {
						optionalStr := ""
						if svc.Optional {
							optionalStr = " (optional)"
						}
						logger.Infof("    - %s%s\n", svc.ID, optionalStr)
					}
				}
			}
			cmd.Println()
		}
	}

	// Display services (ListServices already returns only deployable services)
	if len(services) > 0 {
		// Sort by ID
		sort.Slice(services, func(i, j int) bool {
			return services[i].ID < services[j].ID
		})

		logger.Infoln("Available Services:")
		for _, svc := range services {
			logger.Infof("- %s\n", svc.ID)
			if svc.Description != "" {
				logger.Infof("  Description: %s\n", svc.Description)
			}
			if len(svc.Dependencies) > 0 {
				logger.Infoln("  Dependencies:")
				for _, dep := range svc.Dependencies {
					if dep.Version != "" {
						logger.Infof("    - %s (version: %s)\n", dep.ID, dep.Version)
					} else {
						logger.Infof("    - %s\n", dep.ID)
					}
				}
			}

			// Load and display parameters for the current runtime
			runtimeType := vars.RuntimeFactory.GetRuntimeType().String()
			tp := templates.NewEmbedTemplateProvider(&assets.CatalogFS, "services")

			params, err := tp.ListApplicationTemplateValues(svc.ID)
			if err != nil {
				if !errors.Is(err, templates.ErrRuntimeNotSupported) {
					logger.Errorf("failed to list parameters: %v", err)
				}
			} else if len(params) > 0 {
				logger.Infof("  Supported Parameters (%s):\n", runtimeType)
				for k, v := range params {
					logger.Infof("    %s: %s\n", k, v)
				}
			}
			cmd.Println()
		}
	}

	return nil
}

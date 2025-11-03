package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/project-ai-services/ai-services/internal/pkg/cli/helpers"
	"github.com/project-ai-services/ai-services/internal/pkg/utils"
	"github.com/spf13/cobra"
)

var modelAnnotationKey = "ai-services.io/model"

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
}

func models() ([]string, error) {
	// Fetch all the application Template names
	appTemplateNames, err := helpers.FetchApplicationTemplatesNames()
	if err != nil {
		return nil, fmt.Errorf("failed to list templates: %w", err)
	}

	var appTemplateName string

	if index := fetchAppTemplateIndex(appTemplateNames, templateName); index == -1 {
		return nil, errors.New("provided template name is wrong. Please provide a valid template name")
	} else {
		appTemplateName = appTemplateNames[index]
	}

	applicationPodTemplatesPath := applicationTemplatesPath + appTemplateName

	tmpls, err := helpers.LoadAllTemplates(applicationPodTemplatesPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse the templates: %w", err)
	}

	// Fetch all the models from the pod annotations
	models := func(podSpec helpers.PodSpec) []string {
		modelAnnotations := []string{}
		for key, value := range helpers.FetchPodAnnotations(podSpec) {
			if strings.HasPrefix(key, modelAnnotationKey) {
				modelAnnotations = append(modelAnnotations, value)
			}
		}
		return modelAnnotations
	}

	modelList := []string{}
	for _, podTemplateFileName := range utils.ExtractMapKeys(tmpls) {
		podTemplateFilePath := applicationPodTemplatesPath + "/" + podTemplateFileName

		// load the pod Template
		podSpec, err := helpers.LoadPodTemplate(podTemplateFilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to load pod Template: %s with error: %w", podTemplateFilePath, err)
		}
		modelList = append(modelList, models(*podSpec)...)
	}
	return modelList, nil
}

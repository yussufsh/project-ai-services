package model

import (
	"strings"

	"github.com/spf13/cobra"
)

var templateName string

const (
	applicationTemplatesPath = "applications/"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List models for a given application template",
	Long:  ``,
	Args:  cobra.MaximumNArgs(0),
	RunE: func(cmd *cobra.Command, args []string) error {
		return list(cmd)
	},
}

func init() {
	listCmd.Flags().StringVarP(&templateName, "template", "t", "", "Application template name (Required)")
	listCmd.MarkFlagRequired("template")
}

func list(cmd *cobra.Command) error {
	models, err := models()
	if err != nil {
		return err
	}
	cmd.Println("Models in application template", templateName, ":")
	for _, model := range models {
		cmd.Println("-", model)
	}

	return nil
}

// fetchAppTemplateIndex -> Returns the index of app template if exists, otherwise -1
func fetchAppTemplateIndex(appTemplateNames []string, templateName string) int {
	appTemplateIndex := -1

	for index, appTemplateName := range appTemplateNames {
		if strings.EqualFold(appTemplateName, templateName) {
			appTemplateIndex = index
			break
		}
	}

	return appTemplateIndex
}

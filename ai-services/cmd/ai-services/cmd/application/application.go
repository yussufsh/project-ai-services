package application

import (
	"github.com/spf13/cobra"

	"github.com/project-ai-services/ai-services/cmd/ai-services/cmd/application/image"
	"github.com/project-ai-services/ai-services/cmd/ai-services/cmd/application/model"
)

// ApplicationCmd represents the application command
var ApplicationCmd = &cobra.Command{
	Use:   "application",
	Short: "Deploy and monitor the applications",
	Long:  `The application command helps you deploy and monitor the applications`,
}

func init() {
	ApplicationCmd.AddCommand(templatesCmd)
	ApplicationCmd.AddCommand(createCmd)
	ApplicationCmd.AddCommand(psCmd)
	ApplicationCmd.AddCommand(deleteCmd)
	ApplicationCmd.AddCommand(image.ImageCmd)
	ApplicationCmd.AddCommand(stopCmd)
	ApplicationCmd.AddCommand(startCmd)
	ApplicationCmd.AddCommand(model.ModelCmd)
}

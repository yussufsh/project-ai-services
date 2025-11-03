package model

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var (
	toolImage string
	directory string
)

var downloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download models for a given application template",
	Long:  ``,
	Args:  cobra.MaximumNArgs(0),
	RunE: func(cmd *cobra.Command, args []string) error {
		return download(cmd)
	},
}

func init() {
	downloadCmd.Flags().StringVarP(&templateName, "template", "t", "", "Application template name(Required)")
	downloadCmd.MarkFlagRequired("template")
	downloadCmd.Flags().StringVar(&toolImage, "tool-image", "icr.io/ai-services-private/tools:latest", "Tool image to use for downloading the model(only for the development purpose)")
	downloadCmd.Flags().MarkHidden("tool-image")
	downloadCmd.Flags().StringVar(&directory, "dir", "/var/lib/ai-services/models", "Directory to download the model files")
}

func download(cmd *cobra.Command) error {
	models, err := models()
	if err != nil {
		return err
	}
	cmd.Println("Downloaded Models in application template", templateName, ":")
	for _, model := range models {
		err := downloadModelUsingToolImage(model, directory)
		if err != nil {
			return fmt.Errorf("failed to download model: %w", err)
		}
	}

	return nil
}

func downloadModelUsingToolImage(model, targetDir string) error {
	// check for target model directory, if not present create it
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		err := os.MkdirAll(targetDir, os.ModePerm)
		if err != nil {
			return fmt.Errorf("failed to create target model directory: %w", err)
		}
	}
	fmt.Printf("Downloading model %s to %s\n", model, targetDir)
	command := "podman"
	// All arguments must be passed as a slice of strings
	args := []string{
		"run",
		"-ti",
		"-v",
		fmt.Sprintf("%s:/models:Z", directory),
		toolImage,
		"hf",
		"download",
		model,
		"--local-dir",
		fmt.Sprintf("/models/%s", model),
	}
	cmd := exec.Command(command, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to execute command: %w", err)
	}
	fmt.Printf("Model downloaded successfully\n")
	return nil
}

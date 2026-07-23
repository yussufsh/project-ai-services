package agent

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	agentconfigure "github.com/project-ai-services/ai-services/internal/pkg/agent/configure"
	"github.com/project-ai-services/ai-services/internal/pkg/constants"
	"github.com/project-ai-services/ai-services/internal/pkg/utils"
)

func newConfigureCmd() *cobra.Command {
	var (
		baseDir    string
		caddyImage string
	)

	cmd := &cobra.Command{
		Use:   "configure",
		Short: "Set up the Worker-side Caddy proxy pod (run once before agent start)",
		Long: `Deploy the agent Caddy pod on this Worker LPAR.

The Worker Caddy listens on hostPort 8443 so the control-plane Caddy can
reverse-proxy to it directly.  Its admin API is bound to localhost:2019
so only the agent daemon can register and remove routes dynamically.

This command is idempotent — re-running it after the pod is already
running is safe and will skip the deployment step.

Run this once after 'ai-services bootstrap configure', before 'agent start'.`,
		Example: `  ai-services agent configure
  ai-services agent configure --base-dir /custom/path
  ai-services agent configure --caddy-image icr.io/my-registry/caddy:v2.11.4-0`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			var (
				resolvedDir string
				err         error
			)
			if baseDir == "" {
				resolvedDir = constants.DefaultBaseDir
			} else {
				resolvedDir, err = utils.ValidateBaseDir(baseDir)
				if err != nil {
					return fmt.Errorf("invalid base directory %q: %w", baseDir, err)
				}
			}

			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			return agentconfigure.DeployAgentCaddy(ctx, agentconfigure.Options{
				BaseDir:    resolvedDir,
				CaddyImage: caddyImage,
			})
		},
	}

	cmd.Flags().StringVar(&baseDir, "base-dir", "", fmt.Sprintf("Root data directory on this Worker LPAR (default: %s)", constants.DefaultBaseDir))
	cmd.Flags().StringVar(&caddyImage, "caddy-image", "", "Caddy container image (defaults to the version bundled with this release)")

	return cmd
}

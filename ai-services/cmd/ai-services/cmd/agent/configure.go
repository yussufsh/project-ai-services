package agent

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	agentconfigure "github.com/project-ai-services/ai-services/internal/pkg/agent/configure"
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
		Example: `  ai-services agent configure --base-dir /var/lib/ai-services
  ai-services agent configure --base-dir /var/lib/ai-services --caddy-image icr.io/my-registry/caddy:v2.11.4-0`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			if baseDir == "" {
				return fmt.Errorf("--base-dir is required (e.g. /var/lib/ai-services)")
			}

			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			return agentconfigure.DeployAgentCaddy(ctx, agentconfigure.Options{
				BaseDir:    baseDir,
				CaddyImage: caddyImage,
			})
		},
	}

	cmd.Flags().StringVar(&baseDir, "base-dir", "", "Root data directory on this Worker LPAR (e.g. /var/lib/ai-services)")
	cmd.Flags().StringVar(&caddyImage, "caddy-image", "", "Caddy container image (defaults to the version bundled with this release)")

	return cmd
}

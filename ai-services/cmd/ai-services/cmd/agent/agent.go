// Package agent provides the `ai-services agent` subcommand.
package agent

import "github.com/spf13/cobra"

// AgentCmd returns the cobra command for the agent daemon.
func AgentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Manage the AI Services worker agent",
		Long: `The agent runs on Worker LPARs as a containerised daemon and connects to
the control-plane AgentGateway over a bidirectional gRPC CommandStream.
It executes runtime commands on behalf of the control plane.

Typical workflow on a Worker LPAR:

  1. ai-services bootstrap configure --runtime podman
     Install Podman, configure Spyre cards, ulimits, SELinux, SMT.

  2. Obtain a bootstrap token (on the control plane):
       ai-services catalog agent issue-token

  3. ai-services agent configure --runtime podman --server <host:port> --name <name> --token <token>
     Deploy the agent pod and the Caddy reverse proxy pod.
     The agent daemon starts automatically inside the pod and connects
     to the control plane.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(newConfigureCmd())
	cmd.AddCommand(newStartCmd())
	cmd.AddCommand(newStatusCmd())

	return cmd
}

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
		baseDir     string
		runtimeName string
		httpsPort   int
		domainName  string
		sslCertPath string
		sslKeyPath  string
		// Agent container parameters
		agentServer string
		agentName   string
		agentToken  string
	)

	cmd := &cobra.Command{
		Use:   "configure",
		Short: "Deploy the agent and Caddy pods on this Worker LPAR",
		Long: "Deploy two pods on this Worker LPAR:\n\n" +
			"  ai-services--agent-caddy  Caddy reverse-proxy for external HTTPS traffic to\n" +
			"                            deployed service pods. Admin API bound to a random\n" +
			"                            loopback port assigned at runtime.\n\n" +
			"  ai-services--agent        Agent daemon. Connects to the control-plane\n" +
			"                            AgentGateway and executes runtime commands on\n" +
			"                            behalf of the control plane.\n\n" +
			"This command is idempotent — re-running removes any existing pods and\n" +
			"redeploys fresh.\n\n" +
			"Run once after 'ai-services bootstrap configure'.",
		Example: "  ai-services agent configure --runtime podman --server lpar-0.example.com:9090 --name lpar-1 --token <token>\n" +
			"  ai-services agent configure --runtime podman --server lpar-0.example.com:9090 --name lpar-1 --token <token> --https-port 8443",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			if runtimeName == "" {
				return fmt.Errorf("--runtime is required (podman or openshift)")
			}
			if agentServer == "" {
				return fmt.Errorf("--server is required (control-plane AgentGateway address, e.g. lpar-0.example.com:9090)")
			}
			if agentName == "" {
				return fmt.Errorf("--name is required (name to register this agent under)")
			}
			if agentToken == "" {
				return fmt.Errorf("--token is required (obtain via: ai-services catalog agent issue-token)")
			}

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
				BaseDir:      resolvedDir,
				Runtime:      runtimeName,
				HTTPSPort:    httpsPort,
				DomainName:   domainName,
				SSLCertPath:  sslCertPath,
				SSLKeyPath:   sslKeyPath,
				AgentServer:  agentServer,
				AgentName:    agentName,
				AgentToken:   agentToken,
			})
		},
	}

	cmd.Flags().StringVarP(&runtimeName, "runtime", "r", "", "Local container runtime: podman or openshift (required)")
	cmd.Flags().StringVar(&agentServer, "server", "", "Control-plane AgentGateway address (host:port, required)")
	cmd.Flags().StringVar(&agentName, "name", "", "Name to register this agent under (required)")
	cmd.Flags().StringVar(&agentToken, "token", "", "Single-use bootstrap token (from: ai-services catalog agent issue-token, required)")
	cmd.Flags().StringVar(&baseDir, "base-dir", "", fmt.Sprintf("Root data directory on this Worker LPAR (default: %s)", constants.DefaultBaseDir))
	cmd.Flags().IntVar(&httpsPort, "https-port", 443, "Host port Caddy listens on for external HTTPS traffic")
	cmd.Flags().StringVar(&domainName, "domain-name", "", "Custom domain name for service routes (e.g. example.com). Defaults to <worker-ip>.nip.io")
	cmd.Flags().StringVar(&sslCertPath, "ssl-cert", "", "Path to wildcard SSL certificate (must be used with --ssl-key)")
	cmd.Flags().StringVar(&sslKeyPath, "ssl-key", "", "Path to SSL private key (must be used with --ssl-cert)")

	return cmd
}

package agent

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/project-ai-services/ai-services/internal/pkg/agent/agentbootstrap"
	"github.com/project-ai-services/ai-services/internal/pkg/agent/daemon"
	"github.com/project-ai-services/ai-services/internal/pkg/logger"
	"github.com/project-ai-services/ai-services/internal/pkg/proxy"
	"github.com/project-ai-services/ai-services/internal/pkg/runtime"
	openshiftRuntime "github.com/project-ai-services/ai-services/internal/pkg/runtime/openshift"
	podmanRuntime "github.com/project-ai-services/ai-services/internal/pkg/runtime/podman"
	"github.com/project-ai-services/ai-services/internal/pkg/utils"
)

// newStartCmd returns the hidden "start" subcommand used by the agent container
// entrypoint.  It is not shown in help — users run 'agent configure' instead,
// which deploys the pod that calls this command internally.
func newStartCmd() *cobra.Command {
	var (
		server      string
		agentName   string
		token       string
		runtimeName string
		tlsDir      = agentbootstrap.DefaultAgentTLSDir
	)

	cmd := &cobra.Command{
		Use:    "start",
		Hidden: true, // internal use by the container entrypoint only
		Short:  "Start the agent daemon (called by the container entrypoint)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			if runtimeName == "" {
				return fmt.Errorf("--runtime is required (podman or openshift)")
			}
			if server == "" {
				return fmt.Errorf("--server is required (e.g. server.example.com:9090)")
			}
			if agentName == "" {
				return fmt.Errorf("--name is required")
			}
			if token == "" {
				return fmt.Errorf("--token is required")
			}
			return runStart(server, agentName, token, runtimeName, tlsDir)
		},
	}

	cmd.Flags().StringVar(&server, "server", "", "Control-plane AgentGateway address (host:port)")
	cmd.Flags().StringVar(&agentName, "name", "", "Name to register this agent under")
	cmd.Flags().StringVar(&token, "token", "", "Bootstrap token")
	cmd.Flags().StringVar(&runtimeName, "runtime", "", "Local runtime: podman or openshift")
	cmd.Flags().StringVar(&tlsDir, "tls-dir", tlsDir, "Directory to write TLS material (future mTLS)")

	return cmd
}

func runStart(server, agentName, token, runtimeName, tlsDir string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	cfg := agentbootstrap.Config{
		ControlPlaneURL: server,
		AgentName:       agentName,
		PreSharedToken:  token,
	}

	if err := agentbootstrap.Register(ctx, cfg, tlsDir); err != nil {
		return fmt.Errorf("agent start: registration failed: %w", err)
	}

	rt, err := buildRuntime(runtimeName)
	if err != nil {
		return fmt.Errorf("agent start: %w", err)
	}

	if pc, ok := rt.(*podmanRuntime.PodmanClient); ok {
		injectCaddyManager(pc)
	}

	logger.Infoln("Agent daemon running. Press Ctrl+C to stop.")

	return daemon.New(daemon.Config{
		AgentName:       agentName,
		ControlPlaneURL: server,
		PreSharedToken:  token,
	}, rt).Run(ctx)
}

// injectCaddyManager reads CADDY_ADMIN_URL from the environment (set by the
// agent.yaml.tmpl pod template) and injects a LocalCaddyManager so the daemon
// can register routes with the worker's Caddy pod.
// Both pods are on the shared "ai-services-agent" network, so the caddy pod
// name resolves via Podman DNS inside the container.
func injectCaddyManager(pc *podmanRuntime.PodmanClient) {
	adminURL := utils.GetEnv("CADDY_ADMIN_URL", "")
	mgr, err := proxy.NewLocalCaddyManagerFromEnv(adminURL)
	if err != nil {
		logger.Warningf("Agent: worker Caddy not available (%v) — route registration disabled\n", err)
		return
	}

	pc.SetCaddyManager(mgr)
	logger.Infof("Agent: worker Caddy configured at %s\n", adminURL)
}

// buildRuntime constructs the local runtime for the given name.
func buildRuntime(name string) (runtime.Runtime, error) {
	switch name {
	case "podman":
		rt, err := podmanRuntime.NewPodmanClient()
		if err != nil {
			return nil, fmt.Errorf("failed to initialise Podman runtime: %w", err)
		}
		return rt, nil
	case "openshift":
		rt, err := openshiftRuntime.NewOpenshiftClientWithNamespace("")
		if err != nil {
			return nil, fmt.Errorf("failed to initialise OpenShift runtime: %w", err)
		}
		return rt, nil
	default:
		return nil, fmt.Errorf("unknown runtime %q: must be 'podman' or 'openshift'", name)
	}
}

package agent

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/project-ai-services/ai-services/internal/pkg/catalog/client"
	podmanRuntime "github.com/project-ai-services/ai-services/internal/pkg/runtime/podman"
)

// agentContainerName is the Podman container name for the agent daemon.
// Podman names containers inside a pod as <podName>-<containerName>.
const agentContainerName = "ai-services--agent-agent"

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show live connectivity status for this agent",
		Long: "Query the control plane for the live registry status of this agent.\n\n" +
			"The agent name is read from the running " + agentContainerName + " container's\n" +
			"AGENT_NAME environment variable.\n\n",
		Example: "  ai-services agent status",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			agentName, err := readAgentNameFromContainer()
			if err != nil {
				return err
			}

			return runStatus(agentName)
		},
	}

	return cmd
}

// readAgentNameFromContainer inspects the running agent container and returns
// the value of its AGENT_NAME environment variable.
func readAgentNameFromContainer() (string, error) {
	rt, err := podmanRuntime.NewPodmanClient()
	if err != nil {
		return "", fmt.Errorf("cannot connect to Podman: %w", err)
	}

	ctr, err := rt.InspectContainer(agentContainerName)
	if err != nil {
		return "", fmt.Errorf("agent container %q not found — run 'ai-services agent configure' first: %w", agentContainerName, err)
	}

	name := ctr.Env["AGENT_NAME"]
	if name == "" {
		return "", fmt.Errorf("AGENT_NAME not set in container %q", agentContainerName)
	}

	return name, nil
}

func runStatus(agentName string) error {
	apiClient, err := client.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: cannot connect to control plane API: %v\n", err)
		fmt.Fprintf(os.Stderr, "  Run `ai-services login` first if you have not authenticated.\n")
		return err
	}

	status, err := apiClient.GetAgent(agentName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: could not fetch status for agent %q: %v\n", agentName, err)
		fmt.Fprintf(os.Stderr, "  The agent may not have registered yet. Run: ai-services agent configure\n")
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Live Status (from Control Plane)")
	fmt.Fprintln(w, "--------------------------------")
	fmt.Fprintf(w, "Agent Name:\t%s\n", status.AgentName)
	fmt.Fprintf(w, "Status:\t%s\n", status.Status)
	fmt.Fprintf(w, "Active slots:\t%d\n", status.ActiveSlots)
	if status.LastHeartbeat != "" {
		fmt.Fprintf(w, "Last heartbeat:\t%s\n", status.LastHeartbeat)
	}
	fmt.Fprintf(w, "Checked at:\t%s\n", time.Now().Format(time.RFC3339))
	w.Flush()

	return nil
}

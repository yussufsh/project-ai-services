// Package configure implements the one-time Worker-side setup that deploys the
// agent Caddy pod.  It mirrors the pattern used by catalog configure:
//
//   1. Write the base Caddyfile to <baseDir>/agent/caddy/Caddyfile
//   2. Render agent-caddy.yaml.tmpl with the Caddy image and base dir
//   3. Deploy (or skip if already running) via clipodman.DeployPodAndReadinessCheck
//
// This is intentionally separate from `agent start` so that the daemon loop
// can be restarted freely without re-deploying infrastructure.
package configure

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/project-ai-services/ai-services/assets"
	clipodman "github.com/project-ai-services/ai-services/internal/pkg/cli/podman"
	"github.com/project-ai-services/ai-services/internal/pkg/logger"
	podmodels "github.com/project-ai-services/ai-services/internal/pkg/models"
	"github.com/project-ai-services/ai-services/internal/pkg/runtime/podman"
	"github.com/project-ai-services/ai-services/internal/pkg/specs"
	k8syaml "sigs.k8s.io/yaml"
)

const (
	agentCaddyPodName    = "agent--caddy"
	agentCaddyfileSubDir = "agent/caddy"
	agentCaddyTmplName   = "agent/podman/templates/agent-caddy.yaml.tmpl"
	agentCaddyfileTmpl   = "agent/podman/templates/agent-caddyfile.tmpl"

	dirPerm  = 0o755
	filePerm = 0o644
)

// Options holds the parameters for agent configure.
type Options struct {
	// BaseDir is the root data directory on the Worker, e.g. /var/lib/ai-services.
	// Caddy data is written to <BaseDir>/agent/caddy/.
	BaseDir string
	// CaddyImage is the container image to use for the agent Caddy pod.
	// Defaults to the image specified in assets/agent/podman/values.yaml.
	CaddyImage string
}

// DeployAgentCaddy writes the Caddyfile and deploys the agent Caddy pod.
// If the pod already exists the deployment step is skipped (idempotent).
func DeployAgentCaddy(ctx context.Context, opts Options) error {
	rt, err := podman.NewPodmanClient()
	if err != nil {
		return fmt.Errorf("agent configure: init podman client: %w", err)
	}

	// Resolve Caddy image: flag value takes priority, then values.yaml default.
	caddyImage := opts.CaddyImage
	if caddyImage == "" {
		caddyImage, err = defaultCaddyImage()
		if err != nil {
			return fmt.Errorf("agent configure: resolve caddy image: %w", err)
		}
	}

	// Step 1 — write the Caddyfile to disk so the pod volume-mount picks it up.
	if err := writeAgentCaddyfile(opts.BaseDir); err != nil {
		return err
	}

	// Step 2 — skip if pod already running.
	exists, err := rt.PodExists(agentCaddyPodName)
	if err != nil {
		return fmt.Errorf("agent configure: check pod existence: %w", err)
	}
	if exists {
		logger.InfofCtx(ctx, "agent configure: %s already running, skipping deploy\n", agentCaddyPodName)
		return nil
	}

	// Step 3 — render pod template and deploy.
	return deployAgentCaddyPod(ctx, rt, opts.BaseDir, caddyImage)
}

// writeAgentCaddyfile renders the static Caddyfile template and writes it to
// <baseDir>/agent/caddy/Caddyfile, creating directories as needed.
func writeAgentCaddyfile(baseDir string) error {
	raw, err := assets.AgentFS.ReadFile(agentCaddyfileTmpl)
	if err != nil {
		return fmt.Errorf("agent configure: read caddyfile template: %w", err)
	}

	// The agent-caddyfile.tmpl has no template variables — write it verbatim.
	caddyDir := filepath.Join(baseDir, agentCaddyfileSubDir)
	if err := os.MkdirAll(caddyDir, dirPerm); err != nil {
		return fmt.Errorf("agent configure: create caddy dir %s: %w", caddyDir, err)
	}

	caddyfilePath := filepath.Join(caddyDir, "Caddyfile")
	if err := os.WriteFile(caddyfilePath, raw, filePerm); err != nil {
		return fmt.Errorf("agent configure: write Caddyfile to %s: %w", caddyfilePath, err)
	}

	logger.Infof("agent configure: Caddyfile written to %s\n", caddyfilePath)
	return nil
}

// deployAgentCaddyPod renders the pod template and calls DeployPodAndReadinessCheck.
func deployAgentCaddyPod(ctx context.Context, rt *podman.PodmanClient, baseDir, caddyImage string) error {
	raw, err := assets.AgentFS.ReadFile(agentCaddyTmplName)
	if err != nil {
		return fmt.Errorf("agent configure: read pod template: %w", err)
	}

	tmpl, err := template.New("agent-caddy.yaml.tmpl").Parse(string(raw))
	if err != nil {
		return fmt.Errorf("agent configure: parse pod template: %w", err)
	}

	params := map[string]any{
		"BaseDir": baseDir,
		"Values": map[string]any{
			"caddy": map[string]any{
				"image": caddyImage,
			},
		},
	}

	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, params); err != nil {
		return fmt.Errorf("agent configure: render pod template: %w", err)
	}

	// Unmarshal the rendered YAML into PodSpec for readiness checks and deploy opts.
	var podSpec podmodels.PodSpec
	if err := k8syaml.Unmarshal(rendered.Bytes(), &podSpec); err != nil {
		return fmt.Errorf("agent configure: parse rendered pod yaml: %w", err)
	}

	deployOpts := clipodman.ConstructPodDeployOptions(specs.FetchPodAnnotations(podSpec))

	logger.InfofCtx(ctx, "agent configure: deploying %s\n", agentCaddyPodName)
	if err := clipodman.DeployPodAndReadinessCheck(ctx, rt, &podSpec, "agent-caddy.yaml.tmpl",
		bytes.NewReader(rendered.Bytes()), deployOpts); err != nil {
		return fmt.Errorf("agent configure: deploy caddy pod: %w", err)
	}

	logger.InfofCtx(ctx, "agent configure: %s is ready — Worker Caddy listening on :8443\n", agentCaddyPodName)
	return nil
}

// defaultCaddyImage reads the Caddy image default from assets/agent/podman/values.yaml.
func defaultCaddyImage() (string, error) {
	raw, err := assets.AgentFS.ReadFile("agent/podman/values.yaml")
	if err != nil {
		return "", fmt.Errorf("read agent values.yaml: %w", err)
	}

	var vals struct {
		Caddy struct {
			Image string `yaml:"image"`
		} `yaml:"caddy"`
	}
	if err := k8syaml.Unmarshal(raw, &vals); err != nil {
		return "", fmt.Errorf("parse agent values.yaml: %w", err)
	}
	if vals.Caddy.Image == "" {
		return "", fmt.Errorf("caddy.image not set in agent/podman/values.yaml")
	}
	return vals.Caddy.Image, nil
}

// Package configure implements the one-time Worker-side setup that deploys the
// agent pod and the Caddy reverse-proxy pod:
//
//  1. Write the base Caddyfile to <baseDir>/agent/caddy/Caddyfile
//  2. Deploy ai-services--agent-caddy (Caddy pod)
//  3. Deploy ai-services--agent (daemon pod — agent start runs inside the container)
//
// Running 'agent configure' is the complete setup — no separate 'agent start'
// step is needed because the agent daemon is the pod container's entrypoint.
package configure

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/project-ai-services/ai-services/assets"
	"github.com/project-ai-services/ai-services/internal/pkg/catalog/cli/common/podman/caddy"
	clipodman "github.com/project-ai-services/ai-services/internal/pkg/cli/podman"
	"github.com/project-ai-services/ai-services/internal/pkg/constants"
	"github.com/project-ai-services/ai-services/internal/pkg/logger"
	podmodels "github.com/project-ai-services/ai-services/internal/pkg/models"
	"github.com/project-ai-services/ai-services/internal/pkg/proxy"
	"github.com/project-ai-services/ai-services/internal/pkg/runtime/podman"
	"github.com/project-ai-services/ai-services/internal/pkg/specs"
	"github.com/project-ai-services/ai-services/internal/pkg/utils"
	k8syaml "sigs.k8s.io/yaml"
)

const (
	// AgentPodName is the fixed name of the worker agent daemon pod.
	AgentPodName = "ai-services--agent"

	// AgentCaddyPodName is the fixed name of the worker Caddy pod.
	AgentCaddyPodName = "ai-services--agent-caddy"

	agentCaddyfileSubDir  = "agent/caddy"
	agentPodTmplName      = "agent/podman/templates/agent.yaml.tmpl"
	agentCaddyTmplName    = "agent/podman/templates/agent-caddy.yaml.tmpl"
	agentCaddyfileTmpl    = "agent/podman/templates/agent-caddyfile.tmpl"
	agentAuthSecretTmpl   = "agent/podman/templates/agent-auth-secret.yaml.tmpl"

	dirPerm  = 0o755
	filePerm = 0o644
)

// Options holds the parameters for agent configure.
type Options struct {
	// BaseDir is the root data directory on the Worker, e.g. /var/lib/ai-services.
	// Caddy data is written to <BaseDir>/agent/caddy/.
	BaseDir string
	// Runtime is the local container runtime ("podman" or "openshift").
	// Defaults to "podman". Only podman is supported for worker Caddy deployment.
	Runtime string
	// HTTPSPort is the host port Caddy listens on for external HTTPS traffic.
	// Defaults to 443.
	HTTPSPort int
	// DomainName is an optional custom domain (e.g. "example.com").
	// Priority: SSLCertPath > DomainName > workerIP.nip.io (auto-detected).
	DomainName string
	// SSLCertPath and SSLKeyPath are optional paths to a wildcard TLS cert/key.
	// When provided the domain is extracted from the certificate.
	SSLCertPath string
	SSLKeyPath  string

	// Agent container parameters — passed into the pod template so the agent
	// binary runs as a container inside the pod (no host install required).
	AgentServer      string // control-plane AgentGateway address (host:port)
	AgentName        string // name to register this worker as
	AgentToken       string // single-use bootstrap token
	PodmanSocketPath string // path to podman socket inside pod, e.g. /run/podman/podman.sock
}

// DeployAgentCaddy writes the Caddyfile and deploys both worker pods
// (ai-services--agent-caddy and ai-services--agent). Idempotent.
func DeployAgentCaddy(ctx context.Context, opts Options) error {
	if opts.Runtime == "" {
		opts.Runtime = "podman"
	}
	if opts.Runtime != "podman" {
		return fmt.Errorf("agent configure: runtime %q not supported — only podman is supported for worker Caddy deployment", opts.Runtime)
	}

	rt, err := podman.NewPodmanClient()
	if err != nil {
		return fmt.Errorf("agent configure: init podman client: %w", err)
	}

	// Read all values from values.yaml — single source of truth for images,
	// adminPort and httpsPort, matching the catalog pattern.
	vals, err := readAgentValues()
	if err != nil {
		return fmt.Errorf("agent configure: read values: %w", err)
	}
	if vals.Caddy.Image == "" {
		return fmt.Errorf("agent configure: caddy.image not set in values.yaml")
	}
	if vals.Agent.Image == "" {
		return fmt.Errorf("agent configure: agent.image not set in values.yaml")
	}
	// CLI flag overrides values.yaml; fall back to values.yaml default (443).
	if opts.HTTPSPort > 0 {
		vals.Caddy.HTTPSPort = opts.HTTPSPort
	} else if vals.Caddy.HTTPSPort == 0 {
		vals.Caddy.HTTPSPort = 443
	}
	// Resolve podman socket path — strip unix:// prefix if present.
	if opts.PodmanSocketPath == "" {
		podmanURI, err := utils.ResolvePodmanURI()
		if err != nil {
			return fmt.Errorf("agent configure: resolve podman URI: %w", err)
		}
		opts.PodmanSocketPath = strings.TrimPrefix(podmanURI, "unix://")
	}

	// Step 1 — write the Caddyfile to disk so the pod volume-mount picks it up.
	if err := writeAgentCaddyfile(opts.BaseDir); err != nil {
		return err
	}

	// Step 2 — deploy the Caddy pod first (agent pod resolves its name via DNS).
	if err := redeployPod(ctx, rt, AgentCaddyPodName, func() error {
		return deployAgentCaddyPod(ctx, rt, opts.BaseDir, vals)
	}); err != nil {
		return err
	}

	// Step 3 — verify Caddy is healthy before deploying the agent pod.
	adminURL, err := BuildAdminURL(rt)
	if err != nil {
		return fmt.Errorf("agent configure: resolve admin URL: %w", err)
	}
	pm := proxy.NewCaddyManager(adminURL, constants.AgentCaddyServerName)
	if err := pm.HealthCheck(); err != nil {
		return fmt.Errorf("agent configure: Caddy health check failed: %w", err)
	}

	// Step 4 — ensure podman-auth-secret exists so the agent pod can pull images.
	if err := createPodmanAuthSecret(ctx, rt); err != nil {
		return fmt.Errorf("agent configure: create podman auth secret: %w", err)
	}

	// Step 5 — deploy the agent daemon pod.
	if err := redeployPod(ctx, rt, AgentPodName, func() error {
		return deployAgentPod(ctx, rt, opts, vals)
	}); err != nil {
		return err
	}

	// Step 5 — compute domain suffix (used only for the log line; everything
	// else is wired via pod env vars).
	// TODO register with agent details in future: Right now using wildcard dns
	domainSuffix, err := caddy.ComputeDomainConfig(opts.SSLCertPath, opts.SSLKeyPath, opts.DomainName)
	if err != nil {
		return fmt.Errorf("agent configure: compute domain suffix: %w", err)
	}

	logger.InfofCtx(ctx, "agent configure: ready — caddy admin %s, HTTPS :%d, domain *.%s\n", adminURL, vals.Caddy.HTTPSPort, domainSuffix)
	return nil
}

// redeployPod force-removes any existing pod with name then calls deploy.
func redeployPod(ctx context.Context, rt *podman.PodmanClient, podName string, deploy func() error) error {
	exists, err := rt.PodExists(podName)
	if err != nil {
		return fmt.Errorf("agent configure: check %s existence: %w", podName, err)
	}
	if exists {
		logger.InfofCtx(ctx, "agent configure: %s exists — removing before redeploy\n", podName)
		force := true
		if err := rt.DeletePod(podName, &force); err != nil {
			return fmt.Errorf("agent configure: remove %s: %w", podName, err)
		}
	}
	return deploy()
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

// deployAgentCaddyPod renders agent-caddy.yaml.tmpl (Caddy-only) and deploys it.
func deployAgentCaddyPod(ctx context.Context, rt *podman.PodmanClient, baseDir string, vals *agentCaddyValues) error {
	return renderAndDeploy(ctx, rt, agentCaddyTmplName, "agent-caddy.yaml.tmpl", map[string]any{
		"BaseDir": baseDir,
		"Values": map[string]any{
			"caddy": map[string]any{
				"image":     vals.Caddy.Image,
				"adminPort": vals.Caddy.AdminPort,
				"httpsPort": vals.Caddy.HTTPSPort,
			},
		},
	})
}

// deployAgentPod renders agent.yaml.tmpl (daemon only) and deploys it.
func deployAgentPod(ctx context.Context, rt *podman.PodmanClient, opts Options, vals *agentCaddyValues) error {
	return renderAndDeploy(ctx, rt, agentPodTmplName, "agent.yaml.tmpl", map[string]any{
		"BaseDir":          opts.BaseDir,
		"AgentServer":      opts.AgentServer,
		"AgentName":        opts.AgentName,
		"AgentToken":       opts.AgentToken,
		"PodmanSocketPath": opts.PodmanSocketPath,
		"Values": map[string]any{
			"agent": map[string]any{
				"image": vals.Agent.Image,
			},
		},
	})
}

// createPodmanAuthSecret creates or replaces podman-auth-secret on the worker,
// using the agent-auth-secret.yaml.tmpl template — the same pattern as the catalog.
func createPodmanAuthSecret(ctx context.Context, rt *podman.PodmanClient) error {
	authFilePath, err := utils.GetAuthFilePath()
	if err != nil {
		return fmt.Errorf("resolve auth file path: %w", err)
	}

	authContent, err := os.ReadFile(authFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			logger.WarningfCtx(ctx, "agent configure: auth.json not found at %s — image pulls may fail if registry requires auth\n", authFilePath)
			authContent = []byte("{}")
		} else {
			return fmt.Errorf("read auth file %s: %w", authFilePath, err)
		}
	}

	// If the secret already exists, delete it first so the content is refreshed.
	exists, err := rt.SecretExists("podman-auth-secret")
	if err != nil {
		return fmt.Errorf("check secret existence: %w", err)
	}
	if exists {
		if err := rt.DeleteSecret("podman-auth-secret"); err != nil {
			return fmt.Errorf("delete existing secret: %w", err)
		}
	}

	raw, err := assets.AgentFS.ReadFile(agentAuthSecretTmpl)
	if err != nil {
		return fmt.Errorf("read auth secret template: %w", err)
	}

	tmpl, err := template.New("agent-auth-secret.yaml.tmpl").Parse(string(raw))
	if err != nil {
		return fmt.Errorf("parse auth secret template: %w", err)
	}

	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, map[string]any{
		"AuthFileContent": base64.StdEncoding.EncodeToString(authContent),
	}); err != nil {
		return fmt.Errorf("render auth secret template: %w", err)
	}

	if _, err := rt.CreatePod(bytes.NewReader(rendered.Bytes()), map[string]string{}); err != nil {
		return fmt.Errorf("create podman-auth-secret via kube play: %w", err)
	}

	logger.InfofCtx(ctx, "agent configure: podman-auth-secret created\n")
	return nil
}

// renderAndDeploy is a shared helper: reads tmplPath, renders it with params,
// parses the pod spec, and calls DeployPodAndReadinessCheck.
func renderAndDeploy(ctx context.Context, rt *podman.PodmanClient, tmplPath, tmplName string, params map[string]any) error {
	raw, err := assets.AgentFS.ReadFile(tmplPath)
	if err != nil {
		return fmt.Errorf("agent configure: read template %s: %w", tmplName, err)
	}

	tmpl, err := template.New(tmplName).Parse(string(raw))
	if err != nil {
		return fmt.Errorf("agent configure: parse template %s: %w", tmplName, err)
	}

	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, params); err != nil {
		return fmt.Errorf("agent configure: render %s: %w", tmplName, err)
	}

	var podSpec podmodels.PodSpec
	if err := k8syaml.Unmarshal(rendered.Bytes(), &podSpec); err != nil {
		return fmt.Errorf("agent configure: parse rendered yaml for %s: %w", tmplName, err)
	}

	deployOpts := clipodman.ConstructPodDeployOptions(specs.FetchPodAnnotations(podSpec))

	logger.InfofCtx(ctx, "agent configure: deploying %s\n", podSpec.Name)
	if err := clipodman.DeployPodAndReadinessCheck(ctx, rt, &podSpec, tmplName,
		bytes.NewReader(rendered.Bytes()), deployOpts); err != nil {
		return fmt.Errorf("agent configure: deploy %s: %w", tmplName, err)
	}

	return nil
}

// agentCaddyValues holds the fields read from assets/agent/podman/values.yaml.
type agentCaddyValues struct {
	Agent struct {
		Image string `yaml:"image"`
	} `yaml:"agent"`
	Caddy struct {
		Image     string `yaml:"image"`
		AdminPort string `yaml:"adminPort"`
		HTTPSPort int    `yaml:"httpsPort"`
	} `yaml:"caddy"`
}

// readAgentValues parses assets/agent/podman/values.yaml.
func readAgentValues() (*agentCaddyValues, error) {
	raw, err := assets.AgentFS.ReadFile("agent/podman/values.yaml")
	if err != nil {
		return nil, fmt.Errorf("read agent values.yaml: %w", err)
	}
	var vals agentCaddyValues
	if err := k8syaml.Unmarshal(raw, &vals); err != nil {
		return nil, fmt.Errorf("parse agent values.yaml: %w", err)
	}
	return &vals, nil
}

// BuildAdminURL inspects the running worker Caddy pod and returns the host-side
// admin URL (e.g. "http://localhost:37249").
func BuildAdminURL(rt *podman.PodmanClient) (string, error) {
	adminPort, err := proxy.GetCaddyAdminPort(rt, AgentCaddyPodName)
	if err != nil {
		return "", fmt.Errorf("resolve agent caddy admin port: %w", err)
	}
	return fmt.Sprintf("http://localhost:%s", adminPort), nil
}

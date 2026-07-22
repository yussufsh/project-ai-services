// Package daemon implements the worker agent daemon that runs on each LPAR.
// It connects to the control plane AgentGateway over a bidirectional gRPC stream
// and executes runtime commands on behalf of the control plane.
package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	agentpb "github.com/project-ai-services/ai-services/internal/pkg/agent/proto"
	"github.com/project-ai-services/ai-services/internal/pkg/logger"
	"github.com/project-ai-services/ai-services/internal/pkg/runtime"
	podmanRuntime "github.com/project-ai-services/ai-services/internal/pkg/runtime/podman"
	runtimetypes "github.com/project-ai-services/ai-services/internal/pkg/runtime/types"
)

const (
	heartbeatInterval  = 30 * time.Second
	reconnectBaseDelay = 5 * time.Second
	reconnectMaxDelay  = 120 * time.Second
)

// Config holds the configuration for the agent daemon.
type Config struct {
	AgentName       string
	ControlPlaneURL string // e.g. "lpar-0.example.com:9090"
	PreSharedToken  string
	Labels          map[string]string
	Capabilities    map[string]string
}

// Daemon is the worker agent daemon.
type Daemon struct {
	cfg Config
	rt  runtime.Runtime // injected at construction; determines what commands are dispatched to
}

// New creates a new Daemon with the provided config and runtime.
// The runtime is used for all command dispatches and its Type() is reported
// back to the control plane via COMMAND_TYPE_RUNTIME_TYPE.
func New(cfg Config, rt runtime.Runtime) *Daemon {
	return &Daemon{cfg: cfg, rt: rt}
}

// Run starts the daemon and blocks until ctx is cancelled.
// Registration must have been performed by the caller before Run is invoked
// (i.e. agentbootstrap.Register succeeded). Run only maintains the CommandStream,
// reconnecting with exponential backoff on disconnection.
func (d *Daemon) Run(ctx context.Context) error {
	logger.InfofCtx(ctx, "agent daemon starting: agent_name=%s control_plane=%s runtime=%s",
		d.cfg.AgentName, d.cfg.ControlPlaneURL, d.rt.Type())

	conn, err := grpc.NewClient(d.cfg.ControlPlaneURL,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("agent daemon: dial %s: %w", d.cfg.ControlPlaneURL, err)
	}
	defer conn.Close()

	client := agentpb.NewAgentGatewayClient(conn)

	// Maintain the CommandStream with reconnect loop.
	delay := reconnectBaseDelay
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := d.runStream(ctx, client); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			logger.WarningfCtx(ctx, "agent daemon: stream error (%v), reconnecting in %s", err, delay)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
			if delay < reconnectMaxDelay {
				delay *= 2
			}
			continue
		}
		delay = reconnectBaseDelay
	}
}

// runStream opens the bidirectional CommandStream, sends a heartbeat as the
// first message, then continuously processes incoming Commands.
//
// Commands are dispatched concurrently: each received Command is handled in its
// own goroutine so that slow operations (HTTP proxy, image pulls, etc.) do not
// block other in-flight commands. A dedicated send goroutine serialises all
// CommandResult writes back onto the stream, since gRPC Send is not goroutine-safe.
func (d *Daemon) runStream(ctx context.Context, client agentpb.AgentGatewayClient) error {
	stream, err := client.CommandStream(ctx)
	if err != nil {
		return fmt.Errorf("open stream: %w", err)
	}

	// Send initial heartbeat so the gateway can identify us.
	if err := stream.Send(&agentpb.CommandResult{
		AgentName:   d.cfg.AgentName,
		IsHeartbeat: true,
		Success:     true,
	}); err != nil {
		return fmt.Errorf("send initial heartbeat: %w", err)
	}

	// sendCh serialises all outbound CommandResults onto the single stream.
	// Buffer matches CommandCh so fast producers never block on the send path.
	sendCh := make(chan *agentpb.CommandResult, 64)

	// send goroutine: drains sendCh and writes to stream.
	sendErrCh := make(chan error, 1)
	go func() {
		for res := range sendCh {
			if err := stream.Send(res); err != nil {
				sendErrCh <- err
				return
			}
		}
	}()

	// Start the heartbeat goroutine — it also writes through sendCh.
	hbCtx, hbCancel := context.WithCancel(ctx)
	defer hbCancel()
	go d.heartbeatLoop(hbCtx, sendCh)

	// recv loop: dispatch each Command in its own goroutine.
	recvErrCh := make(chan error, 1)
	go func() {
		for {
			cmd, err := stream.Recv()
			if err == io.EOF {
				recvErrCh <- fmt.Errorf("stream closed by server")
				return
			}
			if err != nil {
				recvErrCh <- fmt.Errorf("recv: %w", err)
				return
			}
			go func(cmd *agentpb.Command) {
				result := d.executeCommand(ctx, cmd)
				select {
				case sendCh <- result:
				case <-ctx.Done():
				}
			}(cmd)
		}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-recvErrCh:
		return err
	case err := <-sendErrCh:
		return fmt.Errorf("send: %w", err)
	}
}

// heartbeatLoop sends a heartbeat every heartbeatInterval until ctx is done.
// Results are written to sendCh so they are serialised by the send goroutine.
func (d *Daemon) heartbeatLoop(ctx context.Context, sendCh chan<- *agentpb.CommandResult) {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			select {
			case sendCh <- &agentpb.CommandResult{
				AgentName:   d.cfg.AgentName,
				IsHeartbeat: true,
				Success:     true,
			}:
			case <-ctx.Done():
				return
			}
		}
	}
}

// executeCommand dispatches a Command to the injected runtime and returns
// the CommandResult.
func (d *Daemon) executeCommand(ctx context.Context, cmd *agentpb.Command) *agentpb.CommandResult {
	result, err := d.dispatchToRuntime(ctx, cmd)

	r := &agentpb.CommandResult{
		CommandId: cmd.GetCommandId(),
		AgentName: d.cfg.AgentName,
	}
	if err != nil {
		r.Success = false
		r.Error = err.Error()
	} else {
		r.Success = true
		r.Data = result
	}
	return r
}

// dispatchToRuntime dispatches the command to d.rt (the injected runtime).
func (d *Daemon) dispatchToRuntime(ctx context.Context, cmd *agentpb.Command) ([]byte, error) {
	rt := d.rt

	switch cmd.GetType() {
	case agentpb.CommandType_COMMAND_TYPE_LIST_IMAGES:
		result, err := rt.ListImages()
		return marshalResult(result, err)

	case agentpb.CommandType_COMMAND_TYPE_PULL_IMAGE:
		var p struct{ Image string }
		if err := json.Unmarshal(cmd.GetPayload(), &p); err != nil {
			return nil, err
		}
		return nil, rt.PullImage(p.Image)

	case agentpb.CommandType_COMMAND_TYPE_LIST_PODS:
		var p struct{ Filters map[string][]string }
		if err := json.Unmarshal(cmd.GetPayload(), &p); err != nil {
			return nil, err
		}
		result, err := rt.ListPods(p.Filters)
		return marshalResult(result, err)

	case agentpb.CommandType_COMMAND_TYPE_CREATE_POD:
		var p struct {
			Body string            `json:"body"`
			Opts map[string]string `json:"opts"`
		}
		if err := json.Unmarshal(cmd.GetPayload(), &p); err != nil {
			return nil, err
		}
		result, err := rt.CreatePod(strings.NewReader(p.Body), p.Opts)
		return marshalResult(result, err)

	case agentpb.CommandType_COMMAND_TYPE_DELETE_POD:
		var p struct {
			ID    string `json:"id"`
			Force *bool  `json:"force"`
		}
		if err := json.Unmarshal(cmd.GetPayload(), &p); err != nil {
			return nil, err
		}
		return nil, rt.DeletePod(p.ID, p.Force)

	case agentpb.CommandType_COMMAND_TYPE_STOP_POD:
		var p struct{ ID string }
		if err := json.Unmarshal(cmd.GetPayload(), &p); err != nil {
			return nil, err
		}
		return nil, rt.StopPod(p.ID)

	case agentpb.CommandType_COMMAND_TYPE_START_POD:
		var p struct{ ID string }
		if err := json.Unmarshal(cmd.GetPayload(), &p); err != nil {
			return nil, err
		}
		return nil, rt.StartPod(p.ID)

	case agentpb.CommandType_COMMAND_TYPE_INSPECT_POD:
		var p struct{ NameOrID string }
		if err := json.Unmarshal(cmd.GetPayload(), &p); err != nil {
			return nil, err
		}
		result, err := rt.InspectPod(p.NameOrID)
		return marshalResult(result, err)

	case agentpb.CommandType_COMMAND_TYPE_POD_EXISTS:
		var p struct{ NameOrID string }
		if err := json.Unmarshal(cmd.GetPayload(), &p); err != nil {
			return nil, err
		}
		result, err := rt.PodExists(p.NameOrID)
		return marshalResult(result, err)

	case agentpb.CommandType_COMMAND_TYPE_POD_LOGS:
		var p struct{ NameOrID string }
		if err := json.Unmarshal(cmd.GetPayload(), &p); err != nil {
			return nil, err
		}
		return nil, rt.PodLogs(p.NameOrID)

	case agentpb.CommandType_COMMAND_TYPE_GET_POD_RESOURCES:
		var p struct{ NameOrID string }
		if err := json.Unmarshal(cmd.GetPayload(), &p); err != nil {
			return nil, err
		}
		result, err := rt.GetPodResources(p.NameOrID)
		return marshalResult(result, err)

	case agentpb.CommandType_COMMAND_TYPE_LIST_SECRETS:
		var p struct{ Filters map[string][]string }
		if err := json.Unmarshal(cmd.GetPayload(), &p); err != nil {
			return nil, err
		}
		result, err := rt.ListSecrets(p.Filters)
		return marshalResult(result, err)

	case agentpb.CommandType_COMMAND_TYPE_DELETE_SECRET:
		var p struct{ Name string }
		if err := json.Unmarshal(cmd.GetPayload(), &p); err != nil {
			return nil, err
		}
		return nil, rt.DeleteSecret(p.Name)

	case agentpb.CommandType_COMMAND_TYPE_SECRET_EXISTS:
		var p struct{ NameOrID string }
		if err := json.Unmarshal(cmd.GetPayload(), &p); err != nil {
			return nil, err
		}
		result, err := rt.SecretExists(p.NameOrID)
		return marshalResult(result, err)

	case agentpb.CommandType_COMMAND_TYPE_DELETE_VOLUME:
		var p struct{ Name string }
		if err := json.Unmarshal(cmd.GetPayload(), &p); err != nil {
			return nil, err
		}
		return nil, rt.DeleteVolume(p.Name)

	case agentpb.CommandType_COMMAND_TYPE_VOLUME_EXISTS:
		var p struct{ NameOrID string }
		if err := json.Unmarshal(cmd.GetPayload(), &p); err != nil {
			return nil, err
		}
		result, err := rt.VolumeExists(p.NameOrID)
		return marshalResult(result, err)

	case agentpb.CommandType_COMMAND_TYPE_INSPECT_CONTAINER:
		var p struct{ NameOrID string }
		if err := json.Unmarshal(cmd.GetPayload(), &p); err != nil {
			return nil, err
		}
		result, err := rt.InspectContainer(p.NameOrID)
		return marshalResult(result, err)

	case agentpb.CommandType_COMMAND_TYPE_CONTAINER_EXISTS:
		var p struct{ NameOrID string }
		if err := json.Unmarshal(cmd.GetPayload(), &p); err != nil {
			return nil, err
		}
		result, err := rt.ContainerExists(p.NameOrID)
		return marshalResult(result, err)

	case agentpb.CommandType_COMMAND_TYPE_CONTAINER_LOGS:
		var p struct{ ContainerNameOrID string }
		if err := json.Unmarshal(cmd.GetPayload(), &p); err != nil {
			return nil, err
		}
		return nil, rt.ContainerLogs(p.ContainerNameOrID)

	case agentpb.CommandType_COMMAND_TYPE_LIST_ROUTES:
		result, err := rt.ListRoutes()
		return marshalResult(result, err)

	case agentpb.CommandType_COMMAND_TYPE_DELETE_PVCS:
		var p struct{ AppLabel string }
		if err := json.Unmarshal(cmd.GetPayload(), &p); err != nil {
			return nil, err
		}
		return nil, rt.DeletePVCs(p.AppLabel)

	case agentpb.CommandType_COMMAND_TYPE_GET_SYSTEM_INFO:
		result, err := rt.GetSystemInfo()
		return marshalResult(result, err)

	case agentpb.CommandType_COMMAND_TYPE_RUNTIME_TYPE:
		// Report the actual runtime type so the control plane can route
		// to the correct deployer (PodmanDeployer, OpenShiftDeployer, etc.).
		return marshalResult(string(rt.Type()), nil)

	case agentpb.CommandType_COMMAND_TYPE_HTTP_PROXY:
		var p struct {
			Method  string            `json:"method"`
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers,omitempty"`
			Body    []byte            `json:"body,omitempty"`
		}
		if err := json.Unmarshal(cmd.GetPayload(), &p); err != nil {
			return nil, err
		}
		// The URL contains the pod name as host (e.g. http://digitize-abc123:4001/path).
		// Pod names are only resolvable inside the Podman bridge network via the
		// dnsname plugin — the daemon process runs on the host and cannot use that
		// resolver. We use the Podman socket (already available as rt) to inspect
		// the pod's infra container and get its bridge IP, then rewrite the URL.
		targetURL, err := d.resolvePodURL(ctx, p.URL)
		if err != nil {
			return nil, err
		}
		var reqBody io.Reader
		if len(p.Body) > 0 {
			reqBody = bytes.NewReader(p.Body)
		}
		httpReq, err := http.NewRequestWithContext(ctx, p.Method, targetURL, reqBody)
		if err != nil {
			return nil, fmt.Errorf("http proxy: build request: %w", err)
		}
		for k, v := range p.Headers {
			httpReq.Header.Set(k, v)
		}
		httpResp, err := http.DefaultClient.Do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("http proxy: do request: %w", err)
		}
		defer httpResp.Body.Close()
		respBody, err := io.ReadAll(httpResp.Body)
		if err != nil {
			return nil, fmt.Errorf("http proxy: read response body: %w", err)
		}
		// Flatten response headers.
		respHeaders := make(map[string]string)
		for k, vals := range httpResp.Header {
			if len(vals) > 0 {
				respHeaders[k] = vals[0]
			}
		}
		type proxyResp struct {
			StatusCode int               `json:"status_code"`
			Headers    map[string]string `json:"headers,omitempty"`
			Body       []byte            `json:"body"`
		}
		return marshalResult(proxyResp{
			StatusCode: httpResp.StatusCode,
			Headers:    respHeaders,
			Body:       respBody,
		}, nil)

	case agentpb.CommandType_COMMAND_TYPE_RUN_EPHEMERAL_CONTAINER:
		var p struct {
			Image  string                 `json:"image"`
			Cmd    []string               `json:"cmd"`
			Mounts []runtimetypes.BindMount `json:"mounts"`
		}
		if err := json.Unmarshal(cmd.GetPayload(), &p); err != nil {
			return nil, err
		}
		exitCode, err := rt.RunEphemeralContainer(p.Image, p.Cmd, p.Mounts)
		return marshalResult(exitCode, err)

	default:
		return nil, fmt.Errorf("unknown command type: %v", cmd.GetType())
	}
}

func marshalResult(v any, err error) ([]byte, error) {
	if err != nil {
		return nil, err
	}
	return json.Marshal(v)
}

// resolvePodURL rewrites a URL whose host is a Podman pod name (e.g.
// "http://digitize-abc123:4001/path") to use the pod's bridge IP instead.
// Pod names are resolved via the Podman socket so no external DNS is needed.
// If the runtime is not a PodmanClient (e.g. in tests), the original URL is
// returned unchanged.
func (d *Daemon) resolvePodURL(ctx context.Context, rawURL string) (string, error) {
	parsed, err := neturl.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("http proxy: parse URL %q: %w", rawURL, err)
	}

	pc, ok := d.rt.(*podmanRuntime.PodmanClient)
	if !ok {
		// Not a Podman runtime (unit-test stub etc.) — use the URL as-is.
		return rawURL, nil
	}

	podName := parsed.Hostname()
	ip, err := pc.GetPodIP(podName)
	if err != nil {
		return "", fmt.Errorf("http proxy: resolve IP for pod %q: %w", podName, err)
	}

	// Rebuild the URL with the IP in place of the pod name.
	port := parsed.Port()
	if port != "" {
		parsed.Host = ip + ":" + port
	} else {
		parsed.Host = ip
	}
	return parsed.String(), nil
}

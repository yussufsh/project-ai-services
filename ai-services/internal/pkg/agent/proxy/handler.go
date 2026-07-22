// Package proxy provides an HTTP handler that tunnels incoming requests to a
// remote worker agent over the existing gRPC CommandStream.
//
// Traffic flow:
//
//	Browser → Caddy (:443) → API Server /agent-proxy/:agent_name/:pod_name/:port/* → AgentHTTPHandler → gRPC stream → agent daemon → pod (localhost)
//
// Caddy registers the upstream as catalog-api:<port>/agent-proxy/<agent_name>/<pod_name>/<container_port>
// instead of <pod_name>:<port> when the deployment is remote.  The rest of the
// route annotation format and registration logic is unchanged.
package proxy

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/project-ai-services/ai-services/internal/pkg/agent/registry"
	"github.com/project-ai-services/ai-services/internal/pkg/runtime/remote"
)

// AgentHTTPHandler handles requests forwarded by Caddy for remote-deployed services.
// It is registered on the API Server's Gin router at:
//
//	GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS /agent-proxy/:agent_name/:pod_name/:port/*path
type AgentHTTPHandler struct {
	registry *registry.Registry
}

// New creates a new AgentHTTPHandler backed by the given registry.
func New(reg *registry.Registry) *AgentHTTPHandler {
	return &AgentHTTPHandler{registry: reg}
}

// RegisterRoutes mounts the proxy handler on the provided RouterGroup.
// No auth middleware is applied here — the route is only reachable via Caddy,
// which already enforces TLS and (optionally) mutual auth.  Operators who want
// an extra auth layer can add middleware at the call site.
func (h *AgentHTTPHandler) RegisterRoutes(rg *gin.RouterGroup) {
	methods := []string{
		http.MethodGet, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions,
	}
	for _, m := range methods {
		rg.Handle(m, "/:agent_name/:pod_name/:port/*path", h.handle)
	}
}

// handle is the Gin handler for all proxied requests.
func (h *AgentHTTPHandler) handle(c *gin.Context) {
	agentName := c.Param("agent_name")
	podName := c.Param("pod_name")
	port := c.Param("port")

	entry, ok := h.registry.Get(agentName)
	if !ok {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("agent %q not found or not connected", agentName)})
		return
	}
	if entry.Status != registry.AgentStatusReady && entry.Status != registry.AgentStatusBusy {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("agent %q is not ready (status=%s)", agentName, entry.Status)})
		return
	}

	rt := remote.New(agentName, h.registry)
	if err := rt.ProxyHTTP(c.Request.Context(), c.Writer, c.Request, podName, port); err != nil {
		// Only write an error response if headers haven't been sent yet.
		if !c.Writer.Written() {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		}
	}
}

// UpstreamFor returns the Caddy upstream string for a remote service pod/port.
// Instead of "pod-name:port" (unreachable from the control plane), Caddy will
// dial the API Server itself at catalog-api:<apiPort> and the API Server tunnels
// through to the agent.
//
// The path prefix /agent-proxy/<agentName>/<podName>/<port> is stripped by Caddy's
// reverse_proxy handler automatically because Caddy's route match is host-only.
func UpstreamFor(apiServerPodName, apiPort, agentName, podName, port string) string {
	return fmt.Sprintf("%s:%s", apiServerPodName, apiPort)
}

// PathPrefixFor returns the path prefix that must be prepended to every request
// Caddy forwards to the API Server for this remote pod/port.
func PathPrefixFor(agentName, podName, port string) string {
	return fmt.Sprintf("/agent-proxy/%s/%s/%s", agentName, podName, port)
}

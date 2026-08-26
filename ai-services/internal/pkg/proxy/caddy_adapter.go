package proxy

import (
	"context"
	"fmt"

	runtimetypes "github.com/project-ai-services/ai-services/internal/pkg/runtime/types"
)

// LocalCaddyManagerAdapter wraps a ProxyManager to satisfy the
// podman.LocalCaddyManager interface. It lives in the proxy package so callers
// (worker join command) can construct it without the podman package needing to
// import proxy directly (which would create an import cycle).
type LocalCaddyManagerAdapter struct {
	pm ProxyManager
}

// NewLocalCaddyManagerAdapter creates an adapter from an existing ProxyManager.
func NewLocalCaddyManagerAdapter(pm ProxyManager) *LocalCaddyManagerAdapter {
	return &LocalCaddyManagerAdapter{pm: pm}
}

// NewLocalCaddyManagerFromEnv creates an adapter using the CADDY_ADMIN_URL env var.
func NewLocalCaddyManagerFromEnv() (*LocalCaddyManagerAdapter, error) {
	pm, err := GetCaddyProxyManager()
	if err != nil {
		return nil, fmt.Errorf("local caddy manager: %w", err)
	}

	return NewLocalCaddyManagerAdapter(pm), nil
}

// RegisterRoute implements podman.LocalCaddyManager.
func (a *LocalCaddyManagerAdapter) RegisterRoute(ctx context.Context, route runtimetypes.ProxyRoute) error {
	return a.pm.RegisterRoute(ctx, Route{
		ID:       route.ID,
		Domain:   route.Domain,
		Upstream: route.Upstream,
		Terminal: route.Terminal,
		Type:     route.Type,
	})
}

// UnregisterRoute implements podman.LocalCaddyManager.
func (a *LocalCaddyManagerAdapter) UnregisterRoute(ctx context.Context, routeID string) error {
	return a.pm.UnregisterRoute(ctx, routeID)
}

// GetRoute implements podman.LocalCaddyManager.
func (a *LocalCaddyManagerAdapter) GetRoute(ctx context.Context, routeID string) (*runtimetypes.ProxyRoute, error) {
	r, err := a.pm.GetRouteByID(ctx, routeID)
	if err != nil {
		return nil, err
	}

	return &runtimetypes.ProxyRoute{
		ID:       r.ID,
		Domain:   r.Domain,
		Upstream: r.Upstream,
		Terminal: r.Terminal,
		Type:     r.Type,
	}, nil
}

// HealthCheck implements podman.LocalCaddyManager.
func (a *LocalCaddyManagerAdapter) HealthCheck(ctx context.Context) error {
	return a.pm.HealthCheck(ctx)
}

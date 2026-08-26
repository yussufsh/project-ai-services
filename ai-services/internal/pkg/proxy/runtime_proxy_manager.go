package proxy

import (
	"context"
	"fmt"

	runtimetypes "github.com/project-ai-services/ai-services/internal/pkg/runtime/types"
)

// RuntimeProxyInterface is the subset of runtime.Runtime needed to manage
// Caddy routes on a remote worker. Using a narrow interface rather than the
// full runtime.Runtime breaks the import cycle between the proxy and runtime
// packages.
type RuntimeProxyInterface interface {
	RegisterProxyRoute(ctx context.Context, route runtimetypes.ProxyRoute) error
	UnregisterProxyRoute(ctx context.Context, routeID string) error
	GetProxyRoute(ctx context.Context, routeID string) (*runtimetypes.ProxyRoute, error)
	ProxyHealthCheck(ctx context.Context) error
}

// runtimeProxyManager implements ProxyManager by delegating to a
// RuntimeProxyInterface. For a RemoteRuntime every call is dispatched over the
// gRPC CommandStream so the worker's local Caddy is managed remotely.
type runtimeProxyManager struct {
	rt RuntimeProxyInterface
}

// NewRuntimeProxyManager wraps a RuntimeProxyInterface as a ProxyManager.
func NewRuntimeProxyManager(rt RuntimeProxyInterface) ProxyManager {
	return &runtimeProxyManager{rt: rt}
}

func (r *runtimeProxyManager) HealthCheck(ctx context.Context) error {
	return r.rt.ProxyHealthCheck(ctx)
}

func (r *runtimeProxyManager) RegisterRoute(ctx context.Context, route Route) error {
	return r.rt.RegisterProxyRoute(ctx, runtimetypes.ProxyRoute{
		ID:       route.ID,
		Domain:   route.Domain,
		Upstream: route.Upstream,
		Terminal: route.Terminal,
		Type:     route.Type,
	})
}

func (r *runtimeProxyManager) UnregisterRoute(ctx context.Context, routeID string) error {
	if routeID == "" {
		return fmt.Errorf("route ID cannot be empty")
	}

	return r.rt.UnregisterProxyRoute(ctx, routeID)
}

func (r *runtimeProxyManager) GetRouteByID(ctx context.Context, routeID string) (*Route, error) {
	pr, err := r.rt.GetProxyRoute(ctx, routeID)
	if err != nil {
		return nil, err
	}

	return &Route{
		ID:       pr.ID,
		Domain:   pr.Domain,
		Upstream: pr.Upstream,
		Terminal: pr.Terminal,
		Type:     pr.Type,
	}, nil
}

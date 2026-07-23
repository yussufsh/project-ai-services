package proxy

import "context"

// ProxyManager defines the interface for managing reverse proxy routes.
type ProxyManager interface {
	// RegisterRoute registers a new route with the proxy
	RegisterRoute(ctx context.Context, route Route) error

	// UnregisterRoute removes a route from the proxy by its ID
	UnregisterRoute(routeID string) error

	// HealthCheck verifies the proxy is available and responding
	HealthCheck() error

	// GetRouteByID retrieves a specific route by its ID from the proxy
	GetRouteByID(routeID string) (*Route, error)
}

// Route represents a reverse proxy route configuration.
type Route struct {
	// ID is the unique identifier for the route
	ID string

	// Domain is the hostname to match (e.g., "service.example.com")
	Domain string

	// Upstream is the backend service address (e.g., "pod-name:8080")
	Upstream string

	// Terminal indicates if route matching should stop after this route
	Terminal bool

	// Type indicates the endpoint type
	Type string

	// TLSTransport instructs the reverse proxy to dial the upstream over TLS
	// with certificate verification disabled. Used when the upstream is a
	// Worker Caddy instance that uses a self-signed internal CA cert.
	TLSTransport bool
}

// Made with Bob

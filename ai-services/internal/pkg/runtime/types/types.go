package types

import "time"

// RuntimeType represents the type of container runtime.
type RuntimeType string

const (
	RuntimeTypePodman    RuntimeType = "podman"
	RuntimeTypeOpenShift RuntimeType = "openshift"
)

// String returns the string representation of RuntimeType.
func (r RuntimeType) String() string {
	return string(r)
}

// Valid checks if the runtime type is valid.
func (r RuntimeType) Valid() bool {
	switch r {
	case RuntimeTypePodman, RuntimeTypeOpenShift:
		return true
	default:
		return false
	}
}

type Pod struct {
	ID               string
	Name             string
	Status           string
	Health           string
	Labels           map[string]string
	Containers       []Container
	Created          time.Time
	Ports            map[string][]string
	State            string
	InfraContainerID string
}

type Container struct {
	ID                     string `json:"ID"`
	Name                   string
	Status                 string
	Health                 string
	Annotations            map[string]string
	Env                    map[string]string
	HealthcheckStartPeriod time.Duration
}

type Image struct {
	RepoTags    []string
	RepoDigests []string
}

type Route struct {
	Name       string
	HostPort   string
	TargetPort string
	Labels     map[string]string
}

// PodResources represents resource allocation and usage for a pod including accelerators.
type PodResources struct {
	CPU        float64  // CPU usage (e.g., 1.5 CPUs)
	MemUsage   uint64   // Memory usage in bytes
	SpyreCards []string // List of Spyre card PCI addresses
}

// CRDResource represents a custom resource in openshift.
type CRDResource struct {
	Name   string
	Labels map[string]string
}

// HTTPProxyResponse carries the result of an HTTP request executed on the
// worker node and returned to the control plane via the gRPC stream.
type HTTPProxyResponse struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
}

// ProxyRoute represents a Caddy reverse-proxy route on a worker node.
// Kept in runtime/types so the Runtime interface can reference it without
// importing the proxy package (which would cause an import cycle).
type ProxyRoute struct {
	ID       string // unique route identifier used as @id in Caddy config
	Domain   string // hostname to match (e.g. "service.example.com")
	Upstream string // backend address (e.g. "10.88.0.5:8080")
	Terminal bool   // stop route matching after this route
	Type     string // endpoint type label (e.g. "ui", "api")
}

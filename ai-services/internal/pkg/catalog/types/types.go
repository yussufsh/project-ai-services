package types

import "time"

// Architecture represents a complete AI solution template
type Architecture struct {
	ID                string             `yaml:"id" json:"id"`
	Name              string             `yaml:"name" json:"name"`
	Description       string             `yaml:"description" json:"description"`
	Version           string             `yaml:"version" json:"version"`
	Type              string             `yaml:"type" json:"type"` // "architecture"
	CertifiedBy       string             `yaml:"certified_by" json:"certified_by"`
	SupportedRuntimes []string           `yaml:"supported_runtimes" json:"supported_runtimes"`
	Services          []ServiceReference `yaml:"services" json:"services"`
	DemoLink          string             `yaml:"demo_link,omitempty" json:"demo_link,omitempty"`
	CodeLink          string             `yaml:"code_link,omitempty" json:"code_link,omitempty"`
	DocumentationLink string             `yaml:"documentation_link,omitempty" json:"documentation_link,omitempty"`
}

// ServiceReference represents a reference to a service in an architecture
type ServiceReference struct {
	ID       string `yaml:"id" json:"id"`
	Version  string `yaml:"version,omitempty" json:"version,omitempty"`
	Optional bool   `yaml:"optional,omitempty" json:"optional,omitempty"`
}

// DependencyReference represents a reference to a dependency service
type DependencyReference struct {
	ID      string `yaml:"id" json:"id"`
	Version string `yaml:"version,omitempty" json:"version,omitempty"`
}

// Service represents a deployable AI service
type Service struct {
	ID                string                `yaml:"id" json:"id"`
	Name              string                `yaml:"name" json:"name"`
	Description       string                `yaml:"description" json:"description"`
	Version           string                `yaml:"version" json:"version"`
	Type              string                `yaml:"type" json:"type"` // "service"
	CertifiedBy       string                `yaml:"certified_by" json:"certified_by"`
	DependencyOnly    bool                  `yaml:"dependency_only,omitempty" json:"dependency_only,omitempty"` // If true, service can only be used as a dependency
	SupportedRuntimes []string              `yaml:"supported_runtimes,omitempty" json:"supported_runtimes,omitempty"`
	Architectures     []string              `yaml:"architectures" json:"architectures"`
	Dependencies      []DependencyReference `yaml:"dependencies" json:"dependencies"`
	Endpoints         []ServiceEndpoint     `yaml:"endpoints,omitempty" json:"endpoints,omitempty"`
}

// ServiceEndpoint defines an endpoint provided by a service
type ServiceEndpoint struct {
	Name        string `yaml:"name" json:"name"`
	Type        string `yaml:"type" json:"type"`
	Port        int    `yaml:"port" json:"port"`
	Path        string `yaml:"path,omitempty" json:"path,omitempty"`
	Description string `yaml:"description" json:"description"`
	HealthCheck string `yaml:"health_check,omitempty" json:"health_check,omitempty"`
}

// RuntimeMetadata contains runtime-specific metadata
type RuntimeMetadata struct {
	PodTemplates         []PodTemplateConfig   `yaml:"pod_templates,omitempty" json:"pod_templates,omitempty"`
	ResourceRequirements *ResourceRequirements `yaml:"resource_requirements,omitempty" json:"resource_requirements,omitempty"`
}

// ResourceRequirements defines resource requirements for a runtime
type ResourceRequirements struct {
	MinCPU    string `yaml:"min_cpu,omitempty" json:"min_cpu,omitempty"`
	MinMemory string `yaml:"min_memory,omitempty" json:"min_memory,omitempty"`
	MinSpyre  int    `yaml:"min_spyre,omitempty" json:"min_spyre,omitempty"` // Number of Spyre cards required
	MinDisk   string `yaml:"min_disk,omitempty" json:"min_disk,omitempty"`
}

// PodTemplateConfig defines pod template configuration
type PodTemplateConfig struct {
	Name             string        `yaml:"name" json:"name"`
	Template         string        `yaml:"template" json:"template"`
	WaitForReady     bool          `yaml:"wait_for_ready,omitempty" json:"wait_for_ready,omitempty"`
	ReadinessTimeout time.Duration `yaml:"readiness_timeout,omitempty" json:"readiness_timeout,omitempty"`
}

// Made with Bob

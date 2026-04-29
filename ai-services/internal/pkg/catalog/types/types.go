package types

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
	ID          string `yaml:"id" json:"id"`
	Version     string `yaml:"version,omitempty" json:"version,omitempty"`
	Optional    bool   `yaml:"optional,omitempty" json:"optional,omitempty"`
	InstructCPU bool   `yaml:"instruct-cpu,omitempty" json:"instruct-cpu,omitempty"` // If true, use instruct-cpu instead of instruct
	RerankerCPU bool   `yaml:"reranker-cpu,omitempty" json:"reranker-cpu,omitempty"` // If true, use reranker-cpu instead of reranker
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
	Dependencies      []DependencyReference `yaml:"dependencies,omitempty" json:"dependencies,omitempty"` // Optional: omitted when dependency_only is true
	RuntimeMetadata   *RuntimeMetadata      `yaml:"-" json:"-"`                                           // Runtime-specific metadata (not in YAML, populated at runtime)
}

// ServiceSummary represents a service without endpoints and pod_templates for API responses
type ServiceSummary struct {
	ID                string                `json:"id"`
	Name              string                `json:"name"`
	Description       string                `json:"description"`
	Version           string                `json:"version"`
	Type              string                `json:"type"`
	CertifiedBy       string                `json:"certified_by"`
	DependencyOnly    bool                  `json:"dependency_only,omitempty"`
	SupportedRuntimes []string              `json:"supported_runtimes,omitempty"`
	Architectures     []string              `json:"architectures"`
	Dependencies      []DependencyReference `json:"dependencies,omitempty"`
}

// RuntimeMetadata contains runtime-specific metadata
type RuntimeMetadata struct {
	Name    string `yaml:"name,omitempty" json:"name,omitempty"`
	Version string `yaml:"version,omitempty" json:"version,omitempty"`
}

// Made with Bob

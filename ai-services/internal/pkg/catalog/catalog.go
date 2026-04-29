package catalog

import (
	"fmt"

	"github.com/project-ai-services/ai-services/assets"
	"github.com/project-ai-services/ai-services/internal/pkg/catalog/types"
	"github.com/project-ai-services/ai-services/internal/pkg/cli/templates"
	runtimeTypes "github.com/project-ai-services/ai-services/internal/pkg/runtime/types"
	"github.com/project-ai-services/ai-services/internal/pkg/vars"
)

// getRuntime returns the current runtime type as a string
func getRuntime() string {
	return vars.RuntimeFactory.GetRuntimeType().String()
}

// ============================================================================
// Architecture Functions
// ============================================================================

// LoadArchitecture loads an architecture by ID from the catalog
func LoadArchitecture(id string) (*types.Architecture, error) {
	tp := templates.NewEmbedTemplateProvider(&assets.CatalogFS, "architectures")

	var arch types.Architecture
	if err := tp.LoadMetadata(id, false, &arch); err != nil {
		return nil, fmt.Errorf("failed to load architecture '%s': %w", id, err)
	}

	return &arch, nil
}

// ListArchitectures lists all available architectures in the catalog
func ListArchitectures() ([]types.Architecture, error) {
	entries, err := assets.CatalogFS.ReadDir("architectures")
	if err != nil {
		return nil, fmt.Errorf("failed to list architectures: %w", err)
	}

	var architectures []types.Architecture
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		arch, err := LoadArchitecture(entry.Name())
		if err != nil {
			// Skip architectures that fail to load
			continue
		}
		architectures = append(architectures, *arch)
	}

	return architectures, nil
}

// ============================================================================
// Service Functions
// ============================================================================

// LoadService loads base service metadata by ID (without runtime-specific data)
func LoadService(id string) (*types.Service, error) {
	tp := templates.NewEmbedTemplateProvider(&assets.CatalogFS, "services")

	var service types.Service
	if err := tp.LoadMetadata(id, false, &service); err != nil {
		return nil, fmt.Errorf("failed to load service '%s': %w", id, err)
	}

	return &service, nil
}

// LoadServiceRuntimeMetadata loads runtime-specific metadata for a service using the current runtime
func LoadServiceRuntimeMetadata(serviceID string) (*types.RuntimeMetadata, error) {
	tp := templates.NewEmbedTemplateProvider(&assets.CatalogFS, "services")

	var runtimeMeta types.RuntimeMetadata
	if err := tp.LoadMetadata(serviceID, true, &runtimeMeta); err != nil {
		return nil, fmt.Errorf("failed to load runtime metadata for service '%s' runtime '%s': %w", serviceID, getRuntime(), err)
	}

	return &runtimeMeta, nil
}

// LoadServiceValues loads values.yaml for a specific service and runtime
func LoadServiceValues(serviceID string) (map[string]interface{}, error) {
	tp := templates.NewEmbedTemplateProvider(&assets.CatalogFS, "services")

	values, err := tp.LoadValues(serviceID, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to load values for service '%s' runtime '%s': %w", serviceID, getRuntime(), err)
	}

	return values, nil
}

// LoadServiceWithRuntime loads a service by ID and merges it with runtime-specific metadata
func LoadServiceWithRuntime(id string, runtime runtimeTypes.RuntimeType) (*types.Service, error) {
	// Load base service metadata
	service, err := LoadService(id)
	if err != nil {
		return nil, err
	}

	// Load runtime-specific metadata
	runtimeMeta, err := LoadServiceRuntimeMetadata(id)
	if err != nil {
		// Runtime metadata is optional, return base service if not found
		return service, nil
	}

	// Merge runtime metadata into service
	service.RuntimeMetadata = runtimeMeta
	if runtimeMeta.Version != "" {
		service.Version = runtimeMeta.Version
	}

	return service, nil
}

// ListServices lists all deployable services (excludes dependency-only services)
func ListServices() ([]types.Service, error) {
	entries, err := assets.CatalogFS.ReadDir("services")
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	var services []types.Service
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		service, err := LoadService(entry.Name())
		if err != nil {
			// Skip services that fail to load
			continue
		}

		// Only include deployable services (not dependency-only)
		if !service.DependencyOnly {
			services = append(services, *service)
		}
	}

	return services, nil
}

// ListServicesWithRuntime lists all deployable services with runtime-specific metadata
func ListServicesWithRuntime(runtime runtimeTypes.RuntimeType) ([]types.Service, error) {
	// Get base list of services
	baseServices, err := ListServices()
	if err != nil {
		return nil, err
	}

	// Enhance each service with runtime metadata
	var services []types.Service
	for _, baseSvc := range baseServices {
		service, err := LoadServiceWithRuntime(baseSvc.ID, runtime)
		if err != nil {
			// Skip services that fail to load with runtime metadata
			continue
		}
		services = append(services, *service)
	}

	return services, nil
}

// ToServiceSummary converts a Service to ServiceSummary (excludes endpoints and templates)
func ToServiceSummary(service *types.Service) types.ServiceSummary {
	summary := types.ServiceSummary{
		ID:                service.ID,
		Name:              service.Name,
		Description:       service.Description,
		Version:           service.Version,
		Type:              service.Type,
		CertifiedBy:       service.CertifiedBy,
		DependencyOnly:    service.DependencyOnly,
		SupportedRuntimes: service.SupportedRuntimes,
		Architectures:     service.Architectures,
		Dependencies:      service.Dependencies,
	}

	return summary
}

// ============================================================================
// Template Resolution Functions
// ============================================================================

// ResolveTemplateToServices resolves a template (architecture or service) to a list of service IDs
// Returns all services including dependencies in dependency order
func ResolveTemplateToServices(template string) ([]string, error) {
	visited := make(map[string]bool)
	var result []string

	// Try architecture first
	if arch, archErr := LoadArchitecture(template); archErr == nil {
		// Architecture mode - resolve all services in the architecture
		for _, svc := range arch.Services {
			if err := resolveDependenciesRecursive(svc.ID, svc.InstructCPU, svc.RerankerCPU, visited, &result); err != nil {
				return nil, err
			}
		}
		return result, nil
	}

	// Try service next
	if _, svcErr := LoadService(template); svcErr == nil {
		// Service mode - resolve this service and its dependencies
		if err := resolveDependenciesRecursive(template, false, false, visited, &result); err != nil {
			return nil, err
		}
		return result, nil
	}

	return nil, fmt.Errorf("template '%s' is not a valid architecture or service", template)
}

// resolveDependenciesRecursive performs depth-first traversal to resolve service dependencies
// instructCPU and rerankerCPU flags control whether to use CPU variants of those services
func resolveDependenciesRecursive(serviceID string, instructCPU bool, rerankerCPU bool, visited map[string]bool, result *[]string) error {
	// Check for circular dependencies
	if visited[serviceID] {
		return nil
	}

	// Load service metadata
	service, err := LoadService(serviceID)
	if err != nil {
		return fmt.Errorf("failed to load service '%s': %w", serviceID, err)
	}

	// Mark as visited
	visited[serviceID] = true

	// Recursively resolve all dependencies (all are required)
	for _, dep := range service.Dependencies {
		depID := dep.ID
		// Apply CPU suffix based on service-specific flags
		if (instructCPU && depID == "instruct") || (rerankerCPU && depID == "reranker") {
			depID = depID + "-cpu"
		}
		if err := resolveDependenciesRecursive(depID, instructCPU, rerankerCPU, visited, result); err != nil {
			return err
		}
	}

	// Add current service to result
	*result = append(*result, serviceID)

	return nil
}

// ============================================================================
// Deployment Order Functions
// ============================================================================

// GetDeploymentOrder groups services into deployment layers using topological sort
// Services in the same layer have no dependencies on each other and can be deployed in parallel
func GetDeploymentOrder(serviceIDs []string) ([][]string, error) {
	// Build dependency graph
	graph := make(map[string][]string)
	inDegree := make(map[string]int)

	// Initialize all services
	for _, svcID := range serviceIDs {
		if _, exists := graph[svcID]; !exists {
			graph[svcID] = []string{}
			inDegree[svcID] = 0
		}
	}

	// Helper function to check if a CPU variant exists in the service list
	hasCPUVariant := func(baseName string) (string, bool) {
		cpuName := baseName + "-cpu"
		// Only check for known CPU variants
		if (baseName == "instruct" || baseName == "reranker") && graph[cpuName] != nil {
			return cpuName, true
		}
		return "", false
	}

	// Build edges (dependencies)
	for _, svcID := range serviceIDs {
		service, err := LoadService(svcID)
		if err != nil {
			return nil, fmt.Errorf("failed to load service '%s': %w", svcID, err)
		}

		for _, dep := range service.Dependencies {
			depID := dep.ID

			// Check if this dependency has a CPU variant in the service list
			if cpuVariant, exists := hasCPUVariant(depID); exists {
				depID = cpuVariant
			}

			// Only add edge if dependency is in our service list
			if _, exists := graph[depID]; exists {
				graph[depID] = append(graph[depID], svcID)
				inDegree[svcID]++
			}
		}
	}

	// Topological sort using Kahn's algorithm
	var layers [][]string
	queue := []string{}

	// Find all services with no dependencies (in-degree = 0)
	for svcID, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, svcID)
		}
	}

	for len(queue) > 0 {
		// Current layer contains all services with no remaining dependencies
		currentLayer := make([]string, len(queue))
		copy(currentLayer, queue)
		layers = append(layers, currentLayer)

		// Process current layer
		nextQueue := []string{}
		for _, svcID := range queue {
			// Reduce in-degree for all dependent services
			for _, dependent := range graph[svcID] {
				inDegree[dependent]--
				if inDegree[dependent] == 0 {
					nextQueue = append(nextQueue, dependent)
				}
			}
		}
		queue = nextQueue
	}

	// Verify all services were processed (detect circular dependencies)
	processedCount := 0
	for _, layer := range layers {
		processedCount += len(layer)
	}
	if processedCount != len(serviceIDs) {
		return nil, fmt.Errorf("circular dependency detected in services")
	}

	return layers, nil
}

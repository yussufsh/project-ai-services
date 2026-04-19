package catalog

import (
	"fmt"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/project-ai-services/ai-services/assets"
	"github.com/project-ai-services/ai-services/internal/pkg/catalog/types"
	"github.com/project-ai-services/ai-services/internal/pkg/cli/templates"
	"github.com/project-ai-services/ai-services/internal/pkg/models"
	runtimeTypes "github.com/project-ai-services/ai-services/internal/pkg/runtime/types"
)

// LoadArchitecture loads an architecture by ID
func LoadArchitecture(id string) (*types.Architecture, error) {
	metadataPath := filepath.Join("architectures", id, "metadata.yaml")

	data, err := assets.CatalogFS.ReadFile(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read architecture metadata for '%s': %w", id, err)
	}

	var arch types.Architecture
	if err := yaml.Unmarshal(data, &arch); err != nil {
		return nil, fmt.Errorf("failed to parse architecture metadata for '%s': %w", id, err)
	}

	return &arch, nil
}

// LoadService loads a service by ID
func LoadService(id string) (*types.Service, error) {
	metadataPath := filepath.Join("services", id, "metadata.yaml")

	data, err := assets.CatalogFS.ReadFile(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read service metadata for '%s': %w", id, err)
	}

	var service types.Service
	if err := yaml.Unmarshal(data, &service); err != nil {
		return nil, fmt.Errorf("failed to parse service metadata for '%s': %w", id, err)
	}

	return &service, nil
}

// LoadServiceRuntimeMetadata loads runtime-specific metadata for a service
func LoadServiceRuntimeMetadata(serviceID, runtime string) (*types.RuntimeMetadata, error) {
	metadataPath := filepath.Join("services", serviceID, runtime, "metadata.yaml")

	data, err := assets.CatalogFS.ReadFile(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read runtime metadata for service '%s' runtime '%s': %w", serviceID, runtime, err)
	}

	var runtimeMeta types.RuntimeMetadata
	if err := yaml.Unmarshal(data, &runtimeMeta); err != nil {
		return nil, fmt.Errorf("failed to parse runtime metadata for service '%s' runtime '%s': %w", serviceID, runtime, err)
	}

	return &runtimeMeta, nil
}

// LoadServiceValues loads values.yaml for a service runtime
func LoadServiceValues(serviceID, runtime string) (map[string]interface{}, error) {
	valuesPath := filepath.Join("services", serviceID, runtime, "values.yaml")

	data, err := assets.CatalogFS.ReadFile(valuesPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read values for service '%s' runtime '%s': %w", serviceID, runtime, err)
	}

	var values map[string]interface{}
	if err := yaml.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("failed to parse values for service '%s' runtime '%s': %w", serviceID, runtime, err)
	}

	return values, nil
}

// ListArchitectures lists all available architectures
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
			// Log error but continue with other architectures
			continue
		}
		architectures = append(architectures, *arch)
	}

	return architectures, nil
}

// ListServices lists all available deployable services
// Only returns services where DependencyOnly is false (default)
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
			// Log error but continue with other services
			continue
		}

		// Only include services that are not dependency-only
		if !service.DependencyOnly {
			services = append(services, *service)
		}
	}

	return services, nil
}

// ArchitectureExists checks if an architecture exists
func ArchitectureExists(id string) bool {
	_, err := LoadArchitecture(id)
	return err == nil
}

// ServiceExists checks if a service exists
func ServiceExists(id string) bool {
	_, err := LoadService(id)
	return err == nil
}

// LoadServicePodTemplate loads and renders a pod template for a service
// It reuses the existing embedTemplateProvider by setting the root to the service path
func LoadServicePodTemplate(
	serviceID, runtime, templateName, appName string,
	values map[string]interface{},
) (*models.PodSpec, error) {
	// Create template provider with service root path
	var runtimeType runtimeTypes.RuntimeType
	if runtime == "podman" {
		runtimeType = runtimeTypes.RuntimeTypePodman
	} else {
		runtimeType = runtimeTypes.RuntimeTypeOpenShift
	}

	tp := templates.NewEmbedTemplateProvider(templates.EmbedOptions{
		FS:      &assets.CatalogFS,
		Root:    filepath.Join("services", serviceID),
		Runtime: runtimeType,
	})

	// Use the existing LoadPodTemplate method
	// The template provider expects app name as empty string since we're at service root
	params := map[string]interface{}{
		"Values":  values,
		"AppName": appName,
	}

	return tp.LoadPodTemplate("", templateName, params)
}

// GetServiceTemplates returns the list of pod templates for a service
func GetServiceTemplates(serviceID, runtime string) ([]string, error) {
	runtimeMeta, err := LoadServiceRuntimeMetadata(serviceID, runtime)
	if err != nil {
		return nil, err
	}

	templates := make([]string, len(runtimeMeta.PodTemplates))
	for i, pt := range runtimeMeta.PodTemplates {
		templates[i] = pt.Template
	}

	return templates, nil
}

// ResolveServiceDependencies resolves all dependencies for one or more services recursively
// Returns a flat list of all unique service IDs needed (including the services themselves)
// Accepts either service IDs (strings) or ServiceReferences
func ResolveServiceDependencies(services ...interface{}) ([]string, error) {
	visited := make(map[string]bool)
	var result []string

	for _, svc := range services {
		var serviceID string
		switch v := svc.(type) {
		case string:
			serviceID = v
		case types.ServiceReference:
			serviceID = v.ID
		default:
			return nil, fmt.Errorf("invalid service type: %T", svc)
		}

		if err := resolveDependenciesRecursive(serviceID, visited, &result); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// resolveDependenciesRecursive performs depth-first traversal of dependencies
func resolveDependenciesRecursive(serviceID string, visited map[string]bool, result *[]string) error {
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
		if err := resolveDependenciesRecursive(dep.ID, visited, result); err != nil {
			return err
		}
	}

	// Add current service to result
	*result = append(*result, serviceID)

	return nil
}

// GetDeploymentOrder returns services grouped into deployment layers
// Services in the same layer can be deployed in parallel
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

	// Build edges (dependencies)
	for _, svcID := range serviceIDs {
		service, err := LoadService(svcID)
		if err != nil {
			return nil, fmt.Errorf("failed to load service '%s': %w", svcID, err)
		}

		for _, dep := range service.Dependencies {
			// Only add edge if dependency is in our service list
			if _, exists := graph[dep.ID]; exists {
				graph[dep.ID] = append(graph[dep.ID], svcID)
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

	// Check for circular dependencies
	processedCount := 0
	for _, layer := range layers {
		processedCount += len(layer)
	}
	if processedCount != len(serviceIDs) {
		return nil, fmt.Errorf("circular dependency detected in services")
	}

	return layers, nil
}

// ValidateDependencies checks if all dependencies for given services exist
func ValidateDependencies(serviceIDs []string) error {
	for _, svcID := range serviceIDs {
		service, err := LoadService(svcID)
		if err != nil {
			return fmt.Errorf("service '%s' not found: %w", svcID, err)
		}

		// Check all dependencies (all are required)
		for _, dep := range service.Dependencies {
			if !ServiceExists(dep.ID) {
				return fmt.Errorf("service '%s' requires dependency '%s' which does not exist", svcID, dep.ID)
			}
		}
	}

	return nil
}

// Made with Bob

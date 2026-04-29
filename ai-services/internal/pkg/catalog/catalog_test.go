package catalog

import (
	"testing"

	"github.com/project-ai-services/ai-services/assets"
	"github.com/project-ai-services/ai-services/internal/pkg/cli/templates"
	"github.com/project-ai-services/ai-services/internal/pkg/runtime"
	"github.com/project-ai-services/ai-services/internal/pkg/runtime/types"
	"github.com/project-ai-services/ai-services/internal/pkg/utils"
	"github.com/project-ai-services/ai-services/internal/pkg/vars"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Architecture Tests
// ============================================================================

func TestLoadArchitecture(t *testing.T) {
	t.Run("Load existing architecture", func(t *testing.T) {
		arch, err := LoadArchitecture("rag")
		require.NoError(t, err, "Should load existing architecture without error")
		require.NotNil(t, arch, "Architecture should not be nil")

		assert.Equal(t, "rag", arch.ID, "Architecture ID should match")
		assert.NotEmpty(t, arch.Name, "Architecture name should not be empty")
		assert.NotEmpty(t, arch.Description, "Architecture description should not be empty")
		assert.NotEmpty(t, arch.Version, "Architecture version should not be empty")
		assert.NotNil(t, arch.Services, "Architecture services should not be nil")
		assert.NotEmpty(t, arch.Services, "Architecture should have services")
	})

	t.Run("Load non-existent architecture", func(t *testing.T) {
		arch, err := LoadArchitecture("non-existent")
		assert.Error(t, err, "Should return error for non-existent architecture")
		assert.Nil(t, arch, "Architecture should be nil on error")
		assert.Contains(t, err.Error(), "failed to load architecture", "Error should mention loading architecture")
	})

	t.Run("Validate architecture structure", func(t *testing.T) {
		arch, err := LoadArchitecture("rag")
		require.NoError(t, err)

		// Validate services
		assert.Greater(t, len(arch.Services), 0, "Should have at least one service")

		// Check all services have IDs and track if at least one is required
		hasRequiredService := false
		for _, svc := range arch.Services {
			assert.NotEmpty(t, svc.ID, "Service should have an ID")
			// Verify the service exists
			_, err := LoadService(svc.ID)
			assert.NoError(t, err, "Service '%s' should exist", svc.ID)
			if !svc.Optional {
				hasRequiredService = true
			}
		}

		assert.True(t, hasRequiredService, "Should have at least one required service")
	})
}

func TestListArchitectures(t *testing.T) {
	t.Run("List all architectures", func(t *testing.T) {
		architectures, err := ListArchitectures()
		require.NoError(t, err, "Should list architectures without error")
		assert.NotEmpty(t, architectures, "Should have at least one architecture")

		// Verify each architecture has required fields
		for _, arch := range architectures {
			assert.NotEmpty(t, arch.ID, "Architecture ID should not be empty")
			assert.NotEmpty(t, arch.Name, "Architecture name should not be empty")
			assert.NotEmpty(t, arch.Version, "Architecture version should not be empty")
		}
	})

	t.Run("Verify rag architecture is in list", func(t *testing.T) {
		architectures, err := ListArchitectures()
		require.NoError(t, err)

		found := false
		for _, arch := range architectures {
			if arch.ID == "rag" {
				found = true
				break
			}
		}
		assert.True(t, found, "RAG architecture should be in the list")
	})
}

func TestArchitectureExists(t *testing.T) {
	testCases := []struct {
		name        string
		archID      string
		shouldExist bool
	}{
		{
			name:        "Existing architecture",
			archID:      "rag",
			shouldExist: true,
		},
		{
			name:        "Non-existent architecture",
			archID:      "non-existent",
			shouldExist: false,
		},
		{
			name:        "Empty architecture ID",
			archID:      "",
			shouldExist: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadArchitecture(tc.archID)
			if tc.shouldExist {
				assert.NoError(t, err, "Architecture '%s' should exist", tc.archID)
			} else {
				assert.Error(t, err, "Architecture '%s' should not exist", tc.archID)
			}
		})
	}
}

// ============================================================================
// Service Tests
// ============================================================================

func TestLoadService(t *testing.T) {
	testCases := []struct {
		name      string
		serviceID string
		wantError bool
	}{
		{
			name:      "Load opensearch service",
			serviceID: "opensearch",
			wantError: false,
		},
		{
			name:      "Load chat service",
			serviceID: "chat",
			wantError: false,
		},
		{
			name:      "Load instruct service",
			serviceID: "instruct",
			wantError: false,
		},
		{
			name:      "Load embedding service",
			serviceID: "embedding",
			wantError: false,
		},
		{
			name:      "Load reranker service",
			serviceID: "reranker",
			wantError: false,
		},
		{
			name:      "Load digitize service",
			serviceID: "digitize",
			wantError: false,
		},
		{
			name:      "Load summarize service",
			serviceID: "summarize",
			wantError: false,
		},
		{
			name:      "Load non-existent service",
			serviceID: "non-existent",
			wantError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			service, err := LoadService(tc.serviceID)

			if tc.wantError {
				assert.Error(t, err, "Should return error for non-existent service")
				assert.Nil(t, service, "Service should be nil on error")
				assert.Contains(t, err.Error(), "failed to load service", "Error should mention loading service")
			} else {
				require.NoError(t, err, "Should load existing service without error")
				require.NotNil(t, service, "Service should not be nil")

				assert.Equal(t, tc.serviceID, service.ID, "Service ID should match")
				assert.NotEmpty(t, service.Name, "Service name should not be empty")
				assert.NotEmpty(t, service.Description, "Service description should not be empty")
				// Version is now in runtime metadata, not in base service metadata
				// SupportedRuntimes may be empty if defined at runtime level
			}
		})
	}
}

func TestLoadServiceRuntimeMetadata(t *testing.T) {
	// Initialize the global RuntimeFactory for tests
	vars.RuntimeFactory = runtime.NewRuntimeFactory(types.RuntimeTypePodman)

	t.Run("Load runtime metadata for opensearch", func(t *testing.T) {
		meta, err := LoadServiceRuntimeMetadata("opensearch")
		require.NoError(t, err, "Should load runtime metadata without error")
		require.NotNil(t, meta, "Runtime metadata should not be nil")
		assert.NotEmpty(t, meta.Name, "Should have service name")
		assert.NotEmpty(t, meta.Version, "Should have version")
	})

	t.Run("Load runtime metadata for non-existent service", func(t *testing.T) {
		meta, err := LoadServiceRuntimeMetadata("non-existent")
		assert.Error(t, err, "Should return error for non-existent service")
		assert.Nil(t, meta, "Runtime metadata should be nil on error")
	})

	t.Run("Validate runtime metadata structure", func(t *testing.T) {
		meta, err := LoadServiceRuntimeMetadata("chat")
		require.NoError(t, err)

		// Validate metadata fields
		assert.NotEmpty(t, meta.Name, "Should have service name")
		assert.NotEmpty(t, meta.Version, "Should have version")
	})
}

func TestLoadServiceValues(t *testing.T) {
	t.Run("Load values for opensearch podman", func(t *testing.T) {
		// Set runtime to podman for this test
		vars.RuntimeFactory = runtime.NewRuntimeFactory(types.RuntimeTypePodman)

		values, err := LoadServiceValues("opensearch")
		require.NoError(t, err, "Should load values without error")
		require.NotNil(t, values, "Values should not be nil")
		assert.NotEmpty(t, values, "Values should not be empty")
	})

	t.Run("Load values for chat podman", func(t *testing.T) {
		// Set runtime to podman for this test
		vars.RuntimeFactory = runtime.NewRuntimeFactory(types.RuntimeTypePodman)

		values, err := LoadServiceValues("chat")
		require.NoError(t, err, "Should load values without error")
		require.NotNil(t, values, "Values should not be nil")
	})

	t.Run("Load values for non-existent service", func(t *testing.T) {
		// Set runtime to podman for this test
		vars.RuntimeFactory = runtime.NewRuntimeFactory(types.RuntimeTypePodman)

		values, err := LoadServiceValues("non-existent")
		assert.Error(t, err, "Should return error for non-existent service")
		assert.Nil(t, values, "Values should be nil on error")
	})

	t.Run("Load values for non-existent runtime", func(t *testing.T) {
		// Set runtime to a non-existent runtime type
		// Since we can't create an invalid runtime type, we'll skip this test
		// or test with openshift if the service doesn't support it
		t.Skip("Skipping test for non-existent runtime as runtime is now determined by vars.RuntimeFactory")
	})
}

func TestListServices(t *testing.T) {
	t.Run("List all services", func(t *testing.T) {
		services, err := ListServices()
		require.NoError(t, err, "Should list services without error")
		assert.NotEmpty(t, services, "Should have at least one service")

		// Verify each service has required fields
		for _, svc := range services {
			assert.NotEmpty(t, svc.ID, "Service ID should not be empty")
			assert.NotEmpty(t, svc.Name, "Service name should not be empty")
			// Version is now in runtime metadata, not in base service metadata
			// SupportedRuntimes may be empty if defined at runtime level
		}
	})

	t.Run("Verify expected deployable services are in list", func(t *testing.T) {
		services, err := ListServices()
		require.NoError(t, err)

		// Only deployable services (not dependency-only)
		expectedServices := []string{
			"chat",
			"digitize",
			"summarize",
		}

		serviceMap := make(map[string]bool)
		for _, svc := range services {
			serviceMap[svc.ID] = true
		}

		for _, expected := range expectedServices {
			assert.True(t, serviceMap[expected], "Service '%s' should be in the list", expected)
		}
	})

	t.Run("Verify dependency-only services are not in list", func(t *testing.T) {
		services, err := ListServices()
		require.NoError(t, err)

		// These services should NOT be in the list (dependency-only)
		dependencyOnlyServices := []string{
			"opensearch",
			"instruct",
			"instruct-cpu",
			"embedding",
			"reranker",
			"reranker-cpu",
		}

		serviceMap := make(map[string]bool)
		for _, svc := range services {
			serviceMap[svc.ID] = true
		}

		for _, depOnly := range dependencyOnlyServices {
			assert.False(t, serviceMap[depOnly], "Dependency-only service '%s' should not be in the list", depOnly)
		}
	})

	t.Run("Verify service count", func(t *testing.T) {
		services, err := ListServices()
		require.NoError(t, err)
		assert.Equal(t, 3, len(services), "Should have exactly 3 deployable services (chat, digitize, summarize)")
	})
}

func TestServiceExists(t *testing.T) {
	testCases := []struct {
		name        string
		serviceID   string
		shouldExist bool
	}{
		{
			name:        "Existing service - opensearch",
			serviceID:   "opensearch",
			shouldExist: true,
		},
		{
			name:        "Existing service - chat",
			serviceID:   "chat",
			shouldExist: true,
		},
		{
			name:        "Non-existent service",
			serviceID:   "non-existent",
			shouldExist: false,
		},
		{
			name:        "Empty service ID",
			serviceID:   "",
			shouldExist: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadService(tc.serviceID)
			if tc.shouldExist {
				assert.NoError(t, err, "Service '%s' should exist", tc.serviceID)
			} else {
				assert.Error(t, err, "Service '%s' should not exist", tc.serviceID)
			}
		})
	}
}

func TestGetServiceTemplates(t *testing.T) {
	t.Run("Get templates for opensearch using LoadAllTemplates", func(t *testing.T) {
		tp := templates.NewEmbedTemplateProvider(&assets.CatalogFS, "services")
		tmpls, err := tp.LoadAllTemplates("opensearch")
		require.NoError(t, err, "Should load templates without error")
		assert.NotEmpty(t, tmpls, "Should have at least one template")

		templateFiles := utils.ExtractMapKeys(tmpls)
		for _, tmpl := range templateFiles {
			assert.NotEmpty(t, tmpl, "Template path should not be empty")
		}
	})

	t.Run("Get templates for chat using LoadAllTemplates", func(t *testing.T) {
		tp := templates.NewEmbedTemplateProvider(&assets.CatalogFS, "services")
		tmpls, err := tp.LoadAllTemplates("chat")
		require.NoError(t, err, "Should load templates without error")
		assert.NotEmpty(t, tmpls, "Chat service should have templates")
	})

	t.Run("Get templates for non-existent service", func(t *testing.T) {
		tp := templates.NewEmbedTemplateProvider(&assets.CatalogFS, "services")
		tmpls, err := tp.LoadAllTemplates("non-existent")
		assert.Error(t, err, "Should return error for non-existent service")
		assert.Nil(t, tmpls, "Templates should be nil on error")
	})
}

// ============================================================================
// Dependency Resolution Tests
// ============================================================================

func TestGetDeploymentOrder(t *testing.T) {
	t.Run("Get deployment order for single service", func(t *testing.T) {
		layers, err := GetDeploymentOrder([]string{"opensearch"})
		require.NoError(t, err, "Should get deployment order without error")
		require.NotNil(t, layers, "Layers should not be nil")

		assert.Equal(t, 1, len(layers), "Should have one layer")
		assert.Contains(t, layers[0], "opensearch", "First layer should contain opensearch")
	})

	t.Run("Get deployment order for services with dependencies", func(t *testing.T) {
		// Chat depends on opensearch, embedding, instruct, reranker
		services := []string{"opensearch", "embedding", "instruct", "reranker", "chat"}
		layers, err := GetDeploymentOrder(services)
		require.NoError(t, err, "Should get deployment order without error")
		require.NotNil(t, layers, "Layers should not be nil")

		assert.Greater(t, len(layers), 1, "Should have multiple layers")

		// OpenSearch should be in first layer (no dependencies)
		assert.Contains(t, layers[0], "opensearch", "First layer should contain opensearch")

		// Chat should be in last layer (depends on others)
		lastLayer := layers[len(layers)-1]
		assert.Contains(t, lastLayer, "chat", "Last layer should contain chat")

		// Verify all services are included
		allServices := flattenLayers(layers)
		for _, svc := range services {
			assert.Contains(t, allServices, svc, "All services should be in deployment order")
		}
	})

	t.Run("Get deployment order for complex architecture", func(t *testing.T) {
		// Get all services for RAG architecture manually
		services := []string{"opensearch", "embedding", "instruct", "reranker", "chat", "digitize"}

		layers, err := GetDeploymentOrder(services)
		require.NoError(t, err, "Should get deployment order without error")
		require.NotNil(t, layers, "Layers should not be nil")

		assert.Greater(t, len(layers), 0, "Should have at least one layer")

		// Verify all services are included
		allServices := flattenLayers(layers)
		assert.Equal(t, len(services), len(allServices), "All services should be in deployment order")
	})

	t.Run("Handle empty service list", func(t *testing.T) {
		layers, err := GetDeploymentOrder([]string{})
		require.NoError(t, err, "Should handle empty list without error")
		assert.Empty(t, layers, "Layers should be empty for empty input")
	})

	t.Run("Detect circular dependencies", func(t *testing.T) {
		// This test would require creating services with circular dependencies
		// Since our current services don't have circular deps, we just verify
		// the algorithm completes without hanging
		services := []string{"chat", "opensearch", "embedding", "instruct", "reranker"}
		layers, err := GetDeploymentOrder(services)
		require.NoError(t, err, "Should complete without hanging")
		assert.NotNil(t, layers, "Should return layers")
	})
	t.Run("Get deployment order with CPU-transformed services", func(t *testing.T) {
		// Simulate rag-dev architecture where instruct and reranker are transformed to CPU variants
		// summarize depends on instruct, but the list has instruct-cpu
		services := []string{"opensearch", "embedding", "instruct-cpu", "reranker-cpu", "chat", "digitize", "summarize"}
		layers, err := GetDeploymentOrder(services)
		require.NoError(t, err, "Should get deployment order without error")
		require.NotNil(t, layers, "Layers should not be nil")

		// Find which layer each service is in
		serviceLayer := make(map[string]int)
		for layerIdx, layer := range layers {
			for _, svc := range layer {
				serviceLayer[svc] = layerIdx
			}
		}

		// Verify summarize is deployed AFTER instruct-cpu (its dependency)
		assert.Greater(t, serviceLayer["summarize"], serviceLayer["instruct-cpu"],
			"summarize should be deployed after instruct-cpu (its dependency)")

		// Verify chat is deployed AFTER all its dependencies
		assert.Greater(t, serviceLayer["chat"], serviceLayer["opensearch"],
			"chat should be deployed after opensearch")
		assert.Greater(t, serviceLayer["chat"], serviceLayer["instruct-cpu"],
			"chat should be deployed after instruct-cpu")
		assert.Greater(t, serviceLayer["chat"], serviceLayer["embedding"],
			"chat should be deployed after embedding")
		assert.Greater(t, serviceLayer["chat"], serviceLayer["reranker-cpu"],
			"chat should be deployed after reranker-cpu")

		// Verify all services are included
		allServices := flattenLayers(layers)
		assert.Equal(t, len(services), len(allServices), "All services should be in deployment order")
		t.Run("Only transform known CPU variants", func(t *testing.T) {
			// Test that only known CPU variants (instruct-cpu, reranker-cpu) are transformed
			// This test verifies the whitelist approach works correctly
			services := []string{"opensearch", "embedding", "instruct-cpu", "reranker-cpu", "chat", "summarize"}
			layers, err := GetDeploymentOrder(services)
			require.NoError(t, err, "Should get deployment order without error")
			require.NotNil(t, layers, "Layers should not be nil")

			// Find which layer each service is in
			serviceLayer := make(map[string]int)
			for layerIdx, layer := range layers {
				for _, svc := range layer {
					serviceLayer[svc] = layerIdx
				}
			}

			// Verify chat is deployed AFTER instruct-cpu (its dependency on instruct is transformed)
			assert.Greater(t, serviceLayer["chat"], serviceLayer["instruct-cpu"],
				"chat should be deployed after instruct-cpu (transformed dependency)")

			// Verify chat is deployed AFTER reranker-cpu (its dependency on reranker is transformed)
			assert.Greater(t, serviceLayer["chat"], serviceLayer["reranker-cpu"],
				"chat should be deployed after reranker-cpu (transformed dependency)")

			// Verify summarize is deployed AFTER instruct-cpu (its dependency on instruct is transformed)
			assert.Greater(t, serviceLayer["summarize"], serviceLayer["instruct-cpu"],
				"summarize should be deployed after instruct-cpu (transformed dependency)")

			// Verify all services are included
			allServices := flattenLayers(layers)
			assert.Equal(t, len(services), len(allServices), "All services should be in deployment order")
		})

	})

}

// ============================================================================
// Concurrency Tests
// ============================================================================

func TestConcurrency(t *testing.T) {
	t.Run("Concurrent architecture loads", func(t *testing.T) {
		done := make(chan bool, 10)

		for i := 0; i < 10; i++ {
			go func() {
				_, err := LoadArchitecture("rag")
				assert.NoError(t, err, "Concurrent load should not error")
				done <- true
			}()
		}

		for i := 0; i < 10; i++ {
			<-done
		}
	})

	t.Run("Concurrent service loads", func(t *testing.T) {
		services := []string{"opensearch", "chat", "instruct", "embedding", "reranker"}
		done := make(chan bool, len(services)*2)

		for _, svc := range services {
			go func(serviceID string) {
				_, err := LoadService(serviceID)
				assert.NoError(t, err, "Concurrent load should not error for %s", serviceID)
				done <- true
			}(svc)

			go func(serviceID string) {
				_, err := LoadServiceRuntimeMetadata(serviceID)
				assert.NoError(t, err, "Concurrent runtime metadata load should not error for %s", serviceID)
				done <- true
			}(svc)
		}

		for i := 0; i < len(services)*2; i++ {
			<-done
		}
	})
}

// ============================================================================
// Helper Functions
// ============================================================================

func indexOf(slice []string, value string) int {
	for i, v := range slice {
		if v == value {
			return i
		}
	}
	return -1
}

func flattenLayers(layers [][]string) []string {
	var result []string
	for _, layer := range layers {
		result = append(result, layer...)
	}
	return result
}

// Made with Bob

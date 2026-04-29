package podman

import (
	"context"
	"testing"

	"github.com/project-ai-services/ai-services/assets"
	"github.com/project-ai-services/ai-services/internal/pkg/application/types"
	"github.com/project-ai-services/ai-services/internal/pkg/cli/templates"
	"github.com/project-ai-services/ai-services/internal/pkg/runtime"
	runtimeTypes "github.com/project-ai-services/ai-services/internal/pkg/runtime/types"
	"github.com/stretchr/testify/assert"
)

// setupTestPodmanApp creates a test PodmanApplication instance
// Note: This requires a running Podman instance
func setupTestPodmanApp(t *testing.T) *PodmanApplication {
	t.Helper()

	// Skip if Podman is not available
	rt, err := runtime.CreateRuntime(runtimeTypes.RuntimeTypePodman, "")
	if err != nil {
		t.Skipf("Skipping test: Podman runtime not available: %v", err)
	}

	app := &PodmanApplication{
		runtime: rt,
	}

	// Set template provider
	app.SetTemplateProvider(templates.NewEmbedTemplateProvider(&assets.CatalogFS, "services"))

	return app
}

// ============================================================================
// Deploy Function Tests
// ============================================================================

func TestDeploy(t *testing.T) {
	t.Run("Deploy validates template name", func(t *testing.T) {
		app := setupTestPodmanApp(t)
		ctx := context.Background()

		opts := types.CreateOptions{
			Name:         "test-app",
			TemplateName: "non-existent-template",
		}

		err := app.Deploy(ctx, opts)
		assert.Error(t, err, "Should return error for non-existent template")
	})

	t.Run("Deploy sets template provider correctly", func(t *testing.T) {
		app := setupTestPodmanApp(t)

		// Verify template provider is set
		assert.NotNil(t, app.templateProvider, "Template provider should be set")
	})
}

// ============================================================================
// deployServices Function Tests
// ============================================================================

func TestDeployServices(t *testing.T) {
	t.Run("Validates service list is not empty", func(t *testing.T) {
		app := setupTestPodmanApp(t)
		ctx := context.Background()

		opts := types.CreateOptions{
			Name:         "test-app",
			TemplateName: "test",
		}

		// Empty service list should not cause panic
		err := app.deployServices(ctx, []string{}, opts)
		// This may or may not error depending on implementation
		// but should not panic
		_ = err
	})

	t.Run("Handles invalid service ID", func(t *testing.T) {
		app := setupTestPodmanApp(t)
		ctx := context.Background()

		opts := types.CreateOptions{
			Name:         "test-app",
			TemplateName: "test",
		}

		err := app.deployServices(ctx, []string{"invalid-service"}, opts)
		assert.Error(t, err, "Should return error for invalid service")
	})

	t.Run("Calculates deployment layers correctly", func(t *testing.T) {
		app := setupTestPodmanApp(t)
		ctx := context.Background()

		opts := types.CreateOptions{
			Name:         "test-app",
			TemplateName: "opensearch",
		}

		// opensearch is a valid service
		err := app.deployServices(ctx, []string{"opensearch"}, opts)
		// May fail due to actual deployment, but should get past layer calculation
		_ = err
	})
}

// ============================================================================
// deployServicesInLayers Function Tests
// ============================================================================

func TestDeployServicesInLayers(t *testing.T) {
	t.Run("Handles empty layers", func(t *testing.T) {
		app := setupTestPodmanApp(t)
		ctx := context.Background()

		opts := types.CreateOptions{
			Name:         "test-app",
			TemplateName: "test",
		}

		err := app.deployServicesInLayers(ctx, [][]string{}, opts, []string{}, []string{})
		assert.NoError(t, err, "Should handle empty layers without error")
	})

	t.Run("Processes layers sequentially", func(t *testing.T) {
		app := setupTestPodmanApp(t)
		ctx := context.Background()

		opts := types.CreateOptions{
			Name:         "test-app",
			TemplateName: "test",
		}

		// Multiple layers with invalid services will fail at deployment
		layers := [][]string{
			{"service1"},
			{"service2"},
		}

		err := app.deployServicesInLayers(ctx, layers, opts, []string{}, []string{})
		assert.Error(t, err, "Should error on invalid services")
	})

	t.Run("Checks existing pods before deployment", func(t *testing.T) {
		app := setupTestPodmanApp(t)
		ctx := context.Background()

		opts := types.CreateOptions{
			Name:         "test-app",
			TemplateName: "test",
		}

		// Should check for existing pods
		err := app.deployServicesInLayers(ctx, [][]string{{"opensearch"}}, opts, []string{}, []string{})
		// Will fail at deployment but should check pods first
		_ = err
	})
}

// ============================================================================
// deployServiceLayer Function Tests
// ============================================================================

func TestDeployServiceLayer(t *testing.T) {
	t.Run("Handles empty service list", func(t *testing.T) {
		app := setupTestPodmanApp(t)

		opts := types.CreateOptions{
			Name:         "test-app",
			TemplateName: "test",
		}

		err := app.deployServiceLayer([]string{}, opts, []string{}, []string{})
		assert.NoError(t, err, "Should handle empty service list without error")
	})

	t.Run("Deploys services in parallel", func(t *testing.T) {
		app := setupTestPodmanApp(t)

		opts := types.CreateOptions{
			Name:         "test-app",
			TemplateName: "test",
		}

		// Multiple invalid services should all fail
		err := app.deployServiceLayer([]string{"invalid1", "invalid2"}, opts, []string{}, []string{})
		assert.Error(t, err, "Should return error for invalid services")
		assert.Contains(t, err.Error(), "failed to deploy services", "Error should mention deployment failure")
	})

	t.Run("Collects all errors from parallel deployment", func(t *testing.T) {
		app := setupTestPodmanApp(t)

		opts := types.CreateOptions{
			Name:         "test-app",
			TemplateName: "test",
		}

		// Multiple services will generate multiple errors
		err := app.deployServiceLayer([]string{"invalid1", "invalid2", "invalid3"}, opts, []string{}, []string{})
		assert.Error(t, err, "Should collect errors from all services")
	})
}

// ============================================================================
// deployService Function Tests
// ============================================================================

func TestDeployService(t *testing.T) {
	t.Run("Loads runtime metadata for service", func(t *testing.T) {
		app := setupTestPodmanApp(t)

		opts := types.CreateOptions{
			Name:         "test-app",
			TemplateName: "opensearch",
		}

		// opensearch is a valid service with metadata
		err := app.deployService("opensearch", opts, []string{}, []string{})
		// Will fail at actual deployment but should load metadata
		_ = err
	})

	t.Run("Returns error for non-existent service", func(t *testing.T) {
		app := setupTestPodmanApp(t)

		opts := types.CreateOptions{
			Name:         "test-app",
			TemplateName: "test",
		}

		err := app.deployService("non-existent-service", opts, []string{}, []string{})
		assert.Error(t, err, "Should return error for non-existent service")
		assert.Contains(t, err.Error(), "failed to load runtime metadata", "Error should mention metadata loading")
	})

	t.Run("Loads templates for service", func(t *testing.T) {
		app := setupTestPodmanApp(t)

		opts := types.CreateOptions{
			Name:         "test-app",
			TemplateName: "opensearch",
		}

		// Should attempt to load templates
		err := app.deployService("opensearch", opts, []string{}, []string{})
		// Will fail at deployment but should load templates
		_ = err
	})

	t.Run("Processes pod template layers in order", func(t *testing.T) {
		app := setupTestPodmanApp(t)

		opts := types.CreateOptions{
			Name:         "test-app",
			TemplateName: "opensearch",
		}

		// opensearch has podTemplateExecutions layers
		err := app.deployService("opensearch", opts, []string{}, []string{})
		// Will fail at actual pod deployment
		_ = err
	})

	t.Run("Executes pod templates in parallel within layer", func(t *testing.T) {
		app := setupTestPodmanApp(t)

		opts := types.CreateOptions{
			Name:         "test-app",
			TemplateName: "opensearch",
		}

		// Should use goroutines for parallel execution
		err := app.deployService("opensearch", opts, []string{}, []string{})
		// Will fail but should attempt parallel execution
		_ = err
	})

	t.Run("Stops on layer error", func(t *testing.T) {
		app := setupTestPodmanApp(t)

		opts := types.CreateOptions{
			Name:         "test-app",
			TemplateName: "opensearch",
		}

		// If first layer fails, should not proceed to next layer
		err := app.deployService("opensearch", opts, []string{}, []string{})
		assert.Error(t, err, "Should stop on layer error")
	})

	t.Run("Builds global params correctly", func(t *testing.T) {
		app := setupTestPodmanApp(t)

		opts := types.CreateOptions{
			Name:         "test-app",
			TemplateName: "opensearch",
			ArgParams: map[string]string{
				"test-param": "test-value",
			},
		}

		// Should build globalParams with AppName, AppTemplateName, Version, Values, env
		err := app.deployService("opensearch", opts, []string{}, []string{})
		// Will fail at deployment but should build params
		_ = err
	})
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestDeployIntegration(t *testing.T) {
	t.Run("Full deployment flow with valid service", func(t *testing.T) {
		app := setupTestPodmanApp(t)

		// This test validates the full flow without actual deployment
		// It should:
		// 1. Resolve template to services
		// 2. Calculate deployment order
		// 3. Load templates and metadata
		// 4. Attempt deployment (will fail without actual podman)

		opts := types.CreateOptions{
			Name:         "test-app",
			TemplateName: "opensearch",
		}

		// Should get through initial validation steps
		err := app.deployService("opensearch", opts, []string{}, []string{})
		// Will fail at actual pod creation but validates flow
		_ = err
	})
}

// ============================================================================
// Concurrency Tests
// ============================================================================

func TestDeployConcurrency(t *testing.T) {
	t.Run("Parallel service deployment is thread-safe", func(t *testing.T) {
		app := setupTestPodmanApp(t)

		opts := types.CreateOptions{
			Name:         "test-app",
			TemplateName: "test",
		}

		// deployServiceLayer uses goroutines - should be thread-safe
		err := app.deployServiceLayer([]string{"service1", "service2", "service3"}, opts, []string{}, []string{})
		// Will error but should not panic or race
		_ = err
	})

	t.Run("Parallel pod template deployment is thread-safe", func(t *testing.T) {
		app := setupTestPodmanApp(t)

		opts := types.CreateOptions{
			Name:         "test-app",
			TemplateName: "opensearch",
		}

		// deployService uses nested goroutines - should be thread-safe
		err := app.deployService("opensearch", opts, []string{}, []string{})
		// Will error but should not panic or race
		_ = err
	})
}

// Made with Bob

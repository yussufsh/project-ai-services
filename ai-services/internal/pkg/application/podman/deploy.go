package podman

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/project-ai-services/ai-services/assets"
	"github.com/project-ai-services/ai-services/internal/pkg/application/types"
	"github.com/project-ai-services/ai-services/internal/pkg/catalog"
	catalogTypes "github.com/project-ai-services/ai-services/internal/pkg/catalog/types"
	"github.com/project-ai-services/ai-services/internal/pkg/cli/helpers"
	"github.com/project-ai-services/ai-services/internal/pkg/cli/templates"
	"github.com/project-ai-services/ai-services/internal/pkg/logger"
	runtimeTypes "github.com/project-ai-services/ai-services/internal/pkg/runtime/types"
	"github.com/project-ai-services/ai-services/internal/pkg/specs"
	"github.com/project-ai-services/ai-services/internal/pkg/spinner"
)

// Deploy deploys either an architecture or a service based on the template name
func (p *PodmanApplication) Deploy(ctx context.Context, opts types.CreateOptions) error {
	// Determine what to deploy and get list of services
	var allServices []string

	// Set template provider for services
	p.templateProvider = templates.NewEmbedTemplateProvider(templates.EmbedOptions{
		FS:      &assets.CatalogFS,
		Root:    "services",
		Runtime: runtimeTypes.RuntimeTypePodman,
	})

	// Try to load as architecture first
	if arch, err := catalog.LoadArchitecture(opts.TemplateName); err == nil {
		logger.Infof("Deploying architecture '%s'\n", arch.ID)

		// Convert ServiceReferences to interface{} slice for ResolveServiceDependencies
		serviceRefs := make([]interface{}, len(arch.Services))
		for i, svc := range arch.Services {
			serviceRefs[i] = svc
		}
		logger.Infof("Architecture contains %d services\n", len(arch.Services))

		// Resolve dependencies for all services
		allServices, err = catalog.ResolveServiceDependencies(serviceRefs...)
		if err != nil {
			return err
		}

		logger.Infof("Total services to deploy (including dependencies): %d\n", len(allServices))
		logger.Infof("Services: %v\n", allServices)
	} else if _, err := catalog.LoadService(opts.TemplateName); err == nil {
		// Try to load as service
		logger.Infof("Deploying service '%s'\n", opts.TemplateName)

		// Resolve service dependencies
		allServices, err = catalog.ResolveServiceDependencies(opts.TemplateName)
		if err != nil {
			return fmt.Errorf("failed to resolve service dependencies: %w", err)
		}

		logger.Infof("Total services to deploy (including dependencies): %d\n", len(allServices))
		logger.Infof("Services: %v\n", allServices)
	} else {
		return fmt.Errorf("template '%s' is neither a valid architecture nor service", opts.TemplateName)
	}

	// Deploy all services using common logic
	if err := p.deployServices(ctx, allServices, opts); err != nil {
		return err
	}

	logger.Infof("'%s' deployed successfully as '%s'\n", opts.TemplateName, opts.Name)
	return nil
}

// deployServices handles the common deployment logic for a list of services
func (p *PodmanApplication) deployServices(
	ctx context.Context,
	allServices []string,
	opts types.CreateOptions,
) error {
	// Validate dependencies
	if err := catalog.ValidateDependencies(allServices); err != nil {
		return fmt.Errorf("dependency validation failed: %w", err)
	}

	// Get deployment order (layers)
	deploymentLayers, err := catalog.GetDeploymentOrder(allServices)
	if err != nil {
		return fmt.Errorf("failed to determine deployment order: %w", err)
	}

	logger.Infof("Deployment will proceed in %d layers\n", len(deploymentLayers))
	for i, layer := range deploymentLayers {
		logger.Infof("Layer %d: %v\n", i+1, layer)
	}

	// Validate and allocate Spyre cards for all services
	totalSpyreCards, err := p.calculateTotalSpyreCards(allServices, opts.Name)
	if err != nil {
		return fmt.Errorf("failed to calculate Spyre card requirements: %w", err)
	}

	var pciAddresses []string
	if totalSpyreCards > 0 {
		pciAddresses, err = p.allocateSpyreCards(totalSpyreCards)
		if err != nil {
			return err
		}
	}

	// Download images and models for all services
	for _, serviceID := range allServices {

		// Create a copy of opts with the service ID as template name
		serviceOpts := opts
		serviceOpts.TemplateName = serviceID

		if err := p.prepareApplicationArtifacts(ctx, serviceOpts); err != nil {
			return err
		}
	}

	// Deploy services layer by layer
	if err := p.deployServicesInLayers(ctx, deploymentLayers, opts, pciAddresses); err != nil {
		return err
	}

	// Print next steps using service template provider
	if err := helpers.PrintNextSteps(p.runtime, opts.Name, opts.TemplateName, p.templateProvider); err != nil {
		logger.Infof("failed to display next steps: %v\n", err)
	}

	return nil
}

// deployServicesInLayers deploys services layer by layer
func (p *PodmanApplication) deployServicesInLayers(
	ctx context.Context,
	layers [][]string,
	opts types.CreateOptions,
	pciAddresses []string,
) error {
	s := spinner.New(fmt.Sprintf("Deploying architecture '%s'...", opts.Name))
	s.Start(ctx)

	for layerIdx, layer := range layers {
		logger.Infof("\nDeploying Layer %d/%d: %v\n", layerIdx+1, len(layers), layer)
		logger.Infoln("-------")

		// Deploy all services in this layer in parallel
		if err := p.deployServiceLayer(ctx, layer, opts, &pciAddresses); err != nil {
			s.Fail(fmt.Sprintf("failed to deploy layer %d", layerIdx+1))
			return fmt.Errorf("failed to deploy layer %d: %w", layerIdx+1, err)
		}

		logger.Infof("Layer %d completed successfully\n", layerIdx+1)
	}

	s.Stop(fmt.Sprintf("Architecture '%s' deployed successfully", opts.Name))
	return nil
}

// deployServiceLayer deploys all services in a layer in parallel
func (p *PodmanApplication) deployServiceLayer(
	ctx context.Context,
	serviceIDs []string,
	opts types.CreateOptions,
	pciAddresses *[]string,
) error {
	var wg sync.WaitGroup
	errCh := make(chan error, len(serviceIDs))

	for _, serviceID := range serviceIDs {
		wg.Add(1)
		go func(svcID string) {
			defer wg.Done()
			if err := p.deployService(ctx, svcID, opts, pciAddresses); err != nil {
				errCh <- fmt.Errorf("service '%s': %w", svcID, err)
			}
		}(serviceID)
	}

	wg.Wait()
	close(errCh)

	// Collect errors
	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to deploy services: %v", errs)
	}

	return nil
}

// deployService deploys a single service
func (p *PodmanApplication) deployService(
	ctx context.Context,
	serviceID string,
	opts types.CreateOptions,
	pciAddresses *[]string,
) error {
	logger.Infof("Deploying service '%s'...\n", serviceID)

	// Load service metadata (for future use in validation/logging)
	_, err := catalog.LoadService(serviceID)
	if err != nil {
		return fmt.Errorf("failed to load service metadata: %w", err)
	}

	// Load runtime metadata
	runtimeMeta, err := catalog.LoadServiceRuntimeMetadata(serviceID, "podman")
	if err != nil {
		return fmt.Errorf("failed to load runtime metadata: %w", err)
	}

	// Load service values
	serviceValues, err := catalog.LoadServiceValues(serviceID, "podman")
	if err != nil {
		return fmt.Errorf("failed to load service values: %w", err)
	}

	// Merge with user-provided values (simple override for now)
	mergedValues := make(map[string]interface{})
	for k, v := range serviceValues {
		mergedValues[k] = v
	}
	for k, v := range opts.ArgParams {
		mergedValues[k] = v
	}

	// Deploy each pod template for this service
	for _, podTemplate := range runtimeMeta.PodTemplates {
		if err := p.deployServicePodTemplate(
			ctx,
			serviceID,
			podTemplate,
			opts.Name,
			mergedValues,
			pciAddresses,
		); err != nil {
			return fmt.Errorf("failed to deploy pod template '%s': %w", podTemplate.Name, err)
		}
	}

	logger.Infof("Service '%s' deployed successfully\n", serviceID)
	return nil
}

// deployServicePodTemplate deploys a single pod template for a service
func (p *PodmanApplication) deployServicePodTemplate(
	ctx context.Context,
	serviceID string,
	podTemplate catalogTypes.PodTemplateConfig,
	appName string,
	values map[string]interface{},
	pciAddresses *[]string,
) error {
	logger.Infof("  Deploying pod template '%s' for service '%s'...\n", podTemplate.Name, serviceID)

	// Check if pod already exists
	podName := fmt.Sprintf("%s--%s", appName, serviceID)
	exists, err := p.runtime.PodExists(podName)
	if err != nil {
		return fmt.Errorf("failed to check if pod exists: %w", err)
	}

	if exists {
		logger.Infof("  Pod '%s' already exists, skipping\n", podName)
		return nil
	}

	// Load and render the pod template to get podSpec for annotations
	podSpec, err := catalog.LoadServicePodTemplate(
		serviceID,
		"podman",
		podTemplate.Template,
		appName,
		values,
	)
	if err != nil {
		return fmt.Errorf("failed to load pod template: %w", err)
	}

	// Get pod annotations for deployment options
	podAnnotations := specs.FetchPodAnnotations(*podSpec)

	// Get environment parameters for the pod (including PCI addresses for Spyre cards)
	env, err := p.returnEnvParamsForPod(podSpec, podAnnotations, pciAddresses)
	if err != nil {
		return fmt.Errorf("failed to get env params: %w", err)
	}

	// Merge env into values for final rendering
	finalValues := make(map[string]interface{})
	for k, v := range values {
		finalValues[k] = v
	}
	finalValues["env"] = env

	// Re-render the template with env params to get final YAML
	podSpec, err = catalog.LoadServicePodTemplate(
		serviceID,
		"podman",
		podTemplate.Template,
		appName,
		finalValues,
	)
	if err != nil {
		return fmt.Errorf("failed to re-render pod template with env: %w", err)
	}

	// Marshal podSpec to YAML bytes
	yamlBytes, err := yaml.Marshal(podSpec)
	if err != nil {
		return fmt.Errorf("failed to marshal pod spec to YAML: %w", err)
	}

	// Deploy the pod using podman kube play
	reader := bytes.NewReader(yamlBytes)
	deployOpts := p.constructPodDeployOptions(podAnnotations)

	pods, err := p.runtime.CreatePod(reader, deployOpts)
	if err != nil {
		return fmt.Errorf("failed to create pod: %w", err)
	}

	logger.Infof("  Successfully deployed pod '%s'\n", podName)

	// Wait for readiness if configured
	if podTemplate.WaitForReady {
		timeout := podTemplate.ReadinessTimeout
		if timeout == 0 {
			timeout = 5 * time.Minute // Default timeout
		}

		logger.Infof("  Waiting for pod '%s' to be ready (timeout: %s)...\n", podName, timeout)

		for _, pod := range pods {
			// Perform readiness checks similar to legacy code
			if err := p.doContainersCreationCheck(podSpec, podTemplate.Name, pod.Name, pod.ID); err != nil {
				return fmt.Errorf("container creation check failed: %w", err)
			}

			// Check readiness for each container
			pInfo, err := p.runtime.InspectPod(pod.ID)
			if err != nil {
				return fmt.Errorf("failed to inspect pod: %w", err)
			}

			for _, container := range pInfo.Containers {
				if err := p.doContainerReadinessCheck(podTemplate.Name, pod.Name, container.ID); err != nil {
					return fmt.Errorf("container readiness check failed: %w", err)
				}
			}
		}

		logger.Infof("  Pod '%s' is ready\n", podName)
	}

	logger.Infof("  Pod template '%s' deployed successfully\n", podTemplate.Name)
	return nil
}

// calculateTotalSpyreCards calculates total Spyre cards needed for all services
func (p *PodmanApplication) calculateTotalSpyreCards(serviceIDs []string, appName string) (int, error) {
	total := 0

	for _, serviceID := range serviceIDs {
		runtimeMeta, err := catalog.LoadServiceRuntimeMetadata(serviceID, "podman")
		if err != nil {
			return 0, fmt.Errorf("failed to load runtime metadata for service '%s': %w", serviceID, err)
		}

		if runtimeMeta.ResourceRequirements != nil && runtimeMeta.ResourceRequirements.MinSpyre > 0 {
			total += runtimeMeta.ResourceRequirements.MinSpyre
		}
	}

	return total, nil
}

// allocateSpyreCards allocates Spyre cards for deployment
func (p *PodmanApplication) allocateSpyreCards(required int) ([]string, error) {
	pciAddresses, err := helpers.FindFreeSpyreCards()
	if err != nil {
		return nil, fmt.Errorf("failed to find free Spyre cards: %w", err)
	}

	available := len(pciAddresses)
	if available < required {
		return nil, fmt.Errorf("insufficient Spyre cards: required %d, available %d", required, available)
	}

	logger.Infof("Allocated %d Spyre cards for deployment\n", required)
	return pciAddresses[:required], nil
}

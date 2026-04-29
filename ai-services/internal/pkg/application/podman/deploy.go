package podman

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/project-ai-services/ai-services/assets"
	"github.com/project-ai-services/ai-services/internal/pkg/application/types"
	"github.com/project-ai-services/ai-services/internal/pkg/catalog"
	"github.com/project-ai-services/ai-services/internal/pkg/cli/helpers"
	"github.com/project-ai-services/ai-services/internal/pkg/cli/templates"
	"github.com/project-ai-services/ai-services/internal/pkg/constants"
	"github.com/project-ai-services/ai-services/internal/pkg/logger"
	"github.com/project-ai-services/ai-services/internal/pkg/spinner"
	"github.com/project-ai-services/ai-services/internal/pkg/utils"
	"github.com/project-ai-services/ai-services/internal/pkg/vars"
)

// Deploy deploys either an architecture or a service based on the template name
func (p *PodmanApplication) Deploy(ctx context.Context, opts types.CreateOptions) error {
	// Proceed to create application
	logger.Infof("Creating application '%s' using template '%s'\n", opts.Name, opts.TemplateName)
	p.SetTemplateProvider(templates.NewEmbedTemplateProvider(&assets.CatalogFS, "services"))

	// Resolve template to services
	allServices, err := catalog.ResolveTemplateToServices(opts.TemplateName)
	if err != nil {
		return err
	}

	serviceCount := len(allServices)
	logger.Infof("Total services to deploy (including dependencies): %d\n", serviceCount, 0)
	logger.Infof("Services: %v\n", allServices)

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
	// Check if pods already exists with the given application name
	existingPods, err := helpers.CheckExistingPodsForApplication(p.runtime, opts.Name)
	if err != nil {
		return fmt.Errorf("failed while checking existing pods for application: %w", err)
	}

	// Get deployment order (layers)
	// Note: Dependencies are already validated by ResolveTemplateToServices
	deploymentLayers, err := catalog.GetDeploymentOrder(allServices)
	if err != nil {
		return fmt.Errorf("failed to determine deployment order: %w", err)
	}

	// Calculate total number of pods to deploy and required Spyre cards
	totalPods := 0
	totalReqSpyreCards := 0
	for _, serviceID := range allServices {
		tmpls, err := p.templateProvider.LoadAllTemplates(serviceID)
		if err != nil {
			return fmt.Errorf("failed to load templates for service '%s': %w", serviceID, err)
		}
		totalPods += len(tmpls)

		// Calculate required Spyre cards for this service using the same logic as create.go
		reqCount, err := p.calculateReqSpyreCards(utils.ExtractMapKeys(tmpls), serviceID, opts.Name)
		if err != nil {
			return fmt.Errorf("failed to calculate Spyre cards for service '%s': %w", serviceID, err)
		}

		totalReqSpyreCards += reqCount
	}

	// If no Spyre cards are required go ahead
	var pciAddresses []string
	if totalReqSpyreCards != 0 {
		pciAddresses, err = allocateSpyreCards(p, totalReqSpyreCards)
		if err != nil {
			return err
		}
	}

	// if all the pods for given application are already deployed, just log and do not proceed further
	if len(existingPods) == totalPods {
		logger.Infof("Pods for given app: %s are already deployed. Please use 'ai-services application ps %s' to see the pods deployed\n", opts.Name, opts.Name)

		return nil
	}

	layerCount := len(deploymentLayers)
	logger.Infof("Deployment will proceed in %d layers\n", layerCount, 0)
	for i, layer := range deploymentLayers {
		logger.Infof("Layer %d: %v\n", i+1, layer)
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
	if err := p.deployServicesInLayers(ctx, deploymentLayers, opts, pciAddresses, existingPods); err != nil {
		return err
	}

	logger.Infoln("-------")

	// Print next steps for all deployed services
	logger.Infoln(constants.NextStepsTitle + ":")
	logger.Infoln("-------")
	for _, serviceID := range allServices {
		if err := helpers.PrintNextSteps(p.templateProvider, p.runtime, opts.Name, serviceID); err != nil {
			// do not want to fail the overall deployment if we cannot print next steps
			logger.Infof("failed to display next steps for service '%s': %v\n", serviceID, err)
		}
	}
	logger.Infof("- Run \"ai-services application info %s --runtime %s\" to view service endpoints.\n\n", opts.Name, vars.RuntimeFactory.GetRuntimeType().String())
	logger.Infoln("")

	return nil
}

// deployServicesInLayers deploys services layer by layer
func (p *PodmanApplication) deployServicesInLayers(
	ctx context.Context,
	layers [][]string,
	opts types.CreateOptions,
	pciAddresses []string,
	existingPods []string,
) error {
	s := spinner.New(fmt.Sprintf("Deploying application '%s'...", opts.Name))
	s.Start(ctx)

	existingPods, err := helpers.CheckExistingPodsForApplication(p.runtime, opts.Name)
	if err != nil {
		return fmt.Errorf("failed while checking existing pods for application: %w", err)
	}

	for layerIdx, layer := range layers {
		logger.Infof("\nDeploying Layer %d/%d: %v\n", layerIdx+1, len(layers), layer)
		logger.Infoln("-------")

		// Deploy all services in this layer in parallel
		if err := p.deployServiceLayer(layer, opts, pciAddresses, existingPods); err != nil {
			s.Fail(fmt.Sprintf("failed to deploy layer %d", layerIdx+1))
			return fmt.Errorf("failed to deploy layer %d: %w", layerIdx+1, err)
		}

		logger.Infof("Layer %d completed successfully\n", layerIdx+1, 0)
	}

	s.Stop(fmt.Sprintf("Application '%s' deployed successfully", opts.Name))

	return nil
}

// deployServiceLayer deploys all services in a layer in parallel
func (p *PodmanApplication) deployServiceLayer(
	serviceIDs []string,
	opts types.CreateOptions,
	pciAddresses []string,
	existingPods []string,
) error {
	var wg sync.WaitGroup
	errCh := make(chan error, len(serviceIDs))

	for _, serviceID := range serviceIDs {
		wg.Add(1)
		go func(svcID string) {
			defer wg.Done()
			if err := p.deployService(svcID, opts, pciAddresses, existingPods); err != nil {
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
	serviceID string, opts types.CreateOptions,
	pciAddresses []string, existingPods []string) error {
	logger.Infof("Deploying service '%s'...\n", serviceID)

	// Load runtime metadata
	runtimeMeta, err := catalog.LoadServiceRuntimeMetadata(serviceID)
	if err != nil {
		return fmt.Errorf("failed to load runtime metadata: %w", err)
	}

	// Load all templates for this service
	tmpls, err := p.templateProvider.LoadAllTemplates(serviceID)
	if err != nil {
		return fmt.Errorf("failed to load templates for service '%s': %w", serviceID, err)
	}
	// Load values for template rendering
	values, err := p.templateProvider.LoadValues(runtimeMeta.Name, opts.ValuesFiles, opts.ArgParams)

	// Build globalParams
	globalParams := map[string]any{
		"AppName":         opts.Name,
		"AppTemplateName": serviceID,
		"Version":         runtimeMeta.Version,
		"Values":          values,
		"env":             map[string]map[string]string{},
	}

	// Deploy all templates found in templates folder
	logger.Infof("\nDeploying all templates for service '%s'...\n", serviceID)
	logger.Infoln("-------")

	var wg sync.WaitGroup
	errCh := make(chan error, len(tmpls))

	// Deploy all pod templates in parallel
	for podTemplateName := range tmpls {
		wg.Add(1)
		go func(t string) {
			defer wg.Done()
			if err := p.executePodTemplateLayer(tmpls[t], globalParams, pciAddresses, existingPods, opts.Name, opts.ValuesFiles, opts.ArgParams); err != nil {
				errCh <- err
			}
		}(podTemplateName)
	}

	wg.Wait()
	close(errCh)

	// collect all errors
	var errs []error
	for e := range errCh {
		errs = append(errs, e)
	}

	// If any errors exist, return them
	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	logger.Infof("Service '%s' deployed successfully\n", serviceID)
	return nil
}

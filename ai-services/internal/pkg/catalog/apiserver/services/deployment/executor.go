package deployment

import (
	"context"
	"fmt"
	"strconv"

	"github.com/project-ai-services/ai-services/internal/pkg/catalog"
	apimodels "github.com/project-ai-services/ai-services/internal/pkg/catalog/apiserver/models"
	"github.com/project-ai-services/ai-services/internal/pkg/catalog/apiserver/services/deployment/repository/openshift"
	"github.com/project-ai-services/ai-services/internal/pkg/catalog/apiserver/services/deployment/repository/podman"
	"github.com/project-ai-services/ai-services/internal/pkg/catalog/db/repository"
	catalogutils "github.com/project-ai-services/ai-services/internal/pkg/catalog/utils"
	"github.com/project-ai-services/ai-services/internal/pkg/proxy"
	"github.com/project-ai-services/ai-services/internal/pkg/runtime"
	openshiftRuntime "github.com/project-ai-services/ai-services/internal/pkg/runtime/openshift"
	podmanRuntime "github.com/project-ai-services/ai-services/internal/pkg/runtime/podman"
	remoteRuntime "github.com/project-ai-services/ai-services/internal/pkg/runtime/remote"
	"github.com/project-ai-services/ai-services/internal/pkg/runtime/types"
	workerconstants "github.com/project-ai-services/ai-services/internal/pkg/worker/constants"
)

// DeploymentExecutor orchestrates the complete deployment process.
// It uses the DeploymentPlanner to create a plan and then executes it
// using the appropriate runtime-specific deployer.
type DeploymentExecutor struct {
	planner         *DeploymentPlanner
	catalogProvider *catalog.CatalogProvider
	appRepo         repository.ApplicationRepository
	serviceRepo     repository.ServiceRepository
	componentRepo   repository.ComponentRepository
	// workerRegistry is used to resolve a RemoteRuntime for worker deployments.
	// Nil means remote deployments are not supported (non-worker setup).
	workerRegistry remoteRuntime.WorkerRegistry
}

// NewDeploymentExecutor creates a new DeploymentExecutor instance.
func NewDeploymentExecutor(
	catalogProvider *catalog.CatalogProvider,
	appRepo repository.ApplicationRepository,
	serviceRepo repository.ServiceRepository,
	componentRepo repository.ComponentRepository,
) *DeploymentExecutor {
	return &DeploymentExecutor{
		planner:         NewDeploymentPlanner(catalogProvider, componentRepo),
		catalogProvider: catalogProvider,
		appRepo:         appRepo,
		serviceRepo:     serviceRepo,
		componentRepo:   componentRepo,
	}
}

// WithWorkerRegistry configures the executor with a worker registry so it can
// dispatch deployments to remote worker nodes. Call this once after construction
// when the catalog API server has a live worker gateway.
func (e *DeploymentExecutor) WithWorkerRegistry(reg remoteRuntime.WorkerRegistry) *DeploymentExecutor {
	e.workerRegistry = reg

	return e
}

// ExecuteWithPlan executes deployment using an existing plan.
// This is used when the plan has already been created and database records inserted.
func (e *DeploymentExecutor) ExecuteWithPlan(
	ctx context.Context,
	plan *DeploymentPlan,
	req apimodels.CreateApplicationRequest,
	runtimeType types.RuntimeType,
) error {
	if err := e.executeDeployment(ctx, plan, req, runtimeType); err != nil {
		return fmt.Errorf("failed to execute deployment: %w", err)
	}

	return nil
}

// executeDeployment routes to the correct deployer.
//
// When plan.WorkerName is set it creates a RemoteRuntime (gRPC CommandStream)
// and dispatches to the Podman deployer (for Podman workers) or the OpenShift
// deployer (for OpenShift workers). For OpenShift workers no config is
// injected — the worker manages routes natively.
//
// When plan.WorkerName is empty it falls back to a local runtime of the given
// runtimeType.
func (e *DeploymentExecutor) executeDeployment(
	ctx context.Context,
	plan *DeploymentPlan,
	req apimodels.CreateApplicationRequest,
	runtimeType types.RuntimeType,
) error {
	// ── Remote worker deployment ──────────────────────────────────────────────
	if plan.WorkerName != "" {
		return e.executeWorkerDeployment(ctx, plan, req)
	}

	// ── Local deployment ──────────────────────────────────────────────────────
	switch runtimeType {
	case types.RuntimeTypePodman:
		return e.executePodmanDeployment(ctx, plan, req)
	case types.RuntimeTypeOpenShift:
		return e.executeOpenShiftDeployment(ctx, plan, req)
	default:
		return fmt.Errorf("unsupported runtime type: %s", runtimeType)
	}
}

// executeWorkerDeployment handles deployments to a named remote worker.
// It builds a RemoteRuntime over the gRPC stream, determines the worker's
// actual runtime type, and calls the appropriate deployer.
func (e *DeploymentExecutor) executeWorkerDeployment(
	ctx context.Context,
	plan *DeploymentPlan,
	req apimodels.CreateApplicationRequest,
) error {
	if e.workerRegistry == nil {
		return fmt.Errorf("worker deployment requested for %q but no worker registry is configured", plan.WorkerName)
	}

	// Resolve the worker's declared runtime type.
	rtStr, ok := e.workerRegistry.WorkerRuntimeType(plan.WorkerName)
	if !ok {
		return fmt.Errorf("worker %q is not connected", plan.WorkerName)
	}

	workerType := types.RuntimeType(rtStr)

	// Build a RemoteRuntime: all calls are forwarded over the gRPC CommandStream
	// regardless of whether the remote is Podman or OpenShift.
	rt, err := runtime.NewRuntimeFactory(workerType).CreateRemote(plan.WorkerName, e.workerRegistry)
	if err != nil {
		return fmt.Errorf("create remote runtime for worker %q: %w", plan.WorkerName, err)
	}

	switch workerType {
	case types.RuntimeTypePodman:
		return e.runPodmanDeployer(ctx, plan, req, rt)
	case types.RuntimeTypeOpenShift:
		return e.runOpenShiftDeployer(ctx, plan, req, rt)
	default:
		return fmt.Errorf("worker %q has unsupported runtime type %q", plan.WorkerName, workerType)
	}
}

// runPodmanDeployer creates a PodmanDeployer with a WorkerConfig sourced from
// the worker's registration metadata and runs the deployment.
func (e *DeploymentExecutor) runPodmanDeployer(
	ctx context.Context,
	plan *DeploymentPlan,
	req apimodels.CreateApplicationRequest,
	rt runtime.Runtime,
) error {
	deployer := podman.NewPodmanDeployer(rt, e.catalogProvider, e.appRepo, e.serviceRepo, e.componentRepo)

	// Inject the worker's podman config so registerApplicationRoutes uses the
	// correct domain suffix, HTTPS port, and routes via the gRPC proxy manager.
	wc, err := e.podmanWorkerConfig(plan.WorkerName, rt)
	if err != nil {
		return fmt.Errorf("worker podman config for %q: %w", plan.WorkerName, err)
	}

	deployer.SetPodmanWorkerConfig(wc)

	return deployer.ExecuteDeployment(ctx, plan, req)
}

// runOpenShiftDeployer creates an OpenShiftDeployer backed by the RemoteRuntime
// and runs the deployment. OpenShift workers manage routes natively via the
// Kubernetes API so no config is needed.
func (e *DeploymentExecutor) runOpenShiftDeployer(
	ctx context.Context,
	plan *DeploymentPlan,
	req apimodels.CreateApplicationRequest,
	rt runtime.Runtime,
) error {
	deployer := openshift.NewOpenShiftDeployer(rt, e.catalogProvider, e.appRepo, e.serviceRepo, e.componentRepo)

	return deployer.ExecuteDeployment(ctx, plan, req)
}

// podmanWorkerConfig builds a WorkerConfig for a Podman worker from its
// registration metadata. The ProxyManager is a thin adapter that forwards
// route registration calls back to the worker over gRPC.
func (e *DeploymentExecutor) podmanWorkerConfig(workerName string, rt runtime.Runtime) (podman.PodmanWorkerConfig, error) {
	meta, ok := e.workerRegistry.WorkerMetadata(workerName)
	if !ok {
		return podman.PodmanWorkerConfig{}, fmt.Errorf("no metadata for worker %q", workerName)
	}

	domainSuffix := meta[workerconstants.MetaKeyDomainSuffix]
	httpsPort := meta[workerconstants.MetaKeyHTTPSPort]

	if domainSuffix == "" {
		return podman.PodmanWorkerConfig{}, fmt.Errorf("worker %q metadata missing %q", workerName, workerconstants.MetaKeyDomainSuffix)
	}

	if _, err := strconv.Atoi(httpsPort); err != nil {
		return podman.PodmanWorkerConfig{}, fmt.Errorf("worker %q metadata has invalid %q: %w", workerName, workerconstants.MetaKeyHTTPSPort, err)
	}

	return podman.PodmanWorkerConfig{
		ProxyManager: proxy.NewRuntimeProxyManager(rt),
		DomainSuffix: domainSuffix,
		HTTPSPort:    httpsPort,
	}, nil
}

// executePodmanDeployment executes deployment for local Podman runtime.
func (e *DeploymentExecutor) executePodmanDeployment(
	ctx context.Context,
	plan *DeploymentPlan,
	req apimodels.CreateApplicationRequest,
) error {
	// Initialize Podman runtime client
	rt, err := podmanRuntime.NewPodmanClient()
	if err != nil {
		return fmt.Errorf("failed to initialize Podman runtime: %w", err)
	}

	// Create podman deployer
	deployer := podman.NewPodmanDeployer(
		rt,
		e.catalogProvider,
		e.appRepo,
		e.serviceRepo,
		e.componentRepo,
	)

	// Execute deployment - handles both architectures and standalone services
	return deployer.ExecuteDeployment(ctx, plan, req)
}

// executeOpenShiftDeployment executes deployment for local OpenShift runtime via Helm.
func (e *DeploymentExecutor) executeOpenShiftDeployment(
	ctx context.Context,
	plan *DeploymentPlan,
	req apimodels.CreateApplicationRequest,
) error {
	// Initialize OpenShift runtime client scoped to the application's namespace
	// so that ListRoutes, ListPods etc. query the correct namespace.
	ns := catalogutils.AppNamespace(plan.ApplicationID)
	rt, err := openshiftRuntime.NewOpenshiftClientWithNamespace(ns)
	if err != nil {
		return fmt.Errorf("failed to initialize OpenShift runtime: %w", err)
	}

	// Create openshift deployer
	deployer := openshift.NewOpenShiftDeployer(
		rt,
		e.catalogProvider,
		e.appRepo,
		e.serviceRepo,
		e.componentRepo,
	)

	return deployer.ExecuteDeployment(ctx, plan, req)
}

// Made with Bob

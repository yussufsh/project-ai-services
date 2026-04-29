package handlers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/project-ai-services/ai-services/internal/pkg/catalog"
	"github.com/project-ai-services/ai-services/internal/pkg/catalog/types"
	"github.com/project-ai-services/ai-services/internal/pkg/vars"
)

// CatalogHandler handles catalog-related HTTP requests
type CatalogHandler struct{}

// NewCatalogHandler creates a new catalog handler
func NewCatalogHandler() *CatalogHandler {
	return &CatalogHandler{}
}

// ListArchitectures godoc
//
//	@Summary		List available architectures
//	@Description	Retrieves a list of all available architecture templates
//	@Tags			Catalog
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{array}		types.Architecture
//	@Failure		401	{object}	ErrorResponse	"Unauthorized - Invalid or missing access token"
//	@Failure		500	{object}	ErrorResponse	"Internal Server Error"
//	@Router			/architectures [get]
func (h *CatalogHandler) ListArchitectures(c *gin.Context) {
	architectures, err := catalog.ListArchitectures()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("Failed to list architectures: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, architectures)
}

// GetArchitectureDetails godoc
//
//	@Summary		Get architecture details
//	@Description	Retrieves detailed information about a specific architecture template
//	@Tags			Catalog
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		string	true	"Architecture template ID (e.g., 'rag')"
//	@Success		200	{object}	types.Architecture
//	@Failure		401	{object}	ErrorResponse	"Unauthorized - Invalid or missing access token"
//	@Failure		404	{object}	ErrorResponse	"Architecture not found"
//	@Failure		500	{object}	ErrorResponse	"Internal Server Error"
//	@Router			/architectures/{id} [get]
func (h *CatalogHandler) GetArchitectureDetails(c *gin.Context) {
	id := c.Param("id")

	architecture, err := catalog.LoadArchitecture(id)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: fmt.Sprintf("Architecture '%s' not found: %v", id, err),
		})
		return
	}

	c.JSON(http.StatusOK, architecture)
}

// ListServices godoc
//
//	@Summary		List available services
//	@Description	Retrieves a list of all deployable service templates. Dependency-only services are excluded from this list. Returns service summaries without endpoints and pod templates.
//	@Tags			Catalog
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{array}		types.ServiceSummary
//	@Failure		401	{object}	ErrorResponse	"Unauthorized - Invalid or missing access token"
//	@Failure		500	{object}	ErrorResponse	"Internal Server Error"
//	@Router			/services [get]
func (h *CatalogHandler) ListServices(c *gin.Context) {
	// Get runtime from global factory
	runtime := vars.RuntimeFactory.GetRuntimeType()

	servicesList, err := catalog.ListServicesWithRuntime(runtime)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("Failed to list services: %v", err),
		})
		return
	}

	// Convert to summaries (exclude endpoints and pod_templates)
	summaries := make([]types.ServiceSummary, len(servicesList))
	for i, svc := range servicesList {
		summaries[i] = catalog.ToServiceSummary(&svc)
	}

	c.JSON(http.StatusOK, summaries)
}

// GetServiceDetails godoc
//
//	@Summary		Get service details
//	@Description	Retrieves detailed information about a specific service template
//	@Tags			Catalog
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		string	true	"Service template ID (e.g., 'summarize')"
//	@Success		200	{object}	types.ServiceSummary
//	@Failure		401	{object}	ErrorResponse	"Unauthorized - Invalid or missing access token"
//	@Failure		404	{object}	ErrorResponse	"Service not found"
//	@Failure		500	{object}	ErrorResponse	"Internal Server Error"
//	@Router			/services/{id} [get]
func (h *CatalogHandler) GetServiceDetails(c *gin.Context) {
	id := c.Param("id")

	// Get runtime from global factory
	runtime := vars.RuntimeFactory.GetRuntimeType()

	service, err := catalog.LoadServiceWithRuntime(id, runtime)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: fmt.Sprintf("Service '%s' not found: %v", id, err),
		})
		return
	}

	c.JSON(http.StatusOK, catalog.ToServiceSummary(service))
}

// GetServiceCustomParameters godoc
//
//	@Summary		Get service custom parameters
//	@Description	Retrieves custom parameters schema for a specific service template. Returns JSON Schema format that UI can use to generate dynamic forms with validation.
//	@Tags			Catalog
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		string	true	"Service template ID (e.g., 'rag', 'summarize')"
//	@Success		200	{object}	map[string]interface{}	"JSON Schema for service parameters"
//	@Failure		401	{object}	ErrorResponse			"Unauthorized - Invalid or missing access token"
//	@Failure		404	{object}	ErrorResponse			"Service not found"
//	@Failure		500	{object}	ErrorResponse			"Internal Server Error"
//	@Router			/services/{id}/params [get]
func (h *CatalogHandler) GetServiceCustomParameters(c *gin.Context) {
	id := c.Param("id")

	// First verify the service exists
	if _, err := catalog.LoadService(id); err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: fmt.Sprintf("Service '%s' not found", id),
		})
		return
	}

	// Load values.yaml for the service (using podman as default runtime)
	// In a real implementation, you might want to support multiple runtimes
	values, err := catalog.LoadServiceValues(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("Failed to load parameters for service '%s': %v", id, err),
		})
		return
	}

	// Convert values.yaml to JSON Schema format
	// This is a simplified version - in production you'd want more sophisticated schema generation
	schema := convertValuesToJSONSchema(values)

	c.JSON(http.StatusOK, schema)
}

// ServiceDetailsResponse represents the response for service details including dependencies
type ServiceDetailsResponse struct {
	Service      types.ServiceSummary   `json:"service"`
	Dependencies []types.ServiceSummary `json:"dependencies"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error string `json:"error"`
}

// convertValuesToJSONSchema converts values.yaml to JSON Schema format
// This is a simplified implementation - production version would be more sophisticated
func convertValuesToJSONSchema(values map[string]interface{}) map[string]interface{} {
	schema := map[string]interface{}{
		"$schema":    "http://json-schema.org/draft-07/schema#",
		"type":       "object",
		"properties": map[string]interface{}{},
	}

	properties := schema["properties"].(map[string]interface{})

	for key, value := range values {
		properties[key] = inferSchemaFromValue(value)
	}

	return schema
}

// inferSchemaFromValue infers JSON Schema type from a value
func inferSchemaFromValue(value interface{}) map[string]interface{} {
	switch v := value.(type) {
	case string:
		return map[string]interface{}{
			"type":    "string",
			"default": v,
		}
	case int, int32, int64:
		return map[string]interface{}{
			"type":    "integer",
			"default": v,
		}
	case float32, float64:
		return map[string]interface{}{
			"type":    "number",
			"default": v,
		}
	case bool:
		return map[string]interface{}{
			"type":    "boolean",
			"default": v,
		}
	case []interface{}:
		itemSchema := map[string]interface{}{"type": "string"}
		if len(v) > 0 {
			itemSchema = inferSchemaFromValue(v[0])
		}
		return map[string]interface{}{
			"type":    "array",
			"items":   itemSchema,
			"default": v,
		}
	case map[string]interface{}:
		nestedProps := map[string]interface{}{}
		for k, val := range v {
			nestedProps[k] = inferSchemaFromValue(val)
		}
		return map[string]interface{}{
			"type":       "object",
			"properties": nestedProps,
		}
	default:
		return map[string]interface{}{
			"type": "string",
		}
	}
}

// Made with Bob

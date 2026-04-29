# Catalog API Testing Guide

This guide demonstrates how to test the Catalog Endpoints API with proper authentication.

## Prerequisites

1. Build the ai-services binary:
```bash
cd ai-services
go build -o bin/ai-services ./cmd/ai-services
```

2. Generate a password hash for the admin user:
```bash
echo "admin" | ./bin/ai-services catalog hashpw --stdin --no-confirm
# Output: 100000.6gUHLKoUT5XwXdRtOmqKag.NjLz7OIVDVgiP6fKR0lAW0LquYH5MHiaTgn7Nf2IRoo
```

## Starting the API Server

Start the server with the admin password hash:
```bash
cd ai-services
./bin/ai-services catalog apiserver \
  --port 8080 \
  --admin-password-hash '100000.6gUHLKoUT5XwXdRtOmqKag.NjLz7OIVDVgiP6fKR0lAW0LquYH5MHiaTgn7Nf2IRoo'
```

The server will start on `http://localhost:8080` with:
- Default admin username: `admin`
- Password: `admin` (matching the hash provided)
- Access token TTL: 3 hours
- Refresh token TTL: 7 days

## API Endpoints

### Health Check (No Authentication Required)
```bash
curl http://localhost:8080/healthz
```

### Swagger Documentation
Open in browser: `http://localhost:8080/swagger/index.html`

## Catalog Endpoints (Authentication Required)

All catalog endpoints require Bearer token authentication.

### 1. List Available Architectures
**Endpoint:** `GET /api/v1/architectures`

**Description:** Retrieves a list of all available architecture templates.

**Example:**
```bash
curl -H "Authorization: Bearer <your_token>" \
  http://localhost:8080/api/v1/architectures
```

**Response:** Array of architecture objects with fields:
- `id`: Architecture template ID
- `name`: Architecture name
- `description`: Detailed description
- `version`: Architecture version
- `type`: "architecture"
- `certified_by`: Certification authority
- `services`: Array of service references
- `supported_runtimes`: Array of supported runtimes
- `demo_link`, `code_link`, `documentation_link`: Optional links

### 2. Get Architecture Details
**Endpoint:** `GET /api/v1/architectures/{id}`

**Description:** Retrieves detailed information about a specific architecture template.

**Example:**
```bash
curl -H "Authorization: Bearer <your_token>" \
  http://localhost:8080/api/v1/architectures/rag
```

**Response:** Single architecture object with full details

### 3. List Available Services
**Endpoint:** `GET /api/v1/services`

**Description:** Retrieves a list of all deployable service templates. Dependency-only services are excluded.

**Example:**
```bash
curl -H "Authorization: Bearer <your_token>" \
  http://localhost:8080/api/v1/services
```

**Response:** Array of service summary objects (dependency-only services excluded):
```json
[
  {
    "id": "chat",
    "name": "Question and Answer",
    "description": "Answer questions in natural language...",
    "version": "1.0.0",
    "type": "service",
    "certified_by": "IBM",
    "architectures": ["rag"],
    "dependencies": [
      {"id": "opensearch", "version": ">=1.0.0"},
      {"id": "embedding", "version": ">=1.0.0"},
      {"id": "instruct", "version": ">=1.0.0"},
      {"id": "reranker", "version": ">=1.0.0"}
    ],
    "requirements": {
      "min_cpu": "2",
      "min_memory": "4Gi",
      "min_disk": "20Gi"
    }
  }
]
```

**Note:** Services with `dependency_only: true` (opensearch, instruct, embedding, reranker) are excluded from this list.

### 4. Get Service Details
**Endpoint:** `GET /api/v1/services/{id}`

**Description:** Retrieves detailed information about a specific service template, including resolved dependencies.

**Example:**
```bash
curl -H "Authorization: Bearer <your_token>" \
  http://localhost:8080/api/v1/services/chat
```

**Response for deployable service (e.g., chat):**
```json
{
  "id": "chat",
  "name": "Question and Answer",
  "description": "Answer questions in natural language...",
  "version": "1.0.0",
  "type": "service",
  "certified_by": "IBM",
  "architectures": ["rag"],
  "dependencies": [
    {"id": "opensearch", "version": ">=1.0.0"},
    {"id": "embedding", "version": ">=1.0.0"},
    {"id": "instruct", "version": ">=1.0.0"},
    {"id": "reranker", "version": ">=1.0.0"}
  ],
  "requirements": {
    "min_cpu": "2",
    "min_memory": "4Gi",
    "min_disk": "20Gi"
  }
}
```

**Response for dependency-only service (e.g., opensearch):**
```json
{
  "id": "opensearch",
  "name": "OpenSearch",
  "description": "Vector database for document storage and retrieval",
  "version": "1.0.0",
  "type": "service",
  "certified_by": "IBM",
  "dependency_only": true,
  "architectures": ["rag"],
  "requirements": {
    "min_cpu": "2",
    "min_memory": "8Gi",
    "min_disk": "50Gi"
  }
}
```

**Note:** Dependency-only services have `dependency_only: true` and no `dependencies` field.

### 5. Get Service Custom Parameters
**Endpoint:** `GET /api/v1/services/{id}/params`

**Description:** Retrieves custom parameters schema for a specific service template in JSON Schema format. This can be used by UIs to generate dynamic forms with validation.

**Example:**
```bash
curl -H "Authorization: Bearer <your_token>" \
  http://localhost:8080/api/v1/services/chat/params
```

**Response:** JSON Schema object with parameter definitions

## Authentication

All catalog endpoints require Bearer token authentication.

### Method 1: Using the CLI (Recommended)

1. **Login using the CLI**:
```bash
cd ai-services
echo "admin" | ./bin/ai-services catalog login \
  --username admin \
  --password-stdin \
  --server http://localhost:8080
```

The token is automatically saved to `~/.config/ai-services/catalog-token` and used for subsequent CLI commands.

### Method 2: Using curl

1. **Login to get an access token**:
```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}'
```

**Response:**
```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "refresh_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expires_in": 10800
}
```

2. **Extract and use the token**:
```bash
# Save token to variable
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}' | \
  python3 -c "import sys, json; print(json.load(sys.stdin)['access_token'])")

# Use token in requests
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/services
```

### Method 3: Refresh Token

When your access token expires, use the refresh token to get a new one:
```bash
curl -X POST http://localhost:8080/api/v1/auth/refresh \
  -H "Content-Type: application/json" \
  -d '{"refresh_token":"<your_refresh_token>"}'
```

## Testing with Swagger UI

1. Open `http://localhost:8080/swagger/index.html` in your browser
2. Click "Authorize" button at the top
3. Enter your Bearer token: `Bearer <your_token>`
4. Click "Authorize" and "Close"
5. Try out the catalog endpoints interactively

## Error Responses

All endpoints return appropriate HTTP status codes:

- `200 OK`: Successful request
- `401 Unauthorized`: Missing or invalid access token
- `404 Not Found`: Architecture or service not found
- `500 Internal Server Error`: Server error

Error response format:
```json
{
  "error": "error message description"
}
```

## Implementation Details

The catalog endpoints are implemented in:
- Handler: `ai-services/internal/pkg/catalog/apiserver/handlers/catalog.go`
- Router: `ai-services/internal/pkg/catalog/apiserver/router.go`
- Service Layer: `ai-services/internal/pkg/catalog/services/services.go`
- Types: `ai-services/internal/pkg/catalog/types/types.go`

All endpoints use the existing catalog service layer which reads from embedded YAML files in the `ai-services/assets/` directory.
Day N:

{{- if eq .API_STATUS "running" }}

- {{ .SERVICE_NAME }} is running. The OpenAI-compatible inference API is available at port 8000 on container 'llm-{{ .InstanceSlug }}'.
{{- else }}

- {{ .SERVICE_NAME }} is unavailable. Please make sure the 'llm' container is running.
{{- end }}

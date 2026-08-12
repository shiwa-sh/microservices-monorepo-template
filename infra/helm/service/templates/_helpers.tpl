{{- define "service.name" -}}
{{- required ".Values.name is required" .Values.name -}}
{{- end -}}

{{- define "service.fullname" -}}
{{ include "service.name" . }}
{{- end -}}

{{- define "service.labels" -}}
app.kubernetes.io/name: {{ include "service.name" . }}
app.kubernetes.io/managed-by: helm
app.kubernetes.io/part-of: platform
{{- end -}}

{{- define "service.selector" -}}
app.kubernetes.io/name: {{ include "service.name" . }}
{{- end -}}

{{/*
Image reference (ADR-0101, ADR-0103). Pin by digest when set (prod — the strongest
immutable identity, `repo@sha256:…`), else by SHA tag (dev/staging). Moving tags
are rejected by lint-floating-tags.
*/}}
{{- define "service.image" -}}
{{- $r := required ".Values.image.repository is required" .Values.image.repository -}}
{{- if .Values.image.digest -}}
{{ $r }}@{{ .Values.image.digest }}
{{- else -}}
{{ $r }}:{{ required ".Values.image.tag or .Values.image.digest is required (concrete git SHA / digest — ADR-0101, ADR-0103)" .Values.image.tag }}
{{- end -}}
{{- end -}}

{{- define "service.worker.image" -}}
{{- $r := required ".Values.worker.image.repository is required" .Values.worker.image.repository -}}
{{- if .Values.worker.image.digest -}}
{{ $r }}@{{ .Values.worker.image.digest }}
{{- else -}}
{{ $r }}:{{ required ".Values.worker.image.tag or .Values.worker.image.digest is required" .Values.worker.image.tag }}
{{- end -}}
{{- end -}}

{{/*
Traefik match rule (ADR-0306). Flat-API mode when ingress.resources is set: the
edge matches /api/<resource> for each resource the service owns, hiding the
service topology behind a flat namespace. Otherwise ingress.pathPrefix is a
literal prefix (the frontend catch-all "/").
*/}}
{{- define "service.ingressMatch" -}}
{{- $host := required ".Values.ingress.host is required when ingress.enabled" .Values.ingress.host -}}
{{- if .Values.ingress.resources -}}
{{- $prefixes := list -}}
{{- range .Values.ingress.resources -}}
{{- $prefixes = append $prefixes (printf "PathPrefix(`/api/%s`)" .) -}}
{{- end -}}
Host(`{{ $host }}`) && ({{ join " || " $prefixes }})
{{- else -}}
Host(`{{ $host }}`) && PathPrefix(`{{ required ".Values.ingress.resources or .Values.ingress.pathPrefix is required" .Values.ingress.pathPrefix }}`)
{{- end -}}
{{- end -}}

{{- define "service.otelEnv" -}}
- name: DEPLOY_ENV
  value: {{ .Values.otel.deployEnv | quote }}
- name: OTEL_EXPORTER_OTLP_ENDPOINT
  value: {{ .Values.otel.endpoint | quote }}
- name: OTEL_TRACES_SAMPLER_ARG
  value: {{ .Values.otel.sampling | quote }}
- name: OTEL_RESOURCE_ATTRIBUTES
  value: "service.name={{ include "service.name" . }},service.namespace=platform"
{{- end -}}

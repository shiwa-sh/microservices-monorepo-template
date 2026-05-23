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

{{- define "service.image" -}}
{{- $r := required ".Values.image.repository is required" .Values.image.repository -}}
{{- $t := required ".Values.image.tag is required (concrete git SHA — ADR-0002)" .Values.image.tag -}}
{{ $r }}:{{ $t }}
{{- end -}}

{{- define "service.worker.image" -}}
{{- $r := required ".Values.worker.image.repository is required" .Values.worker.image.repository -}}
{{- $t := required ".Values.worker.image.tag is required" .Values.worker.image.tag -}}
{{ $r }}:{{ $t }}
{{- end -}}

{{- define "service.ingressPath" -}}
{{- if .Values.ingress.pathPrefix -}}
{{ .Values.ingress.pathPrefix }}
{{- else -}}
/api/{{ include "service.name" . }}/
{{- end -}}
{{- end -}}

{{- define "service.otelEnv" -}}
- name: OTEL_EXPORTER_OTLP_ENDPOINT
  value: {{ .Values.otel.endpoint | quote }}
- name: OTEL_TRACES_SAMPLER_ARG
  value: {{ .Values.otel.sampling | quote }}
- name: OTEL_RESOURCE_ATTRIBUTES
  value: "service.name={{ include "service.name" . }},service.namespace=platform"
{{- end -}}

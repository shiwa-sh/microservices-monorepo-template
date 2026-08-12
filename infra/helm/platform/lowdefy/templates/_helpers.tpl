{{/*
lowdefy.image — the admin console image reference. Prefers an immutable digest
(prod, pinned by promote-on-release) over a tag (local/dev/staging), mirroring the
service chart's service.image helper (ADR-0103).
*/}}
{{- define "lowdefy.image" -}}
{{- $r := required ".Values.lowdefy.image.repository is required" .Values.lowdefy.image.repository -}}
{{- if .Values.lowdefy.image.digest -}}
{{ $r }}@{{ .Values.lowdefy.image.digest }}
{{- else -}}
{{ $r }}:{{ required ".Values.lowdefy.image.tag or .digest is required (concrete git SHA / digest — ADR-0101, ADR-0103)" .Values.lowdefy.image.tag }}
{{- end -}}
{{- end -}}

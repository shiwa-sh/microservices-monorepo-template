{{/*
The container half of the `restricted` Pod Security Standard (ADR-0200), identical
for every first-party workload this chart renders. Written once here because four
templates need the same four lines and a copy that drifts is a namespace that stops
being admittable — the failure lands on whichever pod rolls next, not on the edit.

`readOnlyRootFilesystem` is deliberately NOT here. It is not part of `restricted`,
and these four all write somewhere under / at runtime (Prometheus and Pyroscope to
their data dirs, Alloy to its storage path, the collector to nothing but its own
temp). The shared service chart sets it because first-party images are built to
tolerate it; adding it here would be a separate, per-image change.

The pod half — runAsNonRoot, runAsUser, seccompProfile — stays inline in each
template, because the UID each image actually ships with differs and the reason for
a given number belongs next to the image it applies to.
*/}}
{{- define "observability.containerSecurityContext" -}}
allowPrivilegeEscalation: false
capabilities:
  drop: ["ALL"]
{{- end }}

{{/*
The container security context for every first-party workload this chart renders,
identical in all four. Written once here because a copy that drifts is a namespace
that stops being admittable — and the failure lands on whichever pod rolls next,
not on the edit that caused it.

`readOnlyRootFilesystem` is not part of the `restricted` Pod Security Standard, but
it is the one thing on this list that limits what an already-compromised process can
do, and `ci:scan`'s misconfig gate requires it (KSV-0014). It holds here because all
four of these write only into mounts: Alloy to its storage path, Prometheus and
Pyroscope to their data dirs, the cluster collector to nothing at all — each an
emptyDir, which stays writable under a read-only root.

The pod half — runAsNonRoot, runAsUser, seccompProfile — stays inline in each
template, because the UID each image ships with differs and the reason for a given
number belongs next to the image it applies to.
*/}}
{{- define "observability.containerSecurityContext" -}}
allowPrivilegeEscalation: false
readOnlyRootFilesystem: true
capabilities:
  drop: ["ALL"]
{{- end }}

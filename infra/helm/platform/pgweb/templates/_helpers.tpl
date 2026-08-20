{{/*
The DSN, and where its password comes from.

An environment that keeps the read-only role's password in a Secret (every
deployed one — ADR-0202) cannot put the DSN in a values file, because the DSN
CONTAINS the password. So the URL is assembled from parts here and the password
arrives as an env var, which both containers below expand.

`databaseUrl` remains for the local tier, whose read-only password is a throwaway
committed beside it. An empty one is not a default: pg_isready reads an empty
`-d` as "connect over the local socket", so the init container waits forever on
`/var/run/postgresql:5432` and nothing in the message mentions Postgres being
somewhere else.
*/}}
{{- define "pgweb.dsnEnv" -}}
{{- $pg := .Values.pgweb -}}
{{- if $pg.databaseUrl }}
- { name: PGWEB_DATABASE_URL, value: {{ $pg.databaseUrl | quote }} }
{{- else }}
{{- $db := required "pgweb: set databaseUrl, or database.host and database.name" $pg.database }}
- name: PGWEB_DB_PASSWORD
  valueFrom:
    secretKeyRef: { name: {{ $db.passwordSecret }}, key: {{ $db.passwordKey }} }
- name: PGWEB_DATABASE_URL
  value: "postgres://{{ $db.user }}:$(PGWEB_DB_PASSWORD)@{{ required "pgweb.database.host" $db.host }}:{{ $db.port }}/{{ required "pgweb.database.name" $db.name }}?sslmode={{ $db.sslMode }}"
{{- end }}
{{- end -}}

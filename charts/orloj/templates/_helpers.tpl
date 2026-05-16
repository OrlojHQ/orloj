{{/*
Expand the name of the chart.
*/}}
{{- define "orloj.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "orloj.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart label value.
*/}}
{{- define "orloj.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels.
*/}}
{{- define "orloj.labels" -}}
helm.sh/chart: {{ include "orloj.chart" . }}
{{ include "orloj.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels (shared base).
*/}}
{{- define "orloj.selectorLabels" -}}
app.kubernetes.io/name: {{ include "orloj.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Server selector labels.
*/}}
{{- define "orloj.server.selectorLabels" -}}
{{ include "orloj.selectorLabels" . }}
app.kubernetes.io/component: server
{{- end }}

{{/*
Worker selector labels.
*/}}
{{- define "orloj.worker.selectorLabels" -}}
{{ include "orloj.selectorLabels" . }}
app.kubernetes.io/component: worker
{{- end }}

{{/*
Server service account name.
*/}}
{{- define "orloj.server.serviceAccountName" -}}
{{- if .Values.server.serviceAccount.create }}
{{- default (printf "%s-server" (include "orloj.fullname" .)) .Values.server.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.server.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Worker service account name.
*/}}
{{- define "orloj.worker.serviceAccountName" -}}
{{- if .Values.worker.serviceAccount.create }}
{{- default (printf "%s-worker" (include "orloj.fullname" .)) .Values.worker.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.worker.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Server image reference.
*/}}
{{- define "orloj.server.image" -}}
{{- $tag := default .Chart.AppVersion .Values.image.server.tag -}}
{{- if .Values.image.registry -}}
{{- printf "%s/%s:%s" .Values.image.registry .Values.image.server.repository $tag }}
{{- else -}}
{{- printf "%s:%s" .Values.image.server.repository $tag }}
{{- end -}}
{{- end }}

{{/*
Worker image reference.
*/}}
{{- define "orloj.worker.image" -}}
{{- $tag := default .Chart.AppVersion .Values.image.worker.tag -}}
{{- if .Values.image.registry -}}
{{- printf "%s/%s:%s" .Values.image.registry .Values.image.worker.repository $tag }}
{{- else -}}
{{- printf "%s:%s" .Values.image.worker.repository $tag }}
{{- end -}}
{{- end }}

{{/*
Secret name — either user-provided or chart-managed.
*/}}
{{- define "orloj.secretName" -}}
{{- if .Values.existingSecret }}
{{- .Values.existingSecret }}
{{- else }}
{{- include "orloj.fullname" . }}
{{- end }}
{{- end }}

{{/*
PostgreSQL DSN. Resolves in order:
  1. externalPostgres.dsn (BYO database)
  2. Bitnami subchart service (when postgresql.enabled)
*/}}
{{- define "orloj.postgresDSN" -}}
{{- if .Values.externalPostgres.dsn }}
{{- .Values.externalPostgres.dsn }}
{{- else if .Values.postgresql.enabled }}
{{- $user := .Values.postgresql.auth.username -}}
{{- $db   := .Values.postgresql.auth.database -}}
{{- $host := printf "%s-postgresql" .Release.Name -}}
{{- printf "postgres://%s:$(POSTGRES_PASSWORD)@%s:5432/%s?sslmode=disable" $user $host $db }}
{{- end }}
{{- end }}

{{/*
Operator selector labels.
*/}}
{{- define "orloj.operator.selectorLabels" -}}
{{ include "orloj.selectorLabels" . }}
app.kubernetes.io/component: operator
{{- end }}

{{/*
Operator service account name.
*/}}
{{- define "orloj.operator.serviceAccountName" -}}
{{- printf "%s-operator" (include "orloj.fullname" .) }}
{{- end }}

{{/*
Operator image reference.
*/}}
{{- define "orloj.operator.image" -}}
{{- $tag := default .Chart.AppVersion .Values.operator.image.tag -}}
{{- if .Values.image.registry -}}
{{- printf "%s/%s:%s" .Values.image.registry .Values.operator.image.repository $tag }}
{{- else -}}
{{- printf "%s:%s" .Values.operator.image.repository $tag }}
{{- end -}}
{{- end }}

{{/*
NATS URL. Resolves in order:
  1. externalNats.url (BYO NATS)
  2. Subchart service (when nats.enabled)
*/}}
{{- define "orloj.natsURL" -}}
{{- if .Values.externalNats.url }}
{{- .Values.externalNats.url }}
{{- else if .Values.nats.enabled }}
{{- printf "nats://%s-nats:4222" .Release.Name }}
{{- end }}
{{- end }}

{{- define "orloj.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "orloj.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := include "orloj.name" . -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "orloj.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "orloj.selectorLabels" -}}
app.kubernetes.io/name: {{ include "orloj.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "orloj.labels" -}}
helm.sh/chart: {{ include "orloj.chart" . }}
{{ include "orloj.selectorLabels" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- with .Values.commonLabels }}
{{ toYaml . }}
{{- end }}
{{- end -}}

{{- define "orloj.postgresFullname" -}}
{{- printf "%s-postgres" (include "orloj.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "orloj.natsFullname" -}}
{{- printf "%s-nats" (include "orloj.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "orloj.orlojdFullname" -}}
{{- printf "%s-orlojd" (include "orloj.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "orloj.orlojworkerFullname" -}}
{{- printf "%s-orlojworker" (include "orloj.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "orloj.runtimeSecretName" -}}
{{- printf "%s-runtime-secrets" (include "orloj.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "orloj.runtimeConfigName" -}}
{{- printf "%s-runtime-config" (include "orloj.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

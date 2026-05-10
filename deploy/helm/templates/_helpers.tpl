{{/*
  Expand the name of the chart.
*/}}
{{- define "cnpg-ui.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
  Create a default fully qualified app name.
*/}}
{{- define "cnpg-ui.fullname" -}}
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
  Name of the credentials Secret.
  Can be overridden via .Values.auth.credentialsSecretName.
*/}}
{{- define "cnpg-ui.credentialsSecretName" -}}
{{- .Values.auth.credentialsSecretName | default "cnpg-ui-credentials" }}
{{- end }}

{{/*
  Common labels applied to all resources.
*/}}
{{- define "cnpg-ui.labels" -}}
helm.sh/chart: {{ include "cnpg-ui.name" . }}-{{ .Chart.Version | replace "+" "_" }}
app.kubernetes.io/name: {{ include "cnpg-ui.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
  Selector labels.
*/}}
{{- define "cnpg-ui.selectorLabels" -}}
app.kubernetes.io/name: {{ include "cnpg-ui.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

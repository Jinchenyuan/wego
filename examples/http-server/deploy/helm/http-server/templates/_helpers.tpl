{{- define "wego-http-server.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "wego-http-server.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name (include "wego-http-server.name" .) | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{- define "wego-http-server.selectorLabels" -}}
app.kubernetes.io/name: {{ include "wego-http-server.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

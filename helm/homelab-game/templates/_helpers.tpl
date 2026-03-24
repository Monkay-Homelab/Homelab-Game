{{/*
Fullname: release-name truncated to 63 chars.
*/}}
{{- define "homelab-game.fullname" -}}
{{- default .Chart.Name .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "homelab-game.labels" -}}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: homelab-game
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
{{- end }}

{{/*
Backend selector labels
*/}}
{{- define "homelab-game.backend.selectorLabels" -}}
app.kubernetes.io/name: homelab-game-backend
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Frontend selector labels
*/}}
{{- define "homelab-game.frontend.selectorLabels" -}}
app.kubernetes.io/name: homelab-game-frontend
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Image tag helper — defaults to Chart.appVersion
*/}}
{{- define "homelab-game.backendImage" -}}
{{ .Values.backend.image.repository }}:{{ .Values.backend.image.tag | default .Chart.AppVersion }}
{{- end }}

{{- define "homelab-game.frontendImage" -}}
{{ .Values.frontend.image.repository }}:{{ .Values.frontend.image.tag | default .Chart.AppVersion }}
{{- end }}

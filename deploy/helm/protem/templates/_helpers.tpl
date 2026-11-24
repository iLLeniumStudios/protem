{{- define "protem.fullname" -}}
{{- if contains .Chart.Name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "protem.selectorLabels" -}}
control-plane: controller-manager
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "protem.labels" -}}
{{ include "protem.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{- end -}}

{{- define "protem.serviceAccountName" -}}
{{ include "protem.fullname" . }}-controller-manager
{{- end -}}

{{- define "protem.defaultTag" -}}
v{{ .Chart.AppVersion }}
{{- end -}}

{{- define "protem.serviceAccountSubject" -}}
- kind: ServiceAccount
  name: {{ include "protem.serviceAccountName" . }}
  namespace: {{ .Release.Namespace }}
{{- end -}}

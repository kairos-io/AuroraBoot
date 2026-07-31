{{/*
Expand the name of the chart.
*/}}
{{- define "auroraboot.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "auroraboot.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "auroraboot.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "auroraboot.labels" -}}
helm.sh/chart: {{ include "auroraboot.chart" . }}
{{ include "auroraboot.selectorLabels" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
{{- end -}}

{{- define "auroraboot.selectorLabels" -}}
app.kubernetes.io/name: {{ include "auroraboot.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "auroraboot.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "auroraboot.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "auroraboot.builderNamespace" -}}
{{- default .Release.Namespace .Values.builder.namespace -}}
{{- end -}}

{{- define "auroraboot.registrationSecretName" -}}
{{- printf "%s-registration" (include "auroraboot.fullname" .) -}}
{{- end -}}

{{- define "auroraboot.tlsSecretName" -}}
{{- if .Values.ingress.tls.existingSecret -}}
{{- .Values.ingress.tls.existingSecret -}}
{{- else -}}
{{- printf "%s-tls" (include "auroraboot.fullname" .) -}}
{{- end -}}
{{- end -}}

{{/*
Fail early if host is not set. Every top-level template calls this so
we surface a single friendly error instead of a cascade of confusing
render failures.
*/}}
{{- define "auroraboot.requireHost" -}}
{{- if not .Values.host -}}
{{- fail "host is required: set .Values.host to the externally reachable hostname (e.g. auroraboot.example.com)" -}}
{{- end -}}
{{- end -}}

{{/*
The base URL derived from host. Uses https when the chart is managing
TLS on the Ingress (ingress.tls.enabled), http otherwise. Users whose
TLS is terminated upstream by an LB or reverse proxy without a chart-
managed cert should either set ingress.tls.enabled with existingSecret,
or override the injected URL via extraArgs (--url=...) until a
first-class url override is added.
*/}}
{{- define "auroraboot.url" -}}
{{- if .Values.ingress.tls.enabled -}}
{{- printf "https://%s" .Values.host -}}
{{- else -}}
{{- printf "http://%s" .Values.host -}}
{{- end -}}
{{- end -}}

{{/*
Cluster-internal URL that exporter pods PUT built artifacts to. Uses
the release Service's DNS so upload traffic never leaves the cluster;
callers who want to override can set builder.uploadURL explicitly.
*/}}
{{- define "auroraboot.uploadURL" -}}
{{- if .Values.builder.uploadURL -}}
{{- .Values.builder.uploadURL -}}
{{- else -}}
{{- printf "http://%s.%s.svc.cluster.local:%d" (include "auroraboot.fullname" .) .Release.Namespace (int .Values.service.port) -}}
{{- end -}}
{{- end -}}

{{/*
Resolves the registration token that ends up in the Secret. Both the
Secret template and the Deployment's checksum annotation use this,
so a token change ripples through the pod annotation and forces a
rolling restart on the next helm upgrade.
*/}}
{{- define "auroraboot.registrationToken" -}}
{{- if .Values.registrationToken.value -}}
{{- .Values.registrationToken.value -}}
{{- else -}}
{{- $existing := lookup "v1" "Secret" .Release.Namespace (include "auroraboot.registrationSecretName" .) -}}
{{- if and $existing $existing.data (hasKey $existing.data "token") -}}
{{- index $existing.data "token" | b64dec -}}
{{- else -}}
{{- randAlphaNum 48 -}}
{{- end -}}
{{- end -}}
{{- end -}}

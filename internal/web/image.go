package web

import (
	"bytes"
	"os"
	"path/filepath"
	"text/template"

	"github.com/kairos-io/AuroraBoot/internal/web/jobstorage"
)

func prepareDockerfile(job jobstorage.JobData, tempdir string) error {
	// Create a Dockerfile from a template
	tmpl := `FROM quay.io/kairos/kairos-init:v0.3.0 AS kairos-init

FROM {{.Image}} AS base

COPY --from=kairos-init /kairos-init /kairos-init
RUN /kairos-init -l debug -s install --version "{{.Version}}" -m "{{.Model}}" -v "{{.Variant}}" -t "{{.TrustedBoot}}"{{ if eq .Variant "standard" }} -k "{{.KubernetesDistribution}}" --k8sversion "{{.KubernetesVersion}}"{{ end }}
RUN /kairos-init -l debug -s init --version "{{.Version}}" -m "{{.Model}}" -v "{{.Variant}}" -t "{{.TrustedBoot}}"{{ if eq .Variant "standard" }} -k "{{.KubernetesDistribution}}" --k8sversion "{{.KubernetesVersion}}"{{ end }}
RUN /kairos-init -l debug --validate --version "{{.Version}}" -m "{{.Model}}" -v "{{.Variant}}" -t "{{.TrustedBoot}}"{{ if eq .Variant "standard" }} -k "{{.KubernetesDistribution}}" --k8sversion "{{.KubernetesVersion}}"{{ end }}
RUN rm /kairos-init`

	t, err := template.New("Interpolate Dockerfile content").Parse(tmpl)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	err = t.Execute(&buf, job)
	if err != nil {
		return err
	}

	err = os.WriteFile(filepath.Join(tempdir, "Dockerfile"), buf.Bytes(), 0644)
	if err != nil {
		return err
	}

	return nil
}

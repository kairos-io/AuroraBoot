package operator

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestUploadExporter_HasArtifactsMountAndUploadEnv(t *testing.T) {
	spec := uploadExporter("build-42")

	if want, got := corev1.RestartPolicyNever, spec.Template.Spec.RestartPolicy; want != got {
		t.Fatalf("RestartPolicy = %q, want %q", got, want)
	}
	if spec.BackoffLimit == nil {
		t.Fatalf("BackoffLimit is nil; want retries so a transient network blip does not fail the whole build")
	}
	if len(spec.Template.Spec.Containers) != 1 {
		t.Fatalf("Containers = %d, want 1", len(spec.Template.Spec.Containers))
	}
	c := spec.Template.Spec.Containers[0]

	var mountedArtifacts bool
	for _, vm := range c.VolumeMounts {
		if vm.Name == "artifacts" && vm.MountPath == "/artifacts" && vm.ReadOnly {
			mountedArtifacts = true
			break
		}
	}
	if !mountedArtifacts {
		t.Fatalf("container is missing the read-only /artifacts VolumeMount; the operator injects the volume but exporters must declare the mount")
	}

	var buildIDEnv string
	for _, env := range c.Env {
		if env.Name == "BUILD_ID" {
			buildIDEnv = env.Value
		}
	}
	if buildIDEnv != "build-42" {
		t.Fatalf("BUILD_ID env = %q, want %q", buildIDEnv, "build-42")
	}

	var envFromSecret string
	for _, ef := range c.EnvFrom {
		if ef.SecretRef != nil {
			envFromSecret = ef.SecretRef.Name
		}
	}
	if envFromSecret != uploadSecretName("build-42") {
		t.Fatalf("envFrom SecretRef = %q, want %q (upload URL and token)", envFromSecret, uploadSecretName("build-42"))
	}
}

package operator

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kairos-io/AuroraBoot/pkg/builder"
)

const (
	cloudConfigSecretKey = "cloud-config"
	dockerfileSecretKey  = "Dockerfile"

	buildIDLabel = "auroraboot.io/build-id"
)

func cloudConfigSecretName(id string) string { return id + "-cloud-config" }
func dockerfileSecretName(id string) string  { return id + "-dockerfile" }

// materializeSecrets returns the Secret objects that must exist alongside the
// OSArtifact CR for the operator to consume its cloud-config and Dockerfile
// inputs. Namespace and owner references are left for the caller to stamp,
// since the OSArtifact must be created first to have a UID.
func materializeSecrets(id, namespace string, opts builder.BuildOptions) []corev1.Secret {
	var out []corev1.Secret
	labels := map[string]string{buildIDLabel: id}

	if opts.CloudConfig != "" {
		out = append(out, corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cloudConfigSecretName(id),
				Namespace: namespace,
				Labels:    labels,
			},
			Data: map[string][]byte{
				cloudConfigSecretKey: []byte(opts.CloudConfig),
			},
		})
	}
	if opts.Dockerfile != "" {
		out = append(out, corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      dockerfileSecretName(id),
				Namespace: namespace,
				Labels:    labels,
			},
			Data: map[string][]byte{
				dockerfileSecretKey: []byte(opts.Dockerfile),
			},
		})
	}
	return out
}

package operator

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	buildv1alpha2 "github.com/kairos-io/kairos-operator/api/v1alpha2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/kairos-io/AuroraBoot/pkg/builder"
)

// ErrNotFound is returned when a build ID does not correspond to an OSArtifact
// in the target namespace. Wraps a sentinel so handlers can errors.Is-map it.
var ErrNotFound = errors.New("operator builder: build not found")

type Config struct {
	RESTConfig *rest.Config
	Namespace  string
}

type Builder struct {
	cfg Config
	k8s client.Client
}

// clientFactory lets tests swap in a fake controller-runtime client; production
// callers get the default which builds a real one from cfg.RESTConfig.
type clientFactory func(cfg Config, scheme *runtime.Scheme) (client.Client, error)

var defaultClientFactory clientFactory = func(cfg Config, scheme *runtime.Scheme) (client.Client, error) {
	return client.New(cfg.RESTConfig, client.Options{Scheme: scheme})
}

func New(cfg Config) (*Builder, error) {
	return newWithFactory(cfg, defaultClientFactory)
}

func newWithFactory(cfg Config, factory clientFactory) (*Builder, error) {
	if cfg.RESTConfig == nil {
		return nil, errors.New("operator builder: RESTConfig is required")
	}
	if cfg.Namespace == "" {
		return nil, errors.New("operator builder: Namespace is required")
	}

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("register client-go scheme: %w", err)
	}
	if err := buildv1alpha2.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("register kairos-operator v1alpha2 scheme: %w", err)
	}

	k8s, err := factory(cfg, scheme)
	if err != nil {
		return nil, fmt.Errorf("build controller-runtime client: %w", err)
	}
	return &Builder{cfg: cfg, k8s: k8s}, nil
}

// Build submits an OSArtifact CR plus any inline-Secret inputs it references.
//
// AuroraBoot BuildOptions with no operator equivalent surface as
// ErrInvalidBuildOptions from translateBuildOptions (invalid arch, FIPS or
// TrustedBoot on a pre-built ref). Fields that the operator cannot consume at
// all (OverlayRootfs, BuildContextDir, Signing.*, Source.Variant,
// Source.AllowInsecureRegistries, Provisioning.*, KairosInitImage) are silently
// ignored today; they belong on the local backend and should be rejected by
// the handler before reaching Build once the UI exposes the backend choice.
func (b *Builder) Build(ctx context.Context, opts builder.BuildOptions) (*builder.BuildStatus, error) {
	id := opts.ID
	if id == "" {
		id = uuid.NewString()
	}

	spec, err := translateBuildOptions(id, opts)
	if err != nil {
		return nil, err
	}

	art := &buildv1alpha2.OSArtifact{
		ObjectMeta: metav1.ObjectMeta{
			Name:      id,
			Namespace: b.cfg.Namespace,
			Labels:    map[string]string{buildIDLabel: id},
		},
		Spec: spec,
	}
	if err := b.k8s.Create(ctx, art); err != nil {
		return nil, fmt.Errorf("create OSArtifact %q: %w", id, err)
	}

	// Owner refs bind Secret lifetime to the CR so kubectl delete on the
	// OSArtifact cleans the Secrets up. That requires the CR to already have
	// a UID, which is why we Create it first.
	for _, secret := range materializeSecrets(id, b.cfg.Namespace, opts) {
		secret := secret
		if err := controllerutil.SetOwnerReference(art, &secret, b.k8s.Scheme()); err != nil {
			return nil, fmt.Errorf("set owner on secret %q: %w", secret.Name, err)
		}
		if err := b.k8s.Create(ctx, &secret); err != nil {
			return nil, fmt.Errorf("create secret %q: %w", secret.Name, err)
		}
	}

	return &builder.BuildStatus{ID: id, Phase: builder.BuildPending}, nil
}

func (b *Builder) Status(ctx context.Context, id string) (*builder.BuildStatus, error) {
	art := &buildv1alpha2.OSArtifact{}
	err := b.k8s.Get(ctx, types.NamespacedName{Name: id, Namespace: b.cfg.Namespace}, art)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("%w: %q", ErrNotFound, id)
		}
		return nil, fmt.Errorf("get OSArtifact %q: %w", id, err)
	}
	return statusFromArtifact(art), nil
}

func (b *Builder) List(ctx context.Context) ([]*builder.BuildStatus, error) {
	sel, err := labels.Parse(buildIDLabel)
	if err != nil {
		return nil, fmt.Errorf("build label selector: %w", err)
	}
	list := &buildv1alpha2.OSArtifactList{}
	if err := b.k8s.List(ctx, list,
		client.InNamespace(b.cfg.Namespace),
		client.MatchingLabelsSelector{Selector: sel},
	); err != nil {
		return nil, fmt.Errorf("list OSArtifacts: %w", err)
	}
	out := make([]*builder.BuildStatus, 0, len(list.Items))
	for i := range list.Items {
		out = append(out, statusFromArtifact(&list.Items[i]))
	}
	return out, nil
}

func (b *Builder) Cancel(ctx context.Context, id string) error {
	art := &buildv1alpha2.OSArtifact{
		ObjectMeta: metav1.ObjectMeta{Name: id, Namespace: b.cfg.Namespace},
	}
	if err := b.k8s.Delete(ctx, art); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("delete OSArtifact %q: %w", id, err)
	}
	return nil
}

// statusFromArtifact folds the operator's five-phase model into AuroraBoot's
// four-phase model (Exporting collapses into Building), and defaults an empty
// phase (fresh CR the controller has not seen) to Pending. Artifacts is left
// empty; retrieval from the operator's PVC is tracked in plan section 3.3.
func statusFromArtifact(art *buildv1alpha2.OSArtifact) *builder.BuildStatus {
	phase := builder.BuildPending
	switch art.Status.Phase {
	case "", buildv1alpha2.Pending:
		phase = builder.BuildPending
	case buildv1alpha2.Building, buildv1alpha2.Exporting:
		phase = builder.BuildBuilding
	case buildv1alpha2.Ready:
		phase = builder.BuildReady
	case buildv1alpha2.Error:
		phase = builder.BuildError
	}
	return &builder.BuildStatus{
		ID:      art.Name,
		Phase:   phase,
		Message: art.Status.Message,
	}
}

var _ builder.ArtifactBuilder = (*Builder)(nil)

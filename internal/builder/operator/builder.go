package operator

import (
	"context"
	"errors"
	"fmt"

	"github.com/kairos-io/AuroraBoot/pkg/builder"
	buildv1alpha2 "github.com/kairos-io/kairos-operator/api/v1alpha2"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Config struct {
	RESTConfig *rest.Config
	Namespace  string
}

type Builder struct {
	cfg Config
	k8s client.Client
}

func New(cfg Config) (*Builder, error) {
	if cfg.RESTConfig == nil {
		return nil, errors.New("operator builder: RESTConfig is required")
	}
	if cfg.Namespace == "" {
		return nil, errors.New("operator builder: Namespace is required")
	}
	scheme := runtime.NewScheme()
	if err := buildv1alpha2.AddToScheme(scheme); err != nil {
		return nil, err
	}
	k8s, err := client.New(cfg.RESTConfig, client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}
	return &Builder{cfg: cfg, k8s: k8s}, nil
}

var errNotImplemented = fmt.Errorf("%w: operator builder is a scaffold", builder.ErrNotSupported)

func (b *Builder) Build(ctx context.Context, opts builder.BuildOptions) (*builder.BuildStatus, error) {
	return nil, errNotImplemented
}

func (b *Builder) Status(ctx context.Context, id string) (*builder.BuildStatus, error) {
	return nil, errNotImplemented
}

func (b *Builder) List(ctx context.Context) ([]*builder.BuildStatus, error) {
	return nil, errNotImplemented
}

func (b *Builder) Cancel(ctx context.Context, id string) error {
	return errNotImplemented
}

var _ builder.ArtifactBuilder = (*Builder)(nil)

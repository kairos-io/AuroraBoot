package operator

import (
	"context"
	"errors"
	"fmt"

	"github.com/kairos-io/AuroraBoot/pkg/builder"
	"k8s.io/client-go/rest"
)

type Config struct {
	RESTConfig *rest.Config
	Namespace  string
}

type Builder struct {
	cfg Config
}

func New(cfg Config) (*Builder, error) {
	if cfg.RESTConfig == nil {
		return nil, errors.New("operator builder: RESTConfig is required")
	}
	if cfg.Namespace == "" {
		return nil, errors.New("operator builder: Namespace is required")
	}
	return &Builder{cfg: cfg}, nil
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

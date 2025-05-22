package ops

import (
	"context"
	"fmt"

	"github.com/kairos-io/AuroraBoot/internal"
	"github.com/kairos-io/kairos-agent/v2/pkg/elemental"
	v1 "github.com/kairos-io/kairos-agent/v2/pkg/types/v1"
)

// DumpSource pulls a container image either remotely or locally from a docker daemon
// or simply copies the directory to the destination.
// Supports these prefixes:
// https://github.com/kairos-io/kairos-agent/blob/1e81cdef38677c8a36cae50d3334559976f66481/pkg/types/v1/common.go#L30-L33
func DumpSource(image, dst string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		cfg := NewConfig(
			WithImageExtractor(v1.OCIImageExtractor{}),
			WithLogger(internal.Log),
		)
		e := elemental.NewElemental(cfg)

		imgSource, err := v1.NewSrcFromURI(image)
		if err != nil {
			return err
		}
		if _, err := e.DumpSource(dst, imgSource); err != nil {
			return fmt.Errorf("dumping the source image %s to %s: %w", image, dst, err)
		}

		return nil
	}
}

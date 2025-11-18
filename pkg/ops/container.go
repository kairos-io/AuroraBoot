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
func DumpSource(image string, dstFunc valueGetOnCall, arch string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		dst := dstFunc()
		if image == "" {
			return fmt.Errorf("image source is empty, cannot dump to %s", dst)
		}
		internal.Log.Logger.Debug().Str("arch", arch).Msg("DumpSource: arch parameter")
		opts := []GenericOptions{
			WithImageExtractor(v1.OCIImageExtractor{}),
			WithLogger(internal.Log),
		}
		if arch != "" {
			opts = append(opts, WithArch(arch))
		}
		cfg := NewConfig(opts...)
		if cfg != nil {
			internal.Log.Logger.Debug().Str("arch", cfg.Arch).Msg("DumpSource: config arch after NewConfig")
			if cfg.Platform != nil {
				internal.Log.Logger.Debug().Str("platform", cfg.Platform.String()).Msg("DumpSource: config platform after NewConfig")
			} else {
				internal.Log.Logger.Debug().Msg("DumpSource: config platform is nil after NewConfig")
			}
		}
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

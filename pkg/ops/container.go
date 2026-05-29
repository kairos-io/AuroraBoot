package ops

import (
	"context"
	"fmt"

	"github.com/kairos-io/AuroraBoot/internal"
	"github.com/kairos-io/kairos-agent/v2/pkg/elemental"
	"github.com/kairos-io/kairos-agent/v2/pkg/implementations/imageextractor"
	sdkImage "github.com/kairos-io/kairos-sdk/types/images"
	"github.com/kairos-io/kairos-sdk/utils"
	imageutils "github.com/kairos-io/kairos-sdk/utils/image"
)

// insecureImageExtractor pulls OCI images allowing plain-HTTP registries and
// registries serving untrusted/self-signed TLS certificates. It implements
// kairos-sdk's imagetypes.ImageExtractor so it can be plugged into elemental
// via WithImageExtractor, mirroring kairos-agent's OCIImageExtractor but
// passing the insecure option down to the SDK.
type insecureImageExtractor struct{}

var _ sdkImage.ImageExtractor = insecureImageExtractor{}

func (e insecureImageExtractor) ExtractImage(imageRef, destination, platformRef string, excludes ...string) error {
	if platformRef == "" {
		platformRef = utils.GetCurrentPlatform()
	}
	img, err := imageutils.GetImage(imageRef, platformRef, nil, nil, imageutils.WithInsecureRegistry())
	if err != nil {
		return err
	}
	return imageutils.ExtractOCIImage(img, destination, excludes...)
}

func (e insecureImageExtractor) GetOCIImageSize(imageRef, platformRef string) (int64, error) {
	if platformRef == "" {
		platformRef = utils.GetCurrentPlatform()
	}
	return imageutils.GetOCIImageSize(imageRef, platformRef, nil, nil, imageutils.WithInsecureRegistry())
}

// DumpSource pulls a container image either remotely or locally from a docker daemon
// or simply copies the directory to the destination.
// When insecure is true, pulls are allowed from plain-HTTP registries and from
// registries presenting untrusted/self-signed TLS certificates.
// Supports these prefixes:
// https://github.com/kairos-io/kairos-agent/blob/1e81cdef38677c8a36cae50d3334559976f66481/pkg/types/v1/common.go#L30-L33
func DumpSource(image string, dstFunc valueGetOnCall, arch string, insecure bool) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		dst := dstFunc()
		if image == "" {
			return fmt.Errorf("image source is empty, cannot dump to %s", dst)
		}
		internal.Log.Logger.Debug().Str("arch", arch).Bool("insecure", insecure).Msg("DumpSource: arch parameter")

		var extractor sdkImage.ImageExtractor = imageextractor.OCIImageExtractor{}
		if insecure {
			extractor = insecureImageExtractor{}
		}
		opts := []GenericOptions{
			WithImageExtractor(extractor),
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

		imgSource, err := sdkImage.NewSrcFromURI(image)
		if err != nil {
			return err
		}
		if _, err := e.DumpSource(dst, imgSource); err != nil {
			return fmt.Errorf("dumping the source image %s to %s: %w", image, dst, err)
		}

		return nil
	}
}

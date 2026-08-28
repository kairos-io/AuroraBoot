// Package extensions resolves catalog entries into local system extension files.
package extensions

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
	sdkextensions "github.com/kairos-io/kairos/v4/sdk/extensions"
)

const rawMediaType types.MediaType = "application/vnd.kairos.sysext.raw"

var requestPart = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)

// Request identifies a named extension and an optional catalog version.
type Request struct {
	Name    string
	Version string
}

// ParseRequest parses name or name@version.
func ParseRequest(value string) (Request, error) {
	parts := strings.Split(value, "@")
	if len(parts) > 2 || len(parts) == 0 || !requestPart.MatchString(parts[0]) {
		return Request{}, fmt.Errorf("invalid extension request %q", value)
	}
	request := Request{Name: parts[0]}
	if len(parts) == 2 {
		if !requestPart.MatchString(parts[1]) {
			return Request{}, fmt.Errorf("invalid extension request %q", value)
		}
		request.Version = parts[1]
	}
	return request, nil
}

// Materialize resolves requests and writes their raw OCI layers to destination.
func Materialize(ctx context.Context, catalogSource string, requests []Request, architecture string, destination string, insecure bool) ([]string, error) {
	if len(requests) == 0 {
		return nil, nil
	}

	seen := make(map[string]struct{}, len(requests))
	for _, request := range requests {
		parsed, err := ParseRequest(request.Name)
		if err != nil || parsed.Version != "" || (request.Version != "" && !requestPart.MatchString(request.Version)) {
			return nil, fmt.Errorf("invalid extension request %q", formatRequest(request))
		}
		if _, exists := seen[request.Name]; exists {
			return nil, fmt.Errorf("duplicate extension name %q", request.Name)
		}
		seen[request.Name] = struct{}{}
	}
	if architecture == "" {
		return nil, fmt.Errorf("extension architecture must not be empty")
	}

	reader, err := openCatalog(ctx, catalogSource)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	catalog, err := sdkextensions.Parse(reader)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(destination, 0o755); err != nil {
		return nil, fmt.Errorf("create extension destination: %w", err)
	}

	temporary := make([]string, 0, len(requests))
	outputs := make([]string, 0, len(requests))
	cleanup := func() {
		for _, path := range temporary {
			_ = os.Remove(path)
		}
	}
	defer cleanup()

	for _, request := range requests {
		resolved, resolveErr := catalog.Resolve(request.Name, request.Version, architecture)
		if resolveErr != nil {
			return nil, fmt.Errorf("resolve extension %q: %w", request.Name, resolveErr)
		}
		options := []name.Option{}
		if insecure {
			options = append(options, name.Insecure)
		}
		reference, parseErr := name.NewDigest(resolved.OCI, options...)
		if parseErr != nil {
			return nil, fmt.Errorf("parse OCI reference for extension %q: %w", request.Name, parseErr)
		}
		image, pullErr := remote.Image(reference, remote.WithContext(ctx))
		if pullErr != nil {
			return nil, fmt.Errorf("pull extension %q: %w", request.Name, pullErr)
		}
		layers, layersErr := image.Layers()
		if layersErr != nil {
			return nil, fmt.Errorf("read layers for extension %q: %w", request.Name, layersErr)
		}
		if len(layers) != 1 {
			return nil, fmt.Errorf("extension %q has %d layers, expected exactly one", request.Name, len(layers))
		}
		mediaType, mediaErr := layers[0].MediaType()
		if mediaErr != nil {
			return nil, fmt.Errorf("read layer media type for extension %q: %w", request.Name, mediaErr)
		}
		if mediaType != rawMediaType {
			return nil, fmt.Errorf("extension %q layer media type is %q, expected %q", request.Name, mediaType, rawMediaType)
		}
		stream, streamErr := layers[0].Compressed()
		if streamErr != nil {
			return nil, fmt.Errorf("read extension %q layer: %w", request.Name, streamErr)
		}
		temp, tempErr := os.CreateTemp(destination, "."+request.Name+"-*.sysext.raw")
		if tempErr != nil {
			stream.Close()
			return nil, fmt.Errorf("create temporary file for extension %q: %w", request.Name, tempErr)
		}
		temporary = append(temporary, temp.Name())
		_, copyErr := io.Copy(temp, stream)
		streamErr = stream.Close()
		closeErr := temp.Close()
		if copyErr != nil || streamErr != nil || closeErr != nil {
			return nil, fmt.Errorf("write extension %q: %w", request.Name, firstError(copyErr, streamErr, closeErr))
		}
		outputs = append(outputs, filepath.Join(destination, request.Name+".sysext.raw"))
	}

	for index, path := range temporary {
		if err := os.Rename(path, outputs[index]); err != nil {
			for _, output := range outputs[:index] {
				_ = os.Remove(output)
			}
			return nil, fmt.Errorf("install extension %q: %w", requests[index].Name, err)
		}
	}
	return outputs, nil
}

func openCatalog(ctx context.Context, source string) (io.ReadCloser, error) {
	parsed, err := url.Parse(source)
	if err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") {
		request, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
		if requestErr != nil {
			return nil, fmt.Errorf("create catalog request: %w", requestErr)
		}
		response, responseErr := http.DefaultClient.Do(request)
		if responseErr != nil {
			return nil, fmt.Errorf("load extension catalog: %w", responseErr)
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			response.Body.Close()
			return nil, fmt.Errorf("load extension catalog: HTTP status %s", response.Status)
		}
		return response.Body, nil
	}
	file, openErr := os.Open(source)
	if openErr != nil {
		return nil, fmt.Errorf("load extension catalog: %w", openErr)
	}
	return file, nil
}

func formatRequest(request Request) string {
	if request.Version == "" {
		return request.Name
	}
	return request.Name + "@" + request.Version
}

func firstError(errors ...error) error {
	for _, err := range errors {
		if err != nil {
			return err
		}
	}
	return nil
}

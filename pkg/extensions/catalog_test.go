package extensions

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	ggcrregistry "github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

func TestParseRequest(t *testing.T) {
	tests := []struct {
		value string
		want  Request
		ok    bool
	}{
		{value: "tool", want: Request{Name: "tool"}, ok: true},
		{value: "tool@v1", want: Request{Name: "tool", Version: "v1"}, ok: true},
		{value: "", ok: false},
		{value: "@v1", ok: false},
		{value: "tool@", ok: false},
		{value: "tool@v1@extra", ok: false},
		{value: " tool", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			got, err := ParseRequest(tt.value)
			if (err == nil) != tt.ok {
				t.Fatalf("ParseRequest() error = %v, want success %v", err, tt.ok)
			}
			if got != tt.want {
				t.Fatalf("ParseRequest() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestMaterialize(t *testing.T) {
	registry := httptest.NewServer(ggcrregistry.New())
	t.Cleanup(registry.Close)
	repository := strings.TrimPrefix(registry.URL, "http://") + "/extensions/tool"
	digest := pushImage(t, repository, []layerSpec{{data: "raw extension", mediaType: rawMediaType}})
	catalog := writeCatalog(t, repository+"@"+digest)
	destination := t.TempDir()

	paths, err := Materialize(context.Background(), catalog, []Request{{Name: "tool"}}, "amd64", destination, true)
	if err != nil {
		t.Fatalf("Materialize() error = %v", err)
	}
	if len(paths) != 1 || paths[0] != filepath.Join(destination, "tool.sysext.raw") {
		t.Fatalf("Materialize() paths = %v", paths)
	}
	data, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "raw extension" {
		t.Fatalf("materialized data = %q", data)
	}
}

func TestMaterializeLoadsHTTPCatalogAndResolvesVersion(t *testing.T) {
	registry := httptest.NewServer(ggcrregistry.New())
	t.Cleanup(registry.Close)
	repository := strings.TrimPrefix(registry.URL, "http://") + "/extensions/tool"
	digest := pushImage(t, repository, []layerSpec{{data: "version two", mediaType: rawMediaType}})
	catalogData := catalogJSON(t, []catalogLayer{{name: "tool", latest: "v1", versions: map[string]string{"v2": repository + "@" + digest}}})
	catalogServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_, _ = response.Write(catalogData)
	}))
	t.Cleanup(catalogServer.Close)
	destination := t.TempDir()

	paths, err := Materialize(context.Background(), catalogServer.URL, []Request{{Name: "tool", Version: "v2"}}, "amd64", destination, true)
	if err != nil {
		t.Fatalf("Materialize() error = %v", err)
	}
	data, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "version two" {
		t.Fatalf("materialized data = %q", data)
	}
}

func TestMaterializeRejectsDuplicateNames(t *testing.T) {
	_, err := Materialize(context.Background(), "unused", []Request{{Name: "tool"}, {Name: "tool", Version: "v2"}}, "amd64", t.TempDir(), false)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("Materialize() error = %v, want duplicate error", err)
	}
}

func TestMaterializeRejectsUnexpectedLayerCount(t *testing.T) {
	registry := httptest.NewServer(ggcrregistry.New())
	t.Cleanup(registry.Close)
	repository := strings.TrimPrefix(registry.URL, "http://") + "/extensions/tool"
	digest := pushImage(t, repository, []layerSpec{
		{data: "one", mediaType: rawMediaType},
		{data: "two", mediaType: rawMediaType},
	})
	catalog := writeCatalog(t, repository+"@"+digest)

	_, err := Materialize(context.Background(), catalog, []Request{{Name: "tool"}}, "amd64", t.TempDir(), true)
	if err == nil || !strings.Contains(err.Error(), "expected exactly one") {
		t.Fatalf("Materialize() error = %v, want layer count error", err)
	}
}

func TestMaterializeCleansTemporaryFilesOnFailure(t *testing.T) {
	registry := httptest.NewServer(ggcrregistry.New())
	t.Cleanup(registry.Close)
	registryHost := strings.TrimPrefix(registry.URL, "http://")
	validRepository := registryHost + "/extensions/valid"
	invalidRepository := registryHost + "/extensions/invalid"
	validDigest := pushImage(t, validRepository, []layerSpec{{data: "valid", mediaType: rawMediaType}})
	invalidDigest := pushImage(t, invalidRepository, []layerSpec{{data: "wrong", mediaType: "application/octet-stream"}})
	catalog := writeCatalogLayers(t, []catalogLayer{
		{name: "valid", latest: "v1", versions: map[string]string{"v1": validRepository + "@" + validDigest}},
		{name: "invalid", latest: "v1", versions: map[string]string{"v1": invalidRepository + "@" + invalidDigest}},
	})
	destination := t.TempDir()

	_, err := Materialize(context.Background(), catalog, []Request{{Name: "valid"}, {Name: "invalid"}}, "amd64", destination, true)
	if err == nil || !strings.Contains(err.Error(), "media type") {
		t.Fatalf("Materialize() error = %v, want media type error", err)
	}
	entries, readErr := os.ReadDir(destination)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("destination contains files after failure: %v", entries)
	}
}

type layerSpec struct {
	data      string
	mediaType types.MediaType
}

func pushImage(t *testing.T, repository string, specs []layerSpec) string {
	t.Helper()
	image := empty.Image
	for _, spec := range specs {
		var err error
		image, err = mutate.AppendLayers(image, static.NewLayer([]byte(spec.data), spec.mediaType))
		if err != nil {
			t.Fatal(err)
		}
	}
	tag, err := name.NewTag(repository+":test", name.Insecure)
	if err != nil {
		t.Fatal(err)
	}
	if err := remote.Write(tag, image); err != nil {
		t.Fatal(err)
	}
	digest, err := image.Digest()
	if err != nil {
		t.Fatal(err)
	}
	return digest.String()
}

func writeCatalog(t *testing.T, oci string) string {
	t.Helper()
	return writeCatalogLayers(t, []catalogLayer{{name: "tool", latest: "v1", versions: map[string]string{"v1": oci}}})
}

type catalogLayer struct {
	name     string
	latest   string
	versions map[string]string
}

func writeCatalogLayers(t *testing.T, layers []catalogLayer) string {
	t.Helper()
	data := catalogJSON(t, layers)
	path := filepath.Join(t.TempDir(), "catalog.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func catalogJSON(t *testing.T, layers []catalogLayer) []byte {
	t.Helper()
	catalogLayers := make([]any, 0, len(layers))
	for _, layer := range layers {
		tags := make([]any, 0, len(layer.versions))
		for version, oci := range layer.versions {
			tags = append(tags, map[string]any{
				"tag": version, "sysext": map[string]any{"amd64": map[string]string{"oci": oci}},
			})
		}
		catalogLayers = append(catalogLayers, map[string]any{"name": layer.name, "latest": layer.latest, "tags": tags})
	}
	catalog := map[string]any{
		"repo": "test", "layers": catalogLayers,
	}
	data, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

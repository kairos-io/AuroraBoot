package hadron

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// Catalog fetches the browse-able list of hadron base tags, firmware images
// and software layers from upstream. Results are memoized with a short TTL so
// UI list refreshes don't pound the ghcr token endpoint or the GH API; on any
// fetch error we serve the last-good response rather than a blank list, so a
// transient upstream hiccup doesn't blank the picker mid-wizard.
type Catalog struct {
	// HTTP is the client used for every outbound call. Tests inject an
	// httptest.Server-backed client; production uses a 15s-timeout http.Client.
	HTTP *http.Client
	// BaseTagsURL overrides the ghcr.io v2 tag-list endpoint. Empty means the
	// well-known kairos-io/hadron URL.
	BaseTagsURL string
	// BaseTagsTokenURL overrides the ghcr.io anonymous-token endpoint.
	BaseTagsTokenURL string
	// FirmwareReleasesURL overrides the GH releases endpoint for
	// kairos-io/hadron-firmware.
	FirmwareReleasesURL string
	// LayersReleasesURL overrides the hadron-layers pages releases.json.
	LayersReleasesURL string
	// TTL is how long a cached response is served before a re-fetch. Zero
	// means the DefaultTTL.
	TTL time.Duration

	mu               sync.Mutex
	baseCache        cacheEntry[[]string]
	firmwareCache    cacheEntry[[]FirmwareItem]
	layersCache      cacheEntry[[]LayerItem]
}

// DefaultTTL is the cache lifetime for catalog entries — long enough that a UI
// refresh flurry serves out of cache, short enough that a new upstream release
// shows up on the same day.
const DefaultTTL = time.Hour

// FirmwareItem is one addressable firmware image target derived from the
// hadron-firmware GH release assets. Multiple firmware images share the same
// release tag (the linux-firmware upstream version), so we deduplicate by
// target name and let the UI show them as a flat list.
type FirmwareItem struct {
	Name       string `json:"name"`       // e.g. "linux-firmware-amdgpu"
	Image      string `json:"image"`      // e.g. "ghcr.io/kairos-io/hadron-firmware/linux-firmware-amdgpu"
	Version    string `json:"version"`    // e.g. "20260622"
	ReleaseTag string `json:"releaseTag"` // GH release tag the item was discovered under
}

// LayerItem is one hadron-layers software package (git, gpg, fwupd, ...).
// The upstream releases.json already carries the shape we want, so we mostly
// pass it through.
type LayerItem struct {
	Name        string   `json:"name"`
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Image       string   `json:"image"`
	Latest      string   `json:"latest,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// cacheEntry holds a single fetched value with its expiry. On a fetch error
// the entry is retained past its expiry (best-effort fallback) so the caller
// can serve stale data rather than 500ing a UI page.
type cacheEntry[T any] struct {
	value   T
	expires time.Time
	hasVal  bool
}

// NewCatalog returns a Catalog with the default upstream endpoints and a 15s
// HTTP client timeout. The TTL defaults to DefaultTTL when unset.
func NewCatalog() *Catalog {
	return &Catalog{
		HTTP: &http.Client{Timeout: 15 * time.Second},
	}
}

// httpClient returns the effective client, falling back to a default when the
// caller didn't set one.
func (c *Catalog) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func (c *Catalog) ttl() time.Duration {
	if c.TTL > 0 {
		return c.TTL
	}
	return DefaultTTL
}

// BaseVersions returns the tag list published under ghcr.io/kairos-io/hadron.
// Tags are returned in the order upstream reports them (registry v2 emits them
// oldest-first, which lets the UI show the head as "latest" and the tail as
// version history).
func (c *Catalog) BaseVersions(ctx context.Context) ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.baseCache.hasVal && time.Now().Before(c.baseCache.expires) {
		return append([]string(nil), c.baseCache.value...), nil
	}
	tags, err := c.fetchBaseVersions(ctx)
	if err != nil {
		if c.baseCache.hasVal {
			return append([]string(nil), c.baseCache.value...), nil
		}
		return nil, err
	}
	c.baseCache = cacheEntry[[]string]{value: tags, expires: time.Now().Add(c.ttl()), hasVal: true}
	return append([]string(nil), tags...), nil
}

// Firmware returns the deduplicated firmware image list, most-recent release
// first. Every firmware layer under the most recent release is emitted;
// operators pin to older versions by typing the ref explicitly in the UI.
func (c *Catalog) Firmware(ctx context.Context) ([]FirmwareItem, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.firmwareCache.hasVal && time.Now().Before(c.firmwareCache.expires) {
		return append([]FirmwareItem(nil), c.firmwareCache.value...), nil
	}
	items, err := c.fetchFirmware(ctx)
	if err != nil {
		if c.firmwareCache.hasVal {
			return append([]FirmwareItem(nil), c.firmwareCache.value...), nil
		}
		return nil, err
	}
	c.firmwareCache = cacheEntry[[]FirmwareItem]{value: items, expires: time.Now().Add(c.ttl()), hasVal: true}
	return append([]FirmwareItem(nil), items...), nil
}

// Layers returns the hadron-layers list from the upstream releases.json.
func (c *Catalog) Layers(ctx context.Context) ([]LayerItem, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.layersCache.hasVal && time.Now().Before(c.layersCache.expires) {
		return append([]LayerItem(nil), c.layersCache.value...), nil
	}
	items, err := c.fetchLayers(ctx)
	if err != nil {
		if c.layersCache.hasVal {
			return append([]LayerItem(nil), c.layersCache.value...), nil
		}
		return nil, err
	}
	c.layersCache = cacheEntry[[]LayerItem]{value: items, expires: time.Now().Add(c.ttl()), hasVal: true}
	return append([]LayerItem(nil), items...), nil
}

const (
	defaultBaseTagsURL      = "https://ghcr.io/v2/kairos-io/hadron/tags/list"
	defaultBaseTagsTokenURL = "https://ghcr.io/token?scope=repository:kairos-io/hadron:pull&service=ghcr.io"
	defaultFirmwareURL      = "https://api.github.com/repos/kairos-io/hadron-firmware/releases"
	defaultLayersURL        = "https://kairos-io.github.io/hadron-layers/releases.json"
)

// fetchBaseVersions resolves an anonymous ghcr bearer token and lists tags for
// kairos-io/hadron. Both hops are required because the docker registry v2 API
// rejects anonymous requests without a Bearer token.
func (c *Catalog) fetchBaseVersions(ctx context.Context) ([]string, error) {
	tokenURL := c.BaseTagsTokenURL
	if tokenURL == "" {
		tokenURL = defaultBaseTagsTokenURL
	}
	tagsURL := c.BaseTagsURL
	if tagsURL == "" {
		tagsURL = defaultBaseTagsURL
	}

	tokReq, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL, nil)
	if err != nil {
		return nil, fmt.Errorf("ghcr token request: %w", err)
	}
	tokResp, err := c.httpClient().Do(tokReq)
	if err != nil {
		return nil, fmt.Errorf("ghcr token fetch: %w", err)
	}
	defer tokResp.Body.Close()
	if tokResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ghcr token: unexpected status %d", tokResp.StatusCode)
	}
	var tok struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(tokResp.Body).Decode(&tok); err != nil {
		return nil, fmt.Errorf("ghcr token decode: %w", err)
	}
	if tok.Token == "" {
		return nil, fmt.Errorf("ghcr token: empty")
	}

	tagsReq, err := http.NewRequestWithContext(ctx, http.MethodGet, tagsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("ghcr tags request: %w", err)
	}
	tagsReq.Header.Set("Authorization", "Bearer "+tok.Token)
	tagsResp, err := c.httpClient().Do(tagsReq)
	if err != nil {
		return nil, fmt.Errorf("ghcr tags fetch: %w", err)
	}
	defer tagsResp.Body.Close()
	if tagsResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ghcr tags: unexpected status %d", tagsResp.StatusCode)
	}
	var payload struct {
		Name string   `json:"name"`
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(io.LimitReader(tagsResp.Body, 1<<20)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("ghcr tags decode: %w", err)
	}
	// Registry v2 emits tags in ascending order — reverse so newest-first
	// matches the UI's "latest at top" expectation. Alphabetic reverse works
	// well enough for the semver-ish tag scheme kairos uses without dragging
	// in a full semver parser.
	tags := append([]string(nil), payload.Tags...)
	sort.Sort(sort.Reverse(sort.StringSlice(tags)))
	return tags, nil
}

// fetchFirmware reads GH releases for hadron-firmware and turns each release's
// `.sysext.raw` asset list into a deduplicated list of firmware image refs.
// Assets are named `linux-firmware-<target>_<version>.sysext.raw`; the image
// ref lives at `ghcr.io/kairos-io/hadron-firmware/linux-firmware-<target>:<version>`.
func (c *Catalog) fetchFirmware(ctx context.Context) ([]FirmwareItem, error) {
	url := c.FirmwareReleasesURL
	if url == "" {
		url = defaultFirmwareURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("firmware releases request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("firmware releases fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("firmware releases: unexpected status %d", resp.StatusCode)
	}
	// Cap the response body — a runaway payload would blow up the server.
	// Each release page is small (few KB per release), 2 MiB is generous.
	var releases []struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name string `json:"name"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&releases); err != nil {
		return nil, fmt.Errorf("firmware releases decode: %w", err)
	}

	seen := map[string]bool{}
	items := []FirmwareItem{}
	for _, r := range releases {
		for _, a := range r.Assets {
			name, ver, ok := parseFirmwareAsset(a.Name, r.TagName)
			if !ok {
				continue
			}
			key := name + ":" + ver
			if seen[key] {
				continue
			}
			seen[key] = true
			items = append(items, FirmwareItem{
				Name:       name,
				Image:      "ghcr.io/kairos-io/hadron-firmware/" + name,
				Version:    ver,
				ReleaseTag: r.TagName,
			})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].ReleaseTag != items[j].ReleaseTag {
			return items[i].ReleaseTag > items[j].ReleaseTag
		}
		return items[i].Name < items[j].Name
	})
	return items, nil
}

// parseFirmwareAsset splits `linux-firmware-<target>_<version>.sysext.raw` into
// the image-name and version. Falls back to the release tag when the asset
// name doesn't carry a `_version` suffix (some releases just tag the tree).
func parseFirmwareAsset(assetName, releaseTag string) (string, string, bool) {
	const suffix = ".sysext.raw"
	if !strings.HasSuffix(assetName, suffix) {
		return "", "", false
	}
	stem := strings.TrimSuffix(assetName, suffix)
	idx := strings.LastIndex(stem, "_")
	if idx < 0 {
		if releaseTag == "" || stem == "" {
			return "", "", false
		}
		return stem, releaseTag, true
	}
	name := stem[:idx]
	ver := stem[idx+1:]
	if name == "" || ver == "" {
		return "", "", false
	}
	return name, ver, true
}

// fetchLayers reads the hadron-layers pages releases.json and flattens each
// layer into a LayerItem. The upstream payload already carries description,
// image ref, latest tag and tag history — we mostly pass it through.
func (c *Catalog) fetchLayers(ctx context.Context) ([]LayerItem, error) {
	url := c.LayersReleasesURL
	if url == "" {
		url = defaultLayersURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("layers request: %w", err)
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("layers fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("layers: unexpected status %d", resp.StatusCode)
	}
	var payload struct {
		Layers []struct {
			Name        string `json:"name"`
			Title       string `json:"title"`
			Description string `json:"description"`
			Image       string `json:"image"`
			Latest      string `json:"latest"`
			Tags        []struct {
				Tag string `json:"tag"`
			} `json:"tags"`
		} `json:"layers"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("layers decode: %w", err)
	}
	items := make([]LayerItem, 0, len(payload.Layers))
	for _, l := range payload.Layers {
		tags := make([]string, 0, len(l.Tags))
		for _, t := range l.Tags {
			if t.Tag != "" {
				tags = append(tags, t.Tag)
			}
		}
		items = append(items, LayerItem{
			Name:        l.Name,
			Title:       l.Title,
			Description: l.Description,
			Image:       l.Image,
			Latest:      l.Latest,
			Tags:        tags,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, nil
}

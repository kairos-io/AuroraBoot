package hadron

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// tagsServer stands in for ghcr's two-hop token+tags dance in tests.
func tagsServer(t *testing.T, token string, tags []string, hits *int32) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if hits != nil {
			atomic.AddInt32(hits, 1)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"token":"` + token + `"}`))
	})
	mux.HandleFunc("/v2/kairos-io/hadron/tags/list", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"name":"kairos-io/hadron","tags":` + jsonEncodeList(tags) + `}`))
	})
	return httptest.NewServer(mux)
}

func jsonEncodeList(tags []string) string {
	var b strings.Builder
	b.WriteString("[")
	for i, t := range tags {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("\"")
		b.WriteString(t)
		b.WriteString("\"")
	}
	b.WriteString("]")
	return b.String()
}

func TestCatalog_BaseVersions_SortedReverse(t *testing.T) {
	srv := tagsServer(t, "tok", []string{"v0.1.0", "v0.3.0", "v0.2.0", "main"}, nil)
	defer srv.Close()
	c := &Catalog{
		BaseTagsTokenURL: srv.URL + "/token",
		BaseTagsURL:      srv.URL + "/v2/kairos-io/hadron/tags/list",
	}
	got, err := c.BaseVersions(context.Background())
	if err != nil {
		t.Fatalf("BaseVersions: %v", err)
	}
	want := []string{"v0.3.0", "v0.2.0", "v0.1.0", "main"}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %v", got)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("index %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestCatalog_BaseVersions_CacheHitsAvoidsRefetch(t *testing.T) {
	var hits int32
	srv := tagsServer(t, "tok", []string{"v0.1.0"}, &hits)
	defer srv.Close()
	c := &Catalog{
		BaseTagsTokenURL: srv.URL + "/token",
		BaseTagsURL:      srv.URL + "/v2/kairos-io/hadron/tags/list",
		TTL:              time.Hour,
	}
	for i := 0; i < 5; i++ {
		if _, err := c.BaseVersions(context.Background()); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("expected 1 token fetch, got %d", got)
	}
}

func TestCatalog_BaseVersions_FailSoftServesCached(t *testing.T) {
	var hits int32
	srv := tagsServer(t, "tok", []string{"v0.1.0"}, &hits)
	c := &Catalog{
		BaseTagsTokenURL: srv.URL + "/token",
		BaseTagsURL:      srv.URL + "/v2/kairos-io/hadron/tags/list",
		TTL:              time.Nanosecond, // expire immediately after each fetch
	}
	// Prime cache from the live server.
	if _, err := c.BaseVersions(context.Background()); err != nil {
		t.Fatalf("prime: %v", err)
	}
	srv.Close() // upstream goes away
	// TTL has already expired; the fetcher will fail. We want the stale cache.
	got, err := c.BaseVersions(context.Background())
	if err != nil {
		t.Fatalf("expected fail-soft to succeed, got %v", err)
	}
	if len(got) != 1 || got[0] != "v0.1.0" {
		t.Fatalf("expected stale cache, got %v", got)
	}
}

func TestCatalog_Firmware_DedupesAndSorts(t *testing.T) {
	// Two releases, each with a couple of assets. Later release should sort
	// first; duplicate name/version pairs across releases must be deduped.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[
			{
				"tag_name": "20260622",
				"assets": [
					{"name": "linux-firmware-amdgpu_20260622.sysext.raw"},
					{"name": "linux-firmware-rtw88_20260622.sysext.raw"}
				]
			},
			{
				"tag_name": "20240101",
				"assets": [
					{"name": "linux-firmware-amdgpu_20240101.sysext.raw"},
					{"name": "readme.txt"}
				]
			}
		]`))
	}))
	defer srv.Close()
	c := &Catalog{FirmwareReleasesURL: srv.URL}
	items, err := c.Firmware(context.Background())
	if err != nil {
		t.Fatalf("Firmware: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 firmware items, got %d: %+v", len(items), items)
	}
	// 20260622 items sorted before 20240101 (ReleaseTag desc).
	if items[0].ReleaseTag != "20260622" || items[len(items)-1].ReleaseTag != "20240101" {
		t.Fatalf("unexpected sort: %+v", items)
	}
	// Within a release, name-ascending.
	if items[0].Name != "linux-firmware-amdgpu" || items[1].Name != "linux-firmware-rtw88" {
		t.Fatalf("expected amdgpu before rtw88, got %v %v", items[0].Name, items[1].Name)
	}
	// Image ref is derived from name.
	if items[0].Image != "ghcr.io/kairos-io/hadron-firmware/linux-firmware-amdgpu" {
		t.Fatalf("unexpected image ref: %q", items[0].Image)
	}
}

func TestCatalog_Layers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"layers": [
				{
					"name": "git",
					"title": "git",
					"description": "Git",
					"image": "ghcr.io/kairos-io/git",
					"latest": "2.55.0",
					"tags": [{"tag": "2.55.0"}, {"tag": "2.54.0"}]
				},
				{
					"name": "fwupd",
					"image": "ghcr.io/kairos-io/fwupd",
					"latest": "2.1.6",
					"tags": [{"tag": "2.1.6"}]
				}
			]
		}`))
	}))
	defer srv.Close()
	c := &Catalog{LayersReleasesURL: srv.URL}
	items, err := c.Layers(context.Background())
	if err != nil {
		t.Fatalf("Layers: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 layers, got %d", len(items))
	}
	// Alphabetic sort.
	if items[0].Name != "fwupd" || items[1].Name != "git" {
		t.Fatalf("expected fwupd before git, got %v %v", items[0].Name, items[1].Name)
	}
	if items[1].Latest != "2.55.0" || len(items[1].Tags) != 2 {
		t.Fatalf("layer git tags not passed through: %+v", items[1])
	}
}

func TestParseFirmwareAsset(t *testing.T) {
	cases := []struct {
		asset, tag, name, ver string
		ok                    bool
	}{
		{"linux-firmware-amdgpu_20260622.sysext.raw", "20260622", "linux-firmware-amdgpu", "20260622", true},
		{"readme.txt", "20260622", "", "", false},
		{"linux-firmware-amdgpu.sysext.raw", "20260622", "linux-firmware-amdgpu", "20260622", true},
		{"_20260622.sysext.raw", "20260622", "", "", false},
	}
	for _, tc := range cases {
		name, ver, ok := parseFirmwareAsset(tc.asset, tc.tag)
		if ok != tc.ok {
			t.Fatalf("%s: got ok=%v want %v", tc.asset, ok, tc.ok)
		}
		if ok && (name != tc.name || ver != tc.ver) {
			t.Fatalf("%s: got (%q,%q) want (%q,%q)", tc.asset, name, ver, tc.name, tc.ver)
		}
	}
}

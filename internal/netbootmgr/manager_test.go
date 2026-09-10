package netbootmgr

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestHostFromURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"whitespace only", "   ", ""},
		{"full url with port", "http://fleet.home.arpa:8090", "fleet.home.arpa"},
		{"full url without port", "https://fleet.home.arpa", "fleet.home.arpa"},
		{"url with path", "http://fleet.home.arpa:8080/ui", "fleet.home.arpa"},
		{"ipv4 url", "http://10.0.0.5:8080", "10.0.0.5"},
		{"ipv6 url strips brackets", "http://[fd00::1]:8080", "fd00::1"},
		// web.go documents --url as a URL, but an operator writing a bare host
		// should still get their host back rather than the wildcard.
		{"bare host and port", "fleet.home.arpa:8090", "fleet.home.arpa"},
		{"bare host", "fleet.home.arpa", "fleet.home.arpa"},
		{"scheme only", "http://", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hostFromURL(tc.in); got != tc.want {
				t.Errorf("hostFromURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestAdvertisedHostPrefersConfiguredURL(t *testing.T) {
	if got := advertisedHost("http://fleet.home.arpa:8080"); got != "fleet.home.arpa" {
		t.Errorf("advertisedHost = %q, want fleet.home.arpa", got)
	}
}

func TestAdvertisedHostFallsBackToALocalAddress(t *testing.T) {
	got := advertisedHost("")

	// A host with no up, non-loopback interface can only report the wildcard;
	// anywhere else the point of the fallback is that it does not.
	if localIPv4() == "" {
		if got != bindAddress {
			t.Errorf("advertisedHost = %q with no local address, want %q", got, bindAddress)
		}
		return
	}

	if got == bindAddress {
		t.Fatalf("advertisedHost = %q, want a routable local address", got)
	}
	ip := net.ParseIP(got)
	if ip == nil || ip.To4() == nil {
		t.Fatalf("advertisedHost = %q, want an IPv4 address", got)
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
		t.Errorf("advertisedHost = %q, want an address clients can reach", got)
	}
}

// fakeNetbootArtifacts lays out the three files Start insists on and returns the
// artifacts dir and artifact ID to pass it.
func fakeNetbootArtifacts(t *testing.T) (string, string) {
	t.Helper()
	artifactsDir := t.TempDir()
	artifactID := "a5ec6329-e099-4720-97cf-aef72fc961f7"
	netbootDir := filepath.Join(artifactsDir, artifactID, "netboot")
	if err := os.MkdirAll(netbootDir, 0755); err != nil {
		t.Fatalf("mkdir netboot dir: %v", err)
	}
	for _, name := range []string{"kairos-kernel", "kairos-initrd", "kairos.squashfs"} {
		if err := os.WriteFile(filepath.Join(netbootDir, name), []byte("x"), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return artifactsDir, artifactID
}

// stubAuroraboot puts an "auroraboot" that just blocks first on PATH, so Start
// gets a live child process without a real pixiecore.
func stubAuroraboot(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	stub := filepath.Join(dir, "auroraboot")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nsleep 60\n"), 0755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestStartReportsTheAdvertisedHostAndStillBindsTheWildcard(t *testing.T) {
	stubAuroraboot(t)
	artifactsDir, artifactID := fakeNetbootArtifacts(t)

	m := NewManager("http://fleet.home.arpa:8080")
	if err := m.Start(artifactsDir, artifactID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = m.Stop() }()

	s := m.GetStatus()
	if !s.Running {
		t.Fatal("Running = false after a successful Start")
	}
	if s.Address != bindAddress {
		t.Errorf("Address = %q, want the wildcard bind address %q", s.Address, bindAddress)
	}
	if s.AdvertisedAddress != "fleet.home.arpa" {
		t.Errorf("AdvertisedAddress = %q, want fleet.home.arpa", s.AdvertisedAddress)
	}
	// The netboot HTTP port is the manager's own, not the one in --url.
	if s.Port != "8090" {
		t.Errorf("Port = %q, want 8090", s.Port)
	}
}

func TestStartStillBindsTheWildcardWhenAdvertisingALocalAddress(t *testing.T) {
	stubAuroraboot(t)
	artifactsDir, artifactID := fakeNetbootArtifacts(t)

	m := NewManager("")
	if err := m.Start(artifactsDir, artifactID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = m.Stop() }()

	s := m.GetStatus()
	if s.Address != bindAddress {
		t.Errorf("Address = %q, want the wildcard bind address %q", s.Address, bindAddress)
	}
	if s.AdvertisedAddress != advertisedHost("") {
		t.Errorf("AdvertisedAddress = %q, want %q", s.AdvertisedAddress, advertisedHost(""))
	}
}

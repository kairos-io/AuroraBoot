package auroraboot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/kairos-io/AuroraBoot/pkg/builder"
	"github.com/kairos-io/AuroraBoot/pkg/store"
)

// ccSecret is a value a real cloud-config carries: a join token, a user
// password or an SSH key. It is long and distinctive so a substring search of
// the log cannot match it by accident, and it is NOT in LogRedactValues,
// because AuroraBoot never sees the secrets a user writes into their own
// document. That is the whole point: the header cannot rely on redaction.
const ccSecret = "s3cr3t-join-token-9f2ab41c7d"

const ccWithSecret = `#cloud-config
users:
- name: kairos
  passwd: ` + ccSecret + `
  ssh_authorized_keys:
  - ssh-ed25519 AAAAC3Nza` + ccSecret + ` operator@example
k3s:
  env:
    K3S_TOKEN: ` + ccSecret + `
`

// header renders what a build writes into its log for the given options. The
// build log has no levels: whatever this returns is appended to the artifact
// row and broadcast to every subscribed UI client.
func header(opts builder.BuildOptions) string {
	var buf bytes.Buffer
	writeBuildHeader(&buf, opts, "quay.io/kairos/ubuntu:24.04-core-amd64-generic-v3.6.0", "/out/build-1")
	return buf.String()
}

func TestBuildHeaderKeepsTheCloudConfigOutOfTheLog(t *testing.T) {
	got := header(builder.BuildOptions{ISO: true, CloudConfig: ccWithSecret})

	if strings.Contains(got, ccSecret) {
		t.Errorf("the cloud-config secret reached the build log:\n%s", got)
	}
	// The secret is the thing that must not leak, but a cloud-config is
	// sensitive as a document: usernames, host names and the shape of the
	// fleet config are all disclosure. Assert on lines unique to the
	// document, not only on the planted marker.
	for _, line := range []string{"ssh_authorized_keys", "K3S_TOKEN", "name: kairos", "#cloud-config"} {
		if strings.Contains(got, line) {
			t.Errorf("cloud-config content %q reached the build log:\n%s", line, got)
		}
	}
}

func TestBuildHeaderRecordsTheCloudConfigSizeAndDigest(t *testing.T) {
	got := header(builder.BuildOptions{ISO: true, CloudConfig: ccWithSecret})

	want := fmt.Sprintf("Cloud config: %d bytes, sha256:%x",
		len(ccWithSecret), sha256.Sum256([]byte(ccWithSecret)))
	if !strings.Contains(got, want) {
		t.Errorf("build log does not identify the cloud-config.\nwant line: %s\ngot:\n%s", want, got)
	}
}

// A digest is only worth logging if it tells two documents apart, and a size
// alone does not: two configs of equal length differing in one byte are a
// different build.
func TestBuildHeaderDistinguishesTwoConfigsOfEqualSize(t *testing.T) {
	a := header(builder.BuildOptions{CloudConfig: "#cloud-config\nhostname: alpha\n"})
	b := header(builder.BuildOptions{CloudConfig: "#cloud-config\nhostname: alphb\n"})

	if a == b {
		t.Errorf("two different cloud-configs of the same size produced the same log line:\n%s", a)
	}
}

func TestBuildHeaderSaysNothingWhenThereIsNoCloudConfig(t *testing.T) {
	got := header(builder.BuildOptions{ISO: true})

	if strings.Contains(got, "Cloud config") {
		t.Errorf("build with no cloud-config still logged one:\n%s", got)
	}
}

// The header is the only place these are reported, so the refactor that moved
// it out of run() has to keep reporting them.
func TestBuildHeaderStillReportsTheBuildItself(t *testing.T) {
	got := header(builder.BuildOptions{ISO: true, CloudImage: true, Netboot: true})

	for _, want := range []string{
		"=== Starting AuroraBoot build ===",
		"Image: quay.io/kairos/ubuntu:24.04-core-amd64-generic-v3.6.0",
		"Output: ISO",
		"Output: Cloud Image (raw disk)",
		"Output: Netboot",
		"Output dir: /out/build-1",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("build log lost %q:\n%s", want, got)
		}
	}
}

// capturingStore records what the build log writer persists. Only AppendLog is
// implemented: the embedded nil interface makes any other call panic, so this
// fake cannot quietly absorb a method the writer was not supposed to reach.
type capturingStore struct {
	store.ArtifactStore
	mu  sync.Mutex
	log strings.Builder
}

func (s *capturingStore) AppendLog(_ context.Context, _ string, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.log.WriteString(text)
	return nil
}

func (s *capturingStore) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.log.String()
}

// capturingBroadcaster stands in for the UI hub every connected browser is
// subscribed to.
type capturingBroadcaster struct {
	mu     sync.Mutex
	chunks strings.Builder
}

func (b *capturingBroadcaster) BroadcastLogChunk(_ string, chunk string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.chunks.WriteString(chunk)
}

func (b *capturingBroadcaster) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.chunks.String()
}

// The epic's acceptance criterion, through the writer a build actually uses:
// the header goes to the same dbLogWriter that persists to the artifact row
// and fans out to the UI, so assert on both sinks rather than on a buffer.
func TestStoredAndStreamedLogNeverCarryTheCloudConfig(t *testing.T) {
	st := &capturingStore{}
	bc := &capturingBroadcaster{}
	w := &dbLogWriter{store: st, id: "build-1", ctx: context.Background(), broadcaster: bc,
		// Exactly what the Create handler knows: the secrets AuroraBoot
		// injected. The user's own cloud-config secret is absent, so
		// redaction cannot be what saves us here.
		redactValues: []string{"reg-token-aaaaaaaaaaaa", "node-password-bbbbbbbb"}}

	writeBuildHeader(w, builder.BuildOptions{ISO: true, CloudConfig: ccWithSecret},
		"quay.io/kairos/ubuntu:24.04-core-amd64-generic-v3.6.0", "/out/build-1")
	w.Flush()

	for name, got := range map[string]string{"stored": st.String(), "streamed": bc.String()} {
		if got == "" {
			t.Fatalf("%s log is empty, the writer was not exercised", name)
		}
		if strings.Contains(got, ccSecret) {
			t.Errorf("%s log carries the cloud-config secret:\n%s", name, got)
		}
		if !strings.Contains(got, "Cloud config: ") {
			t.Errorf("%s log does not record that a cloud-config was supplied:\n%s", name, got)
		}
	}
}

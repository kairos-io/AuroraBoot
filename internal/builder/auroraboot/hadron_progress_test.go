package auroraboot

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kairos-io/AuroraBoot/pkg/store"
)

// memArtifactStore is a minimal in-memory ArtifactStore for the progress
// parser test. It records the highest Progress value ever written so the
// test can assert throttling worked without wall-clock racing.
type memArtifactStore struct {
	mu       sync.Mutex
	rec      store.ArtifactRecord
	updates  int
	maxSeen  int
	appended []string
}

func (m *memArtifactStore) Create(_ context.Context, r *store.ArtifactRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rec = *r
	return nil
}
func (m *memArtifactStore) GetByID(_ context.Context, id string) (*store.ArtifactRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := m.rec
	cp.ID = id
	return &cp, nil
}
func (m *memArtifactStore) List(_ context.Context) ([]*store.ArtifactRecord, error) {
	return nil, nil
}
func (m *memArtifactStore) Update(_ context.Context, r *store.ArtifactRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rec = *r
	m.updates++
	if r.Progress > m.maxSeen {
		m.maxSeen = r.Progress
	}
	return nil
}
func (m *memArtifactStore) Delete(_ context.Context, _ string) error { return nil }
func (m *memArtifactStore) DeleteByPhase(_ context.Context, _ string) error {
	return nil
}
func (m *memArtifactStore) GetLogs(_ context.Context, _ string) (string, error) {
	return "", nil
}
func (m *memArtifactStore) AppendLog(_ context.Context, _ string, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.appended = append(m.appended, text)
	return nil
}

// fakeClock returns a monotonically increasing time from a controlled base
// so throttle logic is deterministic.
type fakeClock struct {
	mu   sync.Mutex
	now  time.Time
	step time.Duration
}

func (c *fakeClock) tick() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(c.step)
	return c.now
}

func TestHadronProgressParser_ExtractsMaxFraction(t *testing.T) {
	// A realistic-ish buildkit fragment. Only the stage-0 markers matter;
	// the other lines are noise the sniffer must ignore.
	log := strings.Join([]string{
		"#1 [internal] load build definition",
		"#2 [auth] library/alpine:pull token",
		"#7 [stage-0 1/17] FROM docker.io/library/alpine",
		"#8 [stage-0 2/17] RUN apk add --no-cache curl",
		"noise line that should not match #99 [stage-1 5/5]",
		"#9 [stage-0 5/17] RUN ...",
		"#10 [stage-0 10/17] RUN ...",
		"#11 [stage-0 17/17] RUN echo done",
		"", // ensure trailing newline flushes
	}, "\n")

	s := &memArtifactStore{}
	// Seed a record so GetByID works.
	_ = s.Create(context.Background(), &s.rec) // no-op but keeps the pattern

	// Use a fake clock with a 2s tick so the throttle window is crossed
	// on every persist call — that way we observe the final 99%
	// deterministically without sleeping.
	clk := &fakeClock{now: time.Unix(0, 0), step: 2 * time.Second}

	inner := &bytes.Buffer{}
	w := newHadronProgressWriter(inner, s, "test-id")
	w.now = clk.tick
	w.throttle = time.Second

	if _, err := w.Write([]byte(log)); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	// The parser caps at 99% until FinalizeProgress is called: 17/17
	// would otherwise be exactly 100, but we reserve that for "Ready".
	if got := w.CurrentProgress(); got != 99 {
		t.Fatalf("CurrentProgress: got %d, want 99", got)
	}

	// The inner writer must receive every byte, unmodified.
	if inner.String() != log {
		t.Fatalf("inner writer did not receive the full stream")
	}

	// FinalizeProgress must pin the persisted value to 100.
	w.FinalizeProgress()
	if s.maxSeen != 100 {
		t.Fatalf("after FinalizeProgress: maxSeen=%d, want 100", s.maxSeen)
	}
}

func TestHadronProgressParser_ThrottlesDBWrites(t *testing.T) {
	// Feed many progress lines back-to-back within the throttle window.
	// The store should see far fewer updates than lines.
	var lines []string
	for i := 1; i <= 20; i++ {
		lines = append(lines, "#42 [stage-0 "+itoa(i)+"/20] RUN step")
	}
	body := strings.Join(lines, "\n") + "\n"

	s := &memArtifactStore{}
	_ = s.Create(context.Background(), &s.rec)

	// Fixed clock: every call returns the same instant so no update after
	// the first is allowed through the throttle.
	fixed := time.Unix(1000, 0)
	w := newHadronProgressWriter(&bytes.Buffer{}, s, "test-id")
	w.now = func() time.Time { return fixed }
	w.throttle = time.Second

	if _, err := w.Write([]byte(body)); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	// The very first observation still flushes (lastFlushAt is zero), so
	// exactly one Update call is expected under a stopped clock.
	if s.updates != 1 {
		t.Fatalf("throttle allowed %d updates through a stopped clock, want 1", s.updates)
	}
	// And it must be the highest fraction observed, capped at 99.
	if s.rec.Progress != 99 {
		t.Fatalf("persisted Progress=%d, want 99", s.rec.Progress)
	}
}

func TestHadronProgressParser_HandlesSplitLines(t *testing.T) {
	// A marker split across two Write calls must still be recognized on the
	// second write (once the newline arrives).
	s := &memArtifactStore{}
	_ = s.Create(context.Background(), &s.rec)
	w := newHadronProgressWriter(&bytes.Buffer{}, s, "test-id")
	w.now = time.Now
	w.throttle = 0 // no throttle for this test

	if _, err := w.Write([]byte("#5 [stage-0 3")); err != nil {
		t.Fatal(err)
	}
	if w.CurrentProgress() != 0 {
		t.Fatalf("progress advanced on partial line: %d", w.CurrentProgress())
	}
	if _, err := w.Write([]byte("/6] RUN thing\n")); err != nil {
		t.Fatal(err)
	}
	if got := w.CurrentProgress(); got != 50 {
		t.Fatalf("after full line: got %d, want 50", got)
	}
}

// itoa avoids importing strconv just for a test-local helper.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

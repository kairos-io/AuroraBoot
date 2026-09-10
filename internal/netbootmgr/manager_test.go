package netbootmgr

import (
	"strings"
	"testing"
)

type fakeSink struct {
	chunks []string
}

func (f *fakeSink) BroadcastNetbootLogChunk(chunk string) {
	f.chunks = append(f.chunks, chunk)
}

func TestGetLogsEmptyBeforeStart(t *testing.T) {
	m := NewManager(nil)
	if got := m.GetLogs(); got != "" {
		t.Errorf("GetLogs() before any Start = %q, want empty", got)
	}
}

func TestLogWriterAppendsBroadcastsAndPassesThrough(t *testing.T) {
	sink := &fakeSink{}
	m := NewManager(sink)
	var passOn strings.Builder
	w := &logWriter{m: m, passOn: &passOn}

	n, err := w.Write([]byte("dhcp: request from 00:11:22:33:44:55\n"))
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if n != len("dhcp: request from 00:11:22:33:44:55\n") {
		t.Errorf("Write returned n=%d, want full length", n)
	}

	if got := m.GetLogs(); got != "dhcp: request from 00:11:22:33:44:55\n" {
		t.Errorf("GetLogs() = %q, want the written chunk", got)
	}
	if passOn.String() != "dhcp: request from 00:11:22:33:44:55\n" {
		t.Errorf("passOn writer = %q, want the written chunk", passOn.String())
	}
	if len(sink.chunks) != 1 || sink.chunks[0] != "dhcp: request from 00:11:22:33:44:55\n" {
		t.Errorf("sink.chunks = %v, want a single matching chunk", sink.chunks)
	}
}

func TestLogWriterNilSinkDoesNotPanic(t *testing.T) {
	m := NewManager(nil)
	w := &logWriter{m: m, passOn: &strings.Builder{}}
	if _, err := w.Write([]byte("no sink wired\n")); err != nil {
		t.Fatalf("Write returned error with a nil sink: %v", err)
	}
	if got := m.GetLogs(); got != "no sink wired\n" {
		t.Errorf("GetLogs() = %q, want the written chunk even with no sink", got)
	}
}

func TestLogWriterBoundsBufferToMaxLogBytes(t *testing.T) {
	m := NewManager(nil)
	w := &logWriter{m: m, passOn: &strings.Builder{}}

	// Write well past the cap in two chunks, so the bound has to trim
	// mid-buffer, not just refuse a single too-large write.
	first := strings.Repeat("a", maxLogBytes-10)
	second := strings.Repeat("b", 100)
	if _, err := w.Write([]byte(first)); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if _, err := w.Write([]byte(second)); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	got := m.GetLogs()
	if len(got) != maxLogBytes {
		t.Fatalf("GetLogs() length = %d, want exactly maxLogBytes (%d)", len(got), maxLogBytes)
	}
	if !strings.HasSuffix(got, second) {
		t.Errorf("GetLogs() should retain the most recent write in full")
	}
	if strings.Contains(got, "a") == false {
		t.Errorf("GetLogs() should still retain some of the older content, not just the newest write")
	}
}

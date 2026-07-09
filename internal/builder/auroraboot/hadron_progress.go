package auroraboot

import (
	"bytes"
	"context"
	"io"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/kairos-io/AuroraBoot/pkg/store"
)

// hadronProgressRe matches buildkit stage-0 progress lines like:
//
//	#12 [stage-0 3/17] RUN ...
//
// The two capture groups are the completed step and total step count.
// This is deliberately scoped to stage-0 — the Kairos deployer emits
// different progress markers and this parser should ignore them.
var hadronProgressRe = regexp.MustCompile(`#\d+ \[stage-0 (\d+)/(\d+)\]`)

// hadronProgressWriter wraps an inner io.Writer (usually a *dbLogWriter),
// forwarding every chunk unchanged, but also scanning line-terminated data
// for buildkit `#NN [stage-0 X/Y]` markers. It tracks the highest percentage
// seen and pushes it to the artifact store, throttled to at most one DB
// write per second. FinalizeProgress unconditionally pins progress to 100 —
// the caller invokes it once the hadron build enters the Ready phase.
type hadronProgressWriter struct {
	inner       io.Writer
	store       store.ArtifactStore
	id          string
	ctx         context.Context
	throttle    time.Duration
	now         func() time.Time
	mu          sync.Mutex
	lineBuf     []byte
	maxPct      int
	lastFlushed int
	lastFlushAt time.Time
}

// newHadronProgressWriter wraps inner. If store is nil the writer degrades to
// a pass-through with in-memory tracking (useful in tests).
func newHadronProgressWriter(inner io.Writer, s store.ArtifactStore, id string) *hadronProgressWriter {
	return &hadronProgressWriter{
		inner:    inner,
		store:    s,
		id:       id,
		ctx:      context.Background(),
		throttle: time.Second,
		now:      time.Now,
	}
}

func (w *hadronProgressWriter) Write(p []byte) (int, error) {
	// Forward every chunk to the underlying log writer first so log persistence
	// and broadcast are never blocked by the sniff logic.
	var (
		n   int
		err error
	)
	if w.inner != nil {
		n, err = w.inner.Write(p)
	} else {
		n = len(p)
	}
	w.sniff(p)
	return n, err
}

// sniff appends p to the line-buffer, scans every complete line for progress
// markers, updates maxPct, and — subject to throttling — writes the current
// max to the artifact record. Incomplete trailing lines are held over for the
// next Write.
func (w *hadronProgressWriter) sniff(p []byte) {
	if len(p) == 0 {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	w.lineBuf = append(w.lineBuf, p...)
	lastNL := bytes.LastIndexByte(w.lineBuf, '\n')
	if lastNL < 0 {
		// No complete line yet — nothing to scan.
		return
	}
	toScan := w.lineBuf[:lastNL+1]
	// Preserve any partial trailing line for the next chunk.
	remainder := append([]byte(nil), w.lineBuf[lastNL+1:]...)

	for _, m := range hadronProgressRe.FindAllSubmatch(toScan, -1) {
		x, err1 := strconv.Atoi(string(m[1]))
		y, err2 := strconv.Atoi(string(m[2]))
		if err1 != nil || err2 != nil || y <= 0 {
			continue
		}
		if x > y {
			x = y
		}
		pct := x * 100 / y
		// Never let the progress cross 100 from parsing — Ready pins it to 100.
		if pct > 99 {
			pct = 99
		}
		if pct > w.maxPct {
			w.maxPct = pct
		}
	}
	w.lineBuf = remainder

	w.maybeFlush()
}

// maybeFlush persists w.maxPct to the store if it advanced since the last
// flush and the throttle window has elapsed. Callers must hold w.mu.
func (w *hadronProgressWriter) maybeFlush() {
	if w.store == nil || w.maxPct <= w.lastFlushed {
		return
	}
	now := w.now()
	if !w.lastFlushAt.IsZero() && now.Sub(w.lastFlushAt) < w.throttle {
		return
	}
	w.persistProgress(w.maxPct)
	w.lastFlushed = w.maxPct
	w.lastFlushAt = now
}

// persistProgress writes the given percentage into the artifact record.
// Failures are swallowed intentionally — progress is best-effort telemetry
// and must never abort a build.
func (w *hadronProgressWriter) persistProgress(pct int) {
	rec, err := w.store.GetByID(w.ctx, w.id)
	if err != nil {
		return
	}
	if rec.Progress >= pct {
		return
	}
	rec.Progress = pct
	rec.UpdatedAt = w.now()
	_ = w.store.Update(w.ctx, rec)
}

// FinalizeProgress pins the artifact's progress to 100. Call this once the
// hadron build enters the Ready phase so the UI progress bar snaps to full
// even if the last buildkit fraction observed was smaller.
func (w *hadronProgressWriter) FinalizeProgress() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.store == nil {
		w.maxPct = 100
		w.lastFlushed = 100
		return
	}
	rec, err := w.store.GetByID(w.ctx, w.id)
	if err != nil {
		return
	}
	rec.Progress = 100
	rec.UpdatedAt = w.now()
	_ = w.store.Update(w.ctx, rec)
	w.maxPct = 100
	w.lastFlushed = 100
	w.lastFlushAt = w.now()
}

// CurrentProgress returns the highest percentage observed so far. Exposed for
// tests; production callers read the persisted value via the store.
func (w *hadronProgressWriter) CurrentProgress() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.maxPct
}

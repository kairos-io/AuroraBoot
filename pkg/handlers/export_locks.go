package handlers

import (
	"context"
	"sync"
)

// exportLocks hands out one lock per artifact ID so that only a single image
// export runs for a given artifact at a time.
//
// A fleet upgrade asks every node to pull the same artifact at once. The export
// pipeline is a docker create/export/import/save over a multi-gigabyte image,
// and running N of them concurrently is what produced the HTTP 500s in
// kairos-io/kairos#4195. Queueing costs the later nodes wall-clock time and
// gives all of them a valid tar.
//
// Entries are reference counted and removed when the last waiter leaves, so a
// long-lived server does not accumulate a lock per artifact it ever served.
type exportLocks struct {
	mu    sync.Mutex
	locks map[string]*exportLock
}

// exportLock is a one-token semaphore plus the number of goroutines that hold
// or are waiting for it. refs, not the token, decides when the entry can go.
type exportLock struct {
	token chan struct{}
	refs  int
}

func newExportLocks() *exportLocks {
	return &exportLocks{locks: map[string]*exportLock{}}
}

// acquire blocks until the caller owns the lock for key, or ctx is done. The
// returned release function must be called exactly once, and only when the
// error is nil.
func (l *exportLocks) acquire(ctx context.Context, key string) (func(), error) {
	l.mu.Lock()
	lk, ok := l.locks[key]
	if !ok {
		lk = &exportLock{token: make(chan struct{}, 1)}
		l.locks[key] = lk
	}
	lk.refs++
	l.mu.Unlock()

	select {
	case lk.token <- struct{}{}:
	case <-ctx.Done():
		l.drop(key, lk)
		return nil, ctx.Err()
	}

	return func() {
		<-lk.token
		l.drop(key, lk)
	}, nil
}

func (l *exportLocks) drop(key string, lk *exportLock) {
	l.mu.Lock()
	defer l.mu.Unlock()
	lk.refs--
	if lk.refs == 0 && l.locks[key] == lk {
		delete(l.locks, key)
	}
}

// count reports how many artifacts currently have a lock entry. Only the tests
// read it, to prove the map does not grow with every request served.
func (l *exportLocks) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.locks)
}

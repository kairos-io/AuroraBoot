package auroraboot

import "context"

// StopAndWait cancels every build this builder started and blocks until each
// of their goroutines has returned.
//
// A spec that hands the builder a Ginkgo TempDir as its base directory has to
// call this before the spec ends. Build() returns as soon as the row is
// persisted and Cancel() only signals the context, so without the join a
// goroutine is still writing files such as Dockerfile.kairosify into
// baseDir/<id> while Ginkgo's TempDir cleanup walks the same tree. That race
// surfaces as "DeferCleanup callback returned error: unlinkat
// /tmp/ginkgoNNN/<id>: directory not empty" on whichever CI leg loses it.
//
// Exported for the external auroraboot_test package only. Production code has
// no reason to join a build: the HTTP handler hands the request back as soon
// as the row exists, which is the whole point of the goroutine.
func StopAndWait(b *Builder) {
	b.mu.Lock()
	ids := make([]string, 0, len(b.builds))
	states := make([]*buildState, 0, len(b.builds))
	for id, bs := range b.builds {
		ids = append(ids, id)
		states = append(states, bs)
	}
	b.mu.Unlock()

	for _, id := range ids {
		_ = b.Cancel(context.Background(), id)
	}
	for _, bs := range states {
		<-bs.done
	}
}

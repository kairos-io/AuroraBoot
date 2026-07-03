package operator

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/kairos-io/AuroraBoot/pkg/builder"
	"github.com/kairos-io/AuroraBoot/pkg/store"
)

const (
	// podDiscoveryBudget is how long we wait for the operator to create the
	// build Pod before giving up. Five minutes is generous enough to cover a
	// slow operator reconcile loop but not so long that an RBAC/CRD failure
	// keeps the streaming goroutine alive forever.
	podDiscoveryBudget       = 5 * time.Minute
	podDiscoveryPollInterval = 2 * time.Second

	// containerStartRetryInterval is the backoff between attempts when a
	// container's log endpoint reports the container is still waiting to
	// start. The default budget lets a container take up to ~10 minutes to
	// start before we bail (300 * 2s = 600s).
	containerStartRetryInterval = 2 * time.Second
	containerStartMaxRetries    = 300

	// scanLineBufferMax bounds bufio.Scanner's per-line buffer. Buildah and
	// auroraboot logs rarely exceed a few KB but a stray progress bar or
	// noisy tool can emit a very long line; 1MiB is well above anything
	// we've observed and keeps memory bounded.
	scanLineBufferMax = 1 << 20
)

// logSink receives one log line at a time from the streaming pipeline. The
// production implementation fans lines out to the persistent store and any
// subscribed LogBroadcaster; tests use an in-memory recorder.
type logSink interface {
	WriteLine(container, line string) error
}

// podSource abstracts pod discovery and per-container log streaming so tests
// can inject a fake and the production wiring can use a client-go clientset.
type podSource interface {
	Find(ctx context.Context, buildID string) (*corev1.Pod, error)
	Get(ctx context.Context, podName string) (*corev1.Pod, error)
	Open(ctx context.Context, podName, container string) (io.ReadCloser, error)
}

type clientsetPodSource struct {
	cs        kubernetes.Interface
	namespace string
}

func newClientsetPodSource(cs kubernetes.Interface, namespace string) podSource {
	return &clientsetPodSource{cs: cs, namespace: namespace}
}

func (c *clientsetPodSource) Find(ctx context.Context, buildID string) (*corev1.Pod, error) {
	list, err := c.cs.CoreV1().Pods(c.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: podBuildLabel + "=" + buildID,
	})
	if err != nil {
		return nil, err
	}
	if len(list.Items) == 0 {
		return nil, nil
	}
	// The operator produces exactly one builder Pod per OSArtifact; if more
	// somehow appear, pick the first and let the caller sort it out.
	return &list.Items[0], nil
}

func (c *clientsetPodSource) Get(ctx context.Context, podName string) (*corev1.Pod, error) {
	return c.cs.CoreV1().Pods(c.namespace).Get(ctx, podName, metav1.GetOptions{})
}

func (c *clientsetPodSource) Open(ctx context.Context, podName, container string) (io.ReadCloser, error) {
	return c.cs.CoreV1().Pods(c.namespace).GetLogs(podName, &corev1.PodLogOptions{
		Container: container,
		Follow:    true,
	}).Stream(ctx)
}

// podBuildLabel matches the operator's builder-pod label
// (kairos-operator/internal/controller/osartifact_controller.go: artifactLabel).
const podBuildLabel = "build.kairos.io/artifact"

// waitForPod polls src.Find every pollInterval until a pod is returned or the
// budget expires. It respects ctx cancellation as an immediate exit. Transient
// non-NotFound API errors (a single kube-apiserver hiccup) are tolerated and
// logged to stderr so the caller can still catch a persistent failure via
// the overall discovery budget; a fatal error is only surfaced once the entire
// budget elapses. This mirrors waitContainerLeftWaiting's asymmetric behaviour:
// a transient blip must not permanently disable log streaming for a build.
func waitForPod(ctx context.Context, src podSource, buildID string, budget, pollInterval time.Duration) (*corev1.Pod, error) {
	deadline := time.Now().Add(budget)
	var lastErr error
	for {
		pod, err := src.Find(ctx, buildID)
		switch {
		case err == nil:
			// no-op; fall through to pod nil-check.
		case apierrors.IsNotFound(err):
			// Pod is not created yet; keep waiting.
		default:
			// Transient API error; remember it, log for visibility, keep polling.
			lastErr = err
			fmt.Fprintf(os.Stderr, "waitForPod: transient discovery error for build %q: %v\n", buildID, err)
		}
		if pod != nil {
			return pod, nil
		}
		if time.Now().After(deadline) {
			if lastErr != nil {
				return nil, fmt.Errorf("build pod for %q did not appear within %s (last error: %w)", buildID, budget, lastErr)
			}
			return nil, fmt.Errorf("build pod for %q did not appear within %s", buildID, budget)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

// streamContainer streams one container's logs into sink. It first polls the
// pod until the container leaves Waiting state — the log API returns an empty
// body without error for a Waiting container in some kubelet versions, so
// merely retrying Open on that response would deliver zero lines. Once the
// container is Running or Terminated, it opens the log stream and scans lines
// until EOF, sink returns an error, or ctx is cancelled. Open errors that
// indicate the container is still starting are retried up to maxRetries times
// with retryInterval between attempts.
func streamContainer(ctx context.Context, src podSource, podName, container string, sink logSink, retryInterval time.Duration, maxRetries int) error {
	if err := waitContainerLeftWaiting(ctx, src, podName, container, retryInterval, maxRetries); err != nil {
		return err
	}
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		rc, err := src.Open(ctx, podName, container)
		if err == nil {
			return scanLines(ctx, rc, container, sink)
		}
		if !isContainerNotReadyErr(err) {
			return err
		}
		if attempt >= maxRetries {
			return fmt.Errorf("container %q never became ready after %d attempts: %w", container, attempt, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryInterval):
		}
	}
}

// waitContainerLeftWaiting polls the pod until the named container is either
// Running or Terminated. Returns ctx.Err() on cancellation, a NotFound error
// if the pod disappears, or a deadline error if maxRetries is exhausted.
func waitContainerLeftWaiting(ctx context.Context, src podSource, podName, container string, pollInterval time.Duration, maxRetries int) error {
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		pod, err := src.Get(ctx, podName)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return err
			}
			// Transient API error; retry on the next tick rather than fail.
		} else if containerHasStarted(pod, container) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
	return fmt.Errorf("container %q never left Waiting state after %d polls", container, maxRetries)
}

// containerHasStarted returns true when the named container's status shows it
// is Running or Terminated. Waiting (image pull, init dependency) returns
// false — the log API produces an empty stream in that state.
func containerHasStarted(pod *corev1.Pod, name string) bool {
	check := func(list []corev1.ContainerStatus) bool {
		for _, s := range list {
			if s.Name != name {
				continue
			}
			return s.State.Running != nil || s.State.Terminated != nil
		}
		return false
	}
	return check(pod.Status.InitContainerStatuses) || check(pod.Status.ContainerStatuses)
}

// isContainerNotReadyErr recognises the transient errors kube-apiserver
// returns when a container has not started yet. The exact message varies
// across kubelet versions ("waiting to start", "PodInitializing",
// "ContainerCreating"), so we match on substrings and treat everything else
// as fatal to avoid retry loops that mask real bugs.
func isContainerNotReadyErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if strings.Contains(msg, "waiting to start") ||
		strings.Contains(msg, "ContainerCreating") ||
		strings.Contains(msg, "PodInitializing") {
		return true
	}
	return false
}

// scanLines reads rc line-by-line and hands each raw (unprefixed) line to
// sink.WriteLine along with the container name. It closes rc on return and
// exits promptly on ctx cancellation.
func scanLines(ctx context.Context, rc io.ReadCloser, container string, sink logSink) error {
	defer rc.Close()
	scanner := bufio.NewScanner(rc)
	scanner.Buffer(make([]byte, 0, 4096), scanLineBufferMax)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := sink.WriteLine(container, scanner.Text()); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

// streamAll drives every container in pod: init containers sequentially in
// declaration order (they run in sequence), then all main containers in
// parallel (they run in parallel). It returns the first non-cancellation
// error encountered.
func streamAll(ctx context.Context, src podSource, pod *corev1.Pod, sink logSink, retryInterval time.Duration, maxRetries int) error {
	for _, c := range pod.Spec.InitContainers {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := streamContainer(ctx, src, pod.Name, c.Name, sink, retryInterval, maxRetries); err != nil {
			return err
		}
	}
	if len(pod.Spec.Containers) == 0 {
		return nil
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(pod.Spec.Containers))
	for _, c := range pod.Spec.Containers {
		c := c
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := streamContainer(ctx, src, pod.Name, c.Name, sink, retryInterval, maxRetries); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			return err
		}
	}
	return nil
}

// broadcastingSink fans each line out to the artifact store (if configured)
// and the LogBroadcaster (if configured). Lines land in the store as
// "[<container>] <line>\n" so a post-mortem reader can tell which stage of
// the buildah/auroraboot pipeline produced them; broadcasts carry the same
// prefixed text so subscribers render matching output.
type broadcastingSink struct {
	ctx         context.Context
	buildID     string
	store       store.ArtifactStore
	broadcaster builder.LogBroadcaster
}

func (b *broadcastingSink) WriteLine(container, line string) error {
	chunk := "[" + container + "] " + line + "\n"
	if b.store != nil {
		_ = b.store.AppendLog(b.ctx, b.buildID, chunk)
	}
	if b.broadcaster != nil {
		b.broadcaster.BroadcastLogChunk(b.buildID, chunk)
	}
	return nil
}

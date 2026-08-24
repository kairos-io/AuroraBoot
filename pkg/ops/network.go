package ops

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/cavaliergopher/grab/v3"
	"github.com/kairos-io/AuroraBoot/internal"
)

const (
	UserAgent = "AuroraBoot"

	// downloadAttempts and downloadRetryBaseDelay: grab makes exactly one
	// HTTP attempt per call and returns whatever error it hit, including a
	// transient one (e.g. an HTTP/2 "stream error: ... PROTOCOL_ERROR"
	// from the peer mid-transfer). A multi-GB release asset has plenty of
	// opportunity to hit one of those, so a single failed attempt should
	// not be the whole download's answer.
	downloadAttempts       = 3
	downloadRetryBaseDelay = 2 * time.Second
)

// ServeArtifacts serve local artifacts as standard http server
func ServeArtifacts(listenAddr string, dirFunc valueGetOnCall) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		dir := dirFunc()
		fs := http.FileServer(http.Dir(dir))
		// Use a private mux instead of http.DefaultServeMux: registering "/" on
		// the global mux panics ("multiple registrations for /") if this runs
		// more than once in a process and leaks the handler across servers.
		mux := http.NewServeMux()
		mux.Handle("/", fs)
		serverOne := &http.Server{
			Addr:    listenAddr,
			Handler: mux,
		}
		go func() {
			<-ctx.Done()
			serverOne.Shutdown(context.Background())
		}()
		internal.Log.Logger.Info().Msgf("Listening on %v...", listenAddr)
		return serverOne.ListenAndServe()
	}
}

// DownloadArtifact downloads artifacts remotely (e.g. http(s), ...)
func DownloadArtifact(url string, isoFunc valueGetOnCall) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		// https://github.com/kairos-io/kairos/releases/download/v1.5.0/kairos-alpine-ubuntu-v1.5.0.iso
		dst := isoFunc()
		_, err := download(ctx, url, dst)
		if err == nil {
			internal.Log.Logger.Info().Str("artifact", url).Str("destination", dst).Msg("Artifact downloaded successfully")
		}
		return err
	}
}

// download retries downloadOnce on failure, up to downloadAttempts times.
// It does not retry after ctx is canceled -- that is a caller decision to
// stop, not a transient failure to recover from.
func download(ctx context.Context, url, dst string) (string, error) {
	var dstFile string
	var err error
	for attempt := 1; attempt <= downloadAttempts; attempt++ {
		dstFile, err = downloadOnce(ctx, url, dst)
		if err == nil {
			return dstFile, nil
		}
		if ctx.Err() != nil || attempt == downloadAttempts {
			return dstFile, err
		}
		internal.Log.Logger.Warn().Err(err).Str("artifact", url).Int("attempt", attempt).Msg("download failed, retrying")
		delay := downloadRetryBaseDelay * time.Duration(attempt)
		select {
		case <-ctx.Done():
			return dstFile, err
		case <-time.After(delay):
		}
	}
	return dstFile, err
}

func downloadOnce(ctx context.Context, url, dst string) (string, error) {
	// create client
	client := grab.NewClient()
	// https://github.com/cavaliergopher/grab/issues/104
	client.UserAgent = UserAgent
	req, _ := grab.NewRequest(dst, url)

	// start download
	internal.Log.Logger.Info().Msgf("Downloading %v...", req.URL())
	resp := client.Do(req)
	internal.Log.Logger.Printf("%s:  %v", url, resp.HTTPResponse.Status)

	// start UI loop
	t := time.NewTicker(500 * time.Millisecond)
	defer t.Stop()
	dstFile := filepath.Join(dst, resp.Filename)
Loop:
	for {
		select {
		case <-ctx.Done():
			defer os.RemoveAll(dstFile)
			return dst, fmt.Errorf("context canceled")
		case <-t.C:
			internal.Log.Printf("%s: transferred %v / %v bytes (%.2f%%)",
				url,
				resp.BytesComplete(),
				resp.Size(),
				100*resp.Progress())

		case <-resp.Done:
			// download is complete
			fmt.Printf("transferred %v / %v bytes (%.2f%%)    \n", resp.BytesComplete(), resp.Size(), 100*resp.Progress())
			break Loop
		}
	}

	// check for errors
	if err := resp.Err(); err != nil {
		defer os.RemoveAll(dstFile)
		return dstFile, err
	}

	return dstFile, nil
}

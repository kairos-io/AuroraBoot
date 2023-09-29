package ops

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/cavaliergopher/grab/v3"
	"github.com/rs/zerolog/log"
)

const (
	UserAgent =  "AuroraBoot"
)

// ServeArtifacts serve local artifacts as standard http server
func ServeArtifacts(listenAddr, dir string) func(ctx context.Context) error {
	return func(ctx context.Context) error {

		fs := http.FileServer(http.Dir(dir))
		http.Handle("/", fs)
		serverOne := &http.Server{
			Addr:    listenAddr,
			Handler: nil,
		}
		go func() {
			<-ctx.Done()
			serverOne.Shutdown(context.Background())
		}()
		log.Info().Msgf("Listening on %v...", listenAddr)
		return serverOne.ListenAndServe()
	}
}

// DownloadArtifact downloads artifacts remotely (e.g. http(s), ...)
func DownloadArtifact(url, dst string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		// https://github.com/kairos-io/kairos/releases/download/v1.5.0/kairos-alpine-ubuntu-v1.5.0.iso
		_, err := download(ctx, url, dst)
		return err
	}
}

func download(ctx context.Context, url, dst string) (string, error) {
	// create client
	client := grab.NewClient()
	// https://github.com/cavaliergopher/grab/issues/104
	client.UserAgent = UserAgent
	req, _ := grab.NewRequest(dst, url)

	// start download
	log.Info().Msgf("Downloading %v...", req.URL())
	resp := client.Do(req)
	log.Printf("%s:  %v", url, resp.HTTPResponse.Status)

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
			log.Printf("%s: transferred %v / %v bytes (%.2f%%)",
				url,
				resp.BytesComplete(),
				resp.Size(),
				100*resp.Progress())

		case <-resp.Done:
			// download is complete
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

package deployer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cavaliergopher/grab/v3"
)

func dowloadArtifact(url, dst string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		// https://github.com/kairos-io/kairos/releases/download/v1.5.0/kairos-alpine-ubuntu-v1.5.0.iso
		_, err := download(ctx, url, dst)
		return err
	}
}

func download(ctx context.Context, url, dst string) (string, error) {
	// create client
	client := grab.NewClient()
	req, _ := grab.NewRequest(dst, url)

	// start download
	fmt.Printf("Downloading %v...\n", req.URL())
	resp := client.Do(req)
	fmt.Printf("  %v\n", resp.HTTPResponse.Status)

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
			fmt.Printf("  transferred %v / %v bytes (%.2f%%)\n",
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

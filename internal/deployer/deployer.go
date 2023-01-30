package deployer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/cavaliergopher/grab/v3"
	"github.com/hashicorp/go-multierror"
	"github.com/spectrocloud-labs/herd"
)

type ReleaseArtifact struct {
	ArtifactVersion string
	ReleaseVersion  string
	Flavor          string
	Repository      string
}

func urlGen(repository, releaseVersion, flavor, artifactVersion, artifactType string) string {
	return fmt.Sprintf("https://github.com/%s/releases/download/%s/kairos-%s-%s-%s", repository, releaseVersion, flavor, artifactVersion, artifactType)
}

func (a ReleaseArtifact) ISOUrl() string {
	return urlGen(a.Repository, a.ReleaseVersion, a.Flavor, a.ArtifactVersion, "iso")
}

func (a ReleaseArtifact) InitrdURL() string {
	return urlGen(a.Repository, a.ReleaseVersion, a.Flavor, a.ArtifactVersion, "initrd")
}

func (a ReleaseArtifact) KernelURL() string {
	return urlGen(a.Repository, a.ReleaseVersion, a.Flavor, a.ArtifactVersion, "kernel")
}

func (a ReleaseArtifact) SquashFSURL() string {
	return urlGen(a.Repository, a.ReleaseVersion, a.Flavor, a.ArtifactVersion, "squashfs")
}

// converts chema to Operations
// Netboot or not netboot
// If Not netboot:
//     download iso -> Edit it to attach cloud config (?) -> Offer link to download modified ISO with cloud config
//     download IPXE iso -> offer ISO that boots over ipxe with pixiecore
//
//    or, offer generic IPXE iso -> and start netboot anyway

const (
	downloadNetboot = "download-netboot"
)

func RegisterOperations(g *herd.Graph, artifact ReleaseArtifact) error {

	g.Add(downloadNetboot, herd.WithCallback(dowloadNetbootArtifacts(artifact)))
	g.Add("start-netboot", herd.WithCallback(func(ctx context.Context) error {
		return nil
	}), herd.WithDeps(downloadNetboot))

	return nil
}

func dowloadNetbootArtifacts(artifact ReleaseArtifact) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		// https://github.com/kairos-io/kairos/releases/download/v1.5.0/kairos-alpine-ubuntu-v1.5.0.iso
		var wg sync.WaitGroup
		errs := make(chan error, 1)
		for _, s := range []string{artifact.InitrdURL()} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				filePath, err := download(ctx, ".", s)
				if err != nil {
					errs <- err
				} else {
					fmt.Println("Downloaded", filePath)
				}
			}()

		}

		wg.Wait()
		close(errs)

		var err error

		for e := range errs {
			err = multierror.Append(err, e)
		}
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

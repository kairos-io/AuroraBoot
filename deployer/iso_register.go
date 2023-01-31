package deployer

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/kairos-io/AuroraBoot/pkg/ops"

	"github.com/spectrocloud-labs/herd"
)

// converts chema to Operations
// Netboot or not netboot
// If Not netboot:
//     download iso -> Edit it to attach cloud config (?) -> Offer link to download modified ISO with cloud config
//     download IPXE iso -> offer ISO that boots over ipxe with pixiecore
// TODO ops: start HTTP server, offer artifacts from dir
// TODO ops: download ISO save it to dir
//
//    or, offer generic IPXE iso -> and start netboot anyway

const (
	opDownloadISO     = "download-iso"
	opCopyCloudConfig = "copy-cloud-config"
	opPrepareISO      = "prepare-iso"
	opStartHTTPServer = "start-httpserver"
)

func RegisterISOOperations(g *herd.Graph, artifact ReleaseArtifact, cloudConfigFile string) error {
	dst := "/tmp/iso"

	g.Add(opPrepareISO, herd.WithCallback(func(ctx context.Context) error {
		return os.MkdirAll(dst, 0700)
	}))

	g.Add(opCopyCloudConfig,
		herd.WithDeps(opPrepareISO),
		herd.WithCallback(func(ctx context.Context) error {
			_, err := copy(cloudConfigFile, filepath.Join(dst, "config.yaml"))
			return err
		}))
	g.Add(opDownloadISO, herd.WithCallback(ops.DownloadArtifact(artifact.ISOUrl(), dst)))

	//TODO: add Validate step
	g.Add(
		opStartHTTPServer,
		herd.WithDeps(opDownloadISO, opCopyCloudConfig),
		herd.Background,
		herd.WithCallback(ops.ServeArtifacts(":8080", dst)),
	)

	return nil
}

func copy(src, dst string) (int64, error) {
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return 0, err
	}

	if !sourceFileStat.Mode().IsRegular() {
		return 0, fmt.Errorf("%s is not a regular file", src)
	}

	source, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer destination.Close()
	nBytes, err := io.Copy(destination, source)
	return nBytes, err
}

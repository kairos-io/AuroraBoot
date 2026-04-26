package auroraboot

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kairos-io/AuroraBoot/deployer"
	"github.com/kairos-io/AuroraBoot/pkg/constants"
	"github.com/kairos-io/AuroraBoot/pkg/schema"
	"github.com/kairos-io/AuroraBoot/pkg/uki"
	"github.com/kairos-io/AuroraBoot/pkg/builder"
	"github.com/kairos-io/AuroraBoot/pkg/store"
	sdklogger "github.com/kairos-io/kairos-sdk/types/logger"
	"github.com/spectrocloud-labs/herd"
)

// DeployerFunc abstracts the AuroraBoot deployer execution so we can mock it in tests.
type DeployerFunc func(ctx context.Context, config schema.Config, artifact schema.ReleaseArtifact, outputDir string) error

// UKIBuildFunc abstracts the AuroraBoot pkg/uki.Build call so tests can
// capture the options we pass without running a real build.
type UKIBuildFunc func(opts uki.Options) error

// DefaultDeployerFunc runs AuroraBoot for real.
func DefaultDeployerFunc(ctx context.Context, config schema.Config, artifact schema.ReleaseArtifact, outputDir string) error {
	d := deployer.NewDeployer(config, artifact, herd.EnableInit)
	if err := deployer.RegisterAll(d); err != nil {
		return fmt.Errorf("registering deployer steps: %w", err)
	}
	if err := d.Run(ctx); err != nil {
		return fmt.Errorf("running deployer: %w", err)
	}
	if err := d.CollectErrors(); err != nil {
		return fmt.Errorf("deployer errors: %w", err)
	}
	return nil
}

// DefaultUKIBuildFunc calls pkg/uki.Build for real.
func DefaultUKIBuildFunc(opts uki.Options) error { return uki.Build(opts) }

// Builder implements builder.ArtifactBuilder using AuroraBoot.
type Builder struct {
	builds      map[string]*buildState
	mu          sync.RWMutex
	baseDir     string
	deployFunc     DeployerFunc
	ukiBuildFn     UKIBuildFunc
	store          store.ArtifactStore
	logBroadcaster LogBroadcaster
}

// LogBroadcaster is the hook dbLogWriter calls on every flush. The
// production implementation is a thin wrapper around ws.UIHub — the
// builder package declares this minimal interface so it doesn't have to
// import ws and risk an import cycle. Tests pass a nil broadcaster and
// the flush path falls back to DB-only.
type LogBroadcaster interface {
	BroadcastLogChunk(buildID string, chunk string)
}

type buildState struct {
	status builder.BuildStatus
	cancel context.CancelFunc
}

// dbLogWriter buffers log output and periodically flushes to the artifact
// store. When a broadcaster is attached, every flush also fans the chunk
// out to any subscribed UI clients so they can render logs in real time.
type dbLogWriter struct {
	store       store.ArtifactStore
	id          string
	buf         bytes.Buffer
	mu          sync.Mutex
	ctx         context.Context
	broadcaster LogBroadcaster
}

func (w *dbLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.buf.Write(p)
	if w.buf.Len() > 4096 {
		w.flush()
	}
	return n, err
}

func (w *dbLogWriter) flush() {
	if w.buf.Len() == 0 {
		return
	}
	text := w.buf.String()
	w.buf.Reset()
	_ = w.store.AppendLog(w.ctx, w.id, text)
	if w.broadcaster != nil {
		w.broadcaster.BroadcastLogChunk(w.id, text)
	}
}

// Flush writes any buffered log data to the store.
func (w *dbLogWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.flush()
}

// New creates a new AuroraBoot-backed builder. If deployFunc is nil, DefaultDeployerFunc is used.
// If artifactStore is nil, the builder falls back to in-memory only (useful for tests).
func New(baseDir string, deployFunc DeployerFunc, artifactStore store.ArtifactStore) *Builder {
	if deployFunc == nil {
		deployFunc = DefaultDeployerFunc
	}
	return &Builder{
		builds:     make(map[string]*buildState),
		baseDir:    baseDir,
		deployFunc: deployFunc,
		ukiBuildFn: DefaultUKIBuildFunc,
		store:      artifactStore,
	}
}

// WithUKIBuildFunc swaps the pkg/uki.Build implementation used by buildUKI.
// Tests use this to capture the uki.Options we pass through.
func (b *Builder) WithUKIBuildFunc(fn UKIBuildFunc) *Builder {
	b.ukiBuildFn = fn
	return b
}

// WithLogBroadcaster attaches a broadcaster that receives every log chunk
// flushed by a build. Usually a *ws.UIHub — but kept as an interface so
// the builder package doesn't have to import ws.
func (b *Builder) WithLogBroadcaster(lb LogBroadcaster) *Builder {
	b.logBroadcaster = lb
	return b
}

// Build starts an asynchronous artifact build and returns immediately with a Pending status.
func (b *Builder) Build(ctx context.Context, opts builder.BuildOptions) (*builder.BuildStatus, error) {
	id := opts.ID
	if id == "" {
		id = uuid.New().String()
	}

	outputDir := filepath.Join(b.baseDir, id)
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating output dir: %w", err)
	}

	// Use Background — the build outlives the HTTP request that triggered it.
	// The request ctx would cancel when the handler returns the 201 response.
	buildCtx, cancel := context.WithCancel(context.Background())

	bs := &buildState{
		status: builder.BuildStatus{
			ID:    id,
			Phase: builder.BuildPending,
		},
		cancel: cancel,
	}

	b.mu.Lock()
	b.builds[id] = bs
	b.mu.Unlock()

	// Persist artifact record in DB if store is available.
	if b.store != nil {
		rec := &store.ArtifactRecord{
			ID:               id,
			Name:             opts.Name,
			Phase:            store.ArtifactPending,
			BaseImage:        opts.BaseImage,
			KairosVersion:    opts.KairosVersion,
			Model:            opts.Model,
			ISO:              opts.ISO,
			CloudImage:       opts.CloudImage,
			Netboot:          opts.Netboot,
			FIPS:             opts.FIPS,
			TrustedBoot:      opts.TrustedBoot,
			Arch:             opts.Source.Arch,
			Variant:          opts.Source.Variant,
			RawDisk:          opts.Outputs.RawDisk,
			Tar:              opts.Outputs.Tar,
			GCE:              opts.Outputs.GCE,
			VHD:              opts.Outputs.VHD,
			UKI:              opts.Outputs.UKI,
			KairosInitImage:  opts.KairosInitImage,
			AutoInstall:      opts.Provisioning.AutoInstall,
			RegisterAuroraBoot: opts.Provisioning.RegisterAuroraBoot,
			Dockerfile:       opts.Dockerfile,
			ExtendCmdline:    opts.ExtendCmdline,
			CloudConfig:      opts.CloudConfig,
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		}
		if err := b.store.Create(ctx, rec); err != nil {
			cancel()
			return nil, fmt.Errorf("persisting artifact record: %w", err)
		}
	}

	go b.run(buildCtx, bs, opts, outputDir)

	return &builder.BuildStatus{
		ID:    id,
		Phase: builder.BuildPending,
	}, nil
}

func (b *Builder) run(ctx context.Context, bs *buildState, opts builder.BuildOptions, outputDir string) {
	b.setPhase(bs, builder.BuildBuilding, "")

	// Update DB phase.
	if b.store != nil {
		_ = b.updateDBPhase(ctx, bs.status.ID, store.ArtifactBuilding, "")
	}

	// Set up log writer for DB persistence plus an optional live broadcast
	// to subscribed UI clients.
	var logWriter *dbLogWriter
	if b.store != nil {
		logWriter = &dbLogWriter{
			store:       b.store,
			id:          bs.status.ID,
			ctx:         context.Background(), // use background so flushes work even after build ctx cancel
			broadcaster: b.logBroadcaster,
		}
	}

	// Step 1: If Dockerfile is provided, build a container image from it.
	// The Dockerfile takes precedence — BaseImage is only used when no Dockerfile is set.
	containerImage := opts.BaseImage
	if opts.Dockerfile != "" {
		if logWriter != nil {
			fmt.Fprintf(logWriter, "=== Building from Dockerfile ===\n")
			if opts.BaseImage != "" {
				fmt.Fprintf(logWriter, "Note: Dockerfile overrides base image (%s)\n\n", opts.BaseImage)
			}
			logWriter.Flush()
		}
		img, err := b.dockerBuild(ctx, opts, outputDir, logWriter)
		if err != nil {
			msg := fmt.Sprintf("docker build failed: %v", err)
			b.setPhase(bs, builder.BuildError, msg)
			if b.store != nil {
				if logWriter != nil {
					logWriter.Flush()
				}
				_ = b.updateDBPhase(context.Background(), bs.status.ID, store.ArtifactError, msg)
			}
			return
		}
		containerImage = img
	}

	if opts.Dockerfile == "" && logWriter != nil {
		fmt.Fprintf(logWriter, "=== Using base image: %s ===\n\n", containerImage)
		logWriter.Flush()
	}

	// Step 1.5: Ensure image is kairosified.
	containerImage, err := b.ensureKairosified(ctx, containerImage, opts, outputDir, logWriter)
	if err != nil {
		msg := fmt.Sprintf("kairosify failed: %v", err)
		b.setPhase(bs, builder.BuildError, msg)
		if b.store != nil {
			if logWriter != nil {
				logWriter.Flush()
			}
			_ = b.updateDBPhase(context.Background(), bs.status.ID, store.ArtifactError, msg)
		}
		return
	}

	// Step 2: Assemble AuroraBoot config and run deployer.
	if logWriter != nil {
		fmt.Fprintf(logWriter, "=== Starting AuroraBoot build ===\n")
		fmt.Fprintf(logWriter, "Image: %s\n", containerImage)
		if opts.ISO { fmt.Fprintf(logWriter, "Output: ISO\n") }
		if opts.CloudImage { fmt.Fprintf(logWriter, "Output: Cloud Image (raw disk)\n") }
		if opts.Netboot { fmt.Fprintf(logWriter, "Output: Netboot\n") }
		fmt.Fprintf(logWriter, "Output dir: %s\n", outputDir)
		if opts.CloudConfig != "" {
			fmt.Fprintf(logWriter, "Cloud config:\n%s\n", opts.CloudConfig)
		}
		fmt.Fprintf(logWriter, "\n")
		logWriter.Flush()
	}
	config, artifact := b.assembleConfig(opts, containerImage, outputDir)
	if err := b.deployFunc(ctx, config, artifact, outputDir); err != nil {
		msg := fmt.Sprintf("auroraboot failed: %v", err)
		b.setPhase(bs, builder.BuildError, msg)
		if b.store != nil {
			if logWriter != nil {
				logWriter.Flush()
			}
			_ = b.updateDBPhase(context.Background(), bs.status.ID, store.ArtifactError, msg)
		}
		return
	}

	if logWriter != nil {
		fmt.Fprintf(logWriter, "\n=== AuroraBoot build completed ===\n")
		logWriter.Flush()
	}

	// Step 2.5: Save container image as TAR if requested.
	if opts.Outputs.Tar && containerImage != "" {
		if logWriter != nil {
			fmt.Fprintf(logWriter, "\n=== Saving container image as TAR ===\n")
			logWriter.Flush()
		}
		tarPath := filepath.Join(outputDir, "container.tar")
		cmd := exec.CommandContext(ctx, "docker", "save", "-o", tarPath, containerImage)
		if logWriter != nil {
			cmd.Stdout = logWriter
			cmd.Stderr = logWriter
		}
		if err := cmd.Run(); err != nil && logWriter != nil {
			fmt.Fprintf(logWriter, "Warning: docker save failed: %v\n", err)
			logWriter.Flush()
		}
	}

	// Step 2.6: Build UKI (Unified Kernel Image) if requested.
	if opts.Outputs.UKI {
		if logWriter != nil {
			fmt.Fprintf(logWriter, "\n=== Building UKI (Unified Kernel Image) ===\n")
			logWriter.Flush()
		}
		if err := b.buildUKI(ctx, opts, containerImage, outputDir, logWriter); err != nil {
			msg := fmt.Sprintf("UKI build failed: %v", err)
			if logWriter != nil {
				fmt.Fprintf(logWriter, "%s\n", msg)
				logWriter.Flush()
			}
			b.setPhase(bs, builder.BuildError, msg)
			if b.store != nil {
				_ = b.updateDBPhase(context.Background(), bs.status.ID, store.ArtifactError, msg)
			}
			return
		}
	}

	// Step 2.7: Clean up ISO files when the caller didn't request ISO output.
	// Netboot extraction needs a generated ISO as its source, so GenISO may
	// still have run as scaffolding — but the user never asked for the ISO
	// itself and we shouldn't keep multi-GB files around or list them as
	// build outputs. The raw-disk paths don't produce ISOs at all, so the
	// only files present here would be that scaffolding ISO.
	if !opts.Outputs.ISO {
		for _, pattern := range []string{"*.iso", "*.iso.sha256"} {
			matches, _ := filepath.Glob(filepath.Join(outputDir, pattern))
			for _, p := range matches {
				if err := os.Remove(p); err != nil && logWriter != nil {
					fmt.Fprintf(logWriter, "Warning: could not remove unrequested %s: %v\n", filepath.Base(p), err)
				}
			}
		}
	}

	// Step 3: Collect artifact paths.
	artifacts := collectArtifacts(outputDir)
	if logWriter != nil {
		fmt.Fprintf(logWriter, "Artifacts:\n")
		for _, a := range artifacts {
			fmt.Fprintf(logWriter, "  %s\n", filepath.Base(a))
		}
		logWriter.Flush()
	}

	b.mu.Lock()
	bs.status.Phase = builder.BuildReady
	bs.status.Message = ""
	bs.status.Artifacts = artifacts
	b.mu.Unlock()

	// Update DB with final state.
	if b.store != nil {
		if logWriter != nil {
			logWriter.Flush()
		}
		rec, err := b.store.GetByID(context.Background(), bs.status.ID)
		if err == nil {
			rec.Phase = store.ArtifactReady
			rec.Message = ""
			rec.ArtifactFiles = artifacts
			rec.ContainerImage = containerImage
			rec.UpdatedAt = time.Now()
			_ = b.store.Update(context.Background(), rec)
		}
	}
}

func (b *Builder) updateDBPhase(ctx context.Context, id, phase, message string) error {
	rec, err := b.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	rec.Phase = phase
	rec.Message = message
	rec.UpdatedAt = time.Now()
	return b.store.Update(ctx, rec)
}

func (b *Builder) setPhase(bs *buildState, phase, message string) {
	b.mu.Lock()
	bs.status.Phase = phase
	bs.status.Message = message
	b.mu.Unlock()
}

// isKairosified checks if an image already contains /etc/kairos-release.
func (b *Builder) isKairosified(ctx context.Context, image string) bool {
	cmd := exec.CommandContext(ctx, "docker", "run", "--rm", "--entrypoint", "cat", image, "/etc/kairos-release")
	return cmd.Run() == nil
}

// ensureKairosified checks if the image is a Kairos image, and if not, builds one using kairos-init.
// Skipped when store is nil (test mode without Docker).
func (b *Builder) ensureKairosified(ctx context.Context, image string, opts builder.BuildOptions, outputDir string, logWriter *dbLogWriter) (string, error) {
	if b.store == nil {
		// Test mode — skip kairosification (no Docker available)
		return image, nil
	}
	if b.isKairosified(ctx, image) {
		return image, nil
	}

	kairosInitImage := opts.KairosInitImage
	if kairosInitImage == "" {
		kairosInitImage = os.Getenv("KAIROS_INIT_IMAGE")
	}
	if kairosInitImage == "" {
		kairosInitImage = "quay.io/kairos/kairos-init:v0.8.4"
	}

	// Build kairos-init flags
	var flags []string
	if opts.Model != "" {
		flags = append(flags, "-m", opts.Model)
	}
	if opts.KubernetesDistro != "" {
		flags = append(flags, "-p", opts.KubernetesDistro)
	}
	if opts.KubernetesDistro == "k3s" && opts.KubernetesVersion != "" {
		flags = append(flags, "--provider-k3s-version", opts.KubernetesVersion)
	} else if opts.KubernetesDistro == "k0s" && opts.KubernetesVersion != "" {
		flags = append(flags, "--provider-k0s-version", opts.KubernetesVersion)
	}
	if opts.FIPS {
		flags = append(flags, "--fips")
	}
	if opts.TrustedBoot {
		flags = append(flags, "-t", "true")
	}
	version := opts.KairosVersion
	if version == "" {
		version = "latest"
	}
	flags = append(flags, "--version", version)

	flagStr := strings.Join(flags, " ")

	dockerfile := fmt.Sprintf(`FROM %s AS kairos-init
FROM %s
COPY --from=kairos-init /kairos-init /kairos-init
RUN /kairos-init -l debug -s install %s && \
    /kairos-init -l debug -s init %s && \
    rm -f /kairos-init
`, kairosInitImage, image, flagStr, flagStr)

	// Write Dockerfile
	dockerfilePath := filepath.Join(outputDir, "Dockerfile.kairosify")
	if err := os.WriteFile(dockerfilePath, []byte(dockerfile), 0o644); err != nil {
		return "", fmt.Errorf("writing kairosify Dockerfile: %w", err)
	}

	tag := fmt.Sprintf("auroraboot-kairos:%s", opts.ID)
	cmd := exec.CommandContext(ctx, "docker", "build", "-t", tag, "-f", dockerfilePath, outputDir)
	if logWriter != nil {
		cmd.Stdout = logWriter
		cmd.Stderr = logWriter
	}
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("kairosify docker build: %w", err)
	}
	return tag, nil
}

// assembleConfig builds the AuroraBoot schema.Config and schema.ReleaseArtifact from BuildOptions.
func (b *Builder) assembleConfig(opts builder.BuildOptions, containerImage, outputDir string) (schema.Config, schema.ReleaseArtifact) {
	config := schema.Config{
		State:             outputDir,
		DisableHTTPServer: true,
		DisableNetboot:    !opts.Netboot,
		DisableISOboot:    !opts.ISO,
	}

	if opts.Source.Arch != "" {
		config.Arch = opts.Source.Arch
	}

	if opts.CloudImage || opts.Outputs.RawDisk || opts.Outputs.GCE || opts.Outputs.VHD {
		config.Disk.EFI = true
	}
	config.Disk.GCE = opts.Outputs.GCE
	config.Disk.VHD = opts.Outputs.VHD

	if opts.CloudConfig != "" {
		config.CloudConfig = opts.CloudConfig
	}

	if opts.OverlayRootfs != "" {
		config.ISO.OverlayRootfs = opts.OverlayRootfs
	}

	artifact := schema.ReleaseArtifact{}
	if containerImage != "" {
		artifact.ContainerImage = containerImage
	}

	return config, artifact
}

// buildUKI invokes AuroraBoot's pkg/uki library to produce a UKI ISO.
func (b *Builder) buildUKI(ctx context.Context, opts builder.BuildOptions, containerImage, outputDir string, logWriter *dbLogWriter) error {
	signing := opts.Signing
	if signing.UKISecureBootKey == "" || signing.UKISecureBootCert == "" || signing.UKITPMPCRKey == "" {
		return fmt.Errorf("UKI requires SecureBoot keys: sb-key, sb-cert, and tpm-pcr-private-key must all be provided")
	}

	log := sdklogger.NewKairosLogger("auroraboot-uki", "info", false)
	if logWriter != nil {
		log.Logger = log.Logger.Output(logWriter)
	}

	ukiOpts := uki.Options{
		Source:                 "docker:" + containerImage,
		OutputDir:              outputDir,
		OutputType:             string(constants.IsoOutput),
		Name:                   "kairos",
		SBKey:                  signing.UKISecureBootKey,
		SBCert:                 signing.UKISecureBootCert,
		TPMPCRPrivateKey:       signing.UKITPMPCRKey,
		PublicKeysDir:          signing.UKIPublicKeysDir,
		SecureBootEnroll:       signing.UKISecureBootEnroll,
		OverlayRootfs:          opts.OverlayRootfs,
		IncludeVersionInConfig: true,
		IncludeCmdlineInConfig: true,
		ExtendCmdline:          opts.ExtendCmdline,
		Logger:                 &log,
	}

	errCh := make(chan error, 1)
	go func() { errCh <- b.ukiBuildFn(ukiOpts) }()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// dockerBuild runs `docker build` when a Dockerfile is provided.
func (b *Builder) dockerBuild(ctx context.Context, opts builder.BuildOptions, outputDir string, logWriter *dbLogWriter) (string, error) {
	dockerfilePath := filepath.Join(outputDir, "Dockerfile")
	if err := os.WriteFile(dockerfilePath, []byte(opts.Dockerfile), 0o644); err != nil {
		return "", fmt.Errorf("writing Dockerfile: %w", err)
	}

	tag := fmt.Sprintf("auroraboot-build:%s", opts.ID)

	buildContext := opts.BuildContextDir
	if buildContext == "" {
		buildContext = outputDir
	}

	cmd := exec.CommandContext(ctx, "docker", "build", "--no-cache", "-t", tag, "-f", dockerfilePath, buildContext)
	if logWriter != nil {
		cmd.Stdout = logWriter
		cmd.Stderr = logWriter
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Run(); err != nil {
		return "", err
	}

	return tag, nil
}

// Status returns the current build status for a given ID.
func (b *Builder) Status(ctx context.Context, id string) (*builder.BuildStatus, error) {
	// Try DB first if store is available.
	if b.store != nil {
		rec, err := b.store.GetByID(ctx, id)
		if err == nil {
			return &builder.BuildStatus{
				ID:        rec.ID,
				Phase:     rec.Phase,
				Message:   rec.Message,
				Artifacts: rec.ArtifactFiles,
			}, nil
		}
	}

	// Fall back to in-memory.
	b.mu.RLock()
	defer b.mu.RUnlock()

	bs, ok := b.builds[id]
	if !ok {
		return nil, fmt.Errorf("build %q not found", id)
	}

	status := bs.status // copy
	return &status, nil
}

// List returns status for all builds.
func (b *Builder) List(ctx context.Context) ([]*builder.BuildStatus, error) {
	// Try DB first if store is available.
	if b.store != nil {
		records, err := b.store.List(ctx)
		if err == nil {
			result := make([]*builder.BuildStatus, 0, len(records))
			for _, rec := range records {
				result = append(result, &builder.BuildStatus{
					ID:        rec.ID,
					Phase:     rec.Phase,
					Message:   rec.Message,
					Artifacts: rec.ArtifactFiles,
				})
			}
			return result, nil
		}
	}

	// Fall back to in-memory.
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make([]*builder.BuildStatus, 0, len(b.builds))
	for _, bs := range b.builds {
		status := bs.status // copy
		result = append(result, &status)
	}
	return result, nil
}

// Cancel cancels a running build.
func (b *Builder) Cancel(_ context.Context, id string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	bs, ok := b.builds[id]
	if !ok {
		return fmt.Errorf("build %q not found", id)
	}

	bs.cancel()

	if bs.status.Phase == builder.BuildPending || bs.status.Phase == builder.BuildBuilding {
		bs.status.Phase = builder.BuildError
		bs.status.Message = "cancelled"
	}

	// Update DB if store is available.
	if b.store != nil {
		_ = b.updateDBPhase(context.Background(), id, store.ArtifactError, "cancelled")
	}

	return nil
}

// collectArtifacts returns only the final build output files (ISOs, disk images, checksums),
// skipping the unpacked rootfs and intermediate build files.
func collectArtifacts(dir string) []string {
	// Known artifact file extensions
	artifactExts := map[string]bool{
		".iso":    true,
		".raw":    true,
		".img":    true,
		".tar":    true,
		".tar.gz": true,
		".vhd":    true,
		".sha256":  true,
	}

	var artifacts []string
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		// Skip intermediate directories (unpacked rootfs, build temps)
		if info.IsDir() && (info.Name() == "temp-rootfs" || info.Name() == "build" || info.Name() == "rootfs") {
			return filepath.SkipDir
		}
		if info.IsDir() {
			return nil
		}
		name := info.Name()
		for ext := range artifactExts {
			if strings.HasSuffix(name, ext) {
				artifacts = append(artifacts, path)
				return nil
			}
		}
		return nil
	})
	return artifacts
}

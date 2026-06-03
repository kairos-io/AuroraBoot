// Package redfish provides a gofish-backed Deployer that performs spec-compliant
// Redfish virtual-media deployments: it discovers Systems/Managers (no hardcoded
// IDs), inserts media by URL (VirtualMedia.InsertMedia), sets a one-time boot
// override, resets the system with an explicit ResetType, and polls the returned
// Task to a terminal state. The gofish *APIClient is kept private; callers only
// ever see our own types.
package redfish

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/schemas"
)

// taskPollInterval is the delay between Task polls. Kept modest so a cancelled
// context aborts promptly.
const taskPollInterval = 3 * time.Second

// Deployer drives a Redfish virtual-media deployment against one BMC. Create it
// with NewDeployer, call Connect, defer Close (which logs the session out), then
// Inspect and/or Deploy. It is not safe for concurrent use.
type Deployer struct {
	endpoint  string
	username  string
	password  string
	vendor    VendorType
	verifySSL bool
	timeout   time.Duration

	// quirks is the per-vendor profile selected from vendor. For VendorGeneric it
	// is the zero-value (all-nil-hook) profile, so the default path is unchanged.
	quirks quirks

	// client is the live gofish connection. It is private by design: gofish types
	// must not leak into pkg/hardware or the CLI (decision D1 guardrail).
	client *gofish.APIClient
}

// Config holds the connection parameters for a Deployer.
type Config struct {
	Endpoint  string
	Username  string
	Password  string
	Vendor    VendorType
	VerifySSL bool
	Timeout   time.Duration
}

// NewDeployer builds a Deployer from a Config. It does not open a connection;
// call Connect for that. VerifySSL defaults to enforcing TLS verification: the
// only way to disable it is an explicit VerifySSL:false, so there is no silent
// downgrade.
func NewDeployer(cfg Config) *Deployer {
	vendor := cfg.Vendor
	if vendor == "" {
		vendor = VendorGeneric
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Minute
	}
	return &Deployer{
		endpoint:  cfg.Endpoint,
		username:  cfg.Username,
		password:  cfg.Password,
		vendor:    vendor,
		verifySSL: cfg.VerifySSL,
		timeout:   timeout,
		quirks:    quirksFor(vendor),
	}
}

// Connect opens an authenticated Redfish session. It creates the session with a
// JSON credential body (gofish does this by default) and obtains an X-Auth-Token;
// Close tears the session down. The supplied context bounds the connection setup.
func (d *Deployer) Connect(ctx context.Context) error {
	if d.client != nil {
		return errors.New("already connected")
	}

	cfg := gofish.ClientConfig{
		Endpoint: d.endpoint,
		Username: d.username,
		Password: d.password,
		// Insecure is the inverse of VerifySSL; verification stays on unless the
		// caller explicitly opted out.
		Insecure: !d.verifySSL,
		// Token/session auth (BasicAuth:false) so we get a deletable session.
		BasicAuth: false,
		// Inject a defensive HTTP client that rejects cross-origin redirects and
		// caps response-body size, so a compromised BMC cannot bounce our
		// credentialed client to another host or OOM us. We set NoModifyTransport
		// because our wrapping RoundTripper is not a bare *http.Transport, so gofish
		// must not try to mutate it — TLS verification is configured inside the
		// defensive client (mirroring Insecure) instead.
		HTTPClient:        newDefensiveHTTPClient(!d.verifySSL),
		NoModifyTransport: true,
	}

	client, err := gofish.ConnectContext(ctx, cfg)
	if err != nil {
		// gofish error strings can echo the endpoint but never the credentials;
		// scrub defensively in case a future version changes that.
		return fmt.Errorf("connecting to Redfish service: %w", d.scrub(err))
	}
	d.client = client
	return nil
}

// Close logs the Redfish session out (DELETE on the session resource) and drops
// the connection. It is safe to call on a never-connected or already-closed
// Deployer, and callers MUST defer it so the session is not leaked on either the
// success or the error path.
func (d *Deployer) Close() error {
	if d.client == nil {
		return nil
	}
	d.client.Logout()
	d.client = nil
	return nil
}

// Inspect discovers the primary ComputerSystem and returns its typed hardware
// summary. Memory and CPU come from the nested MemorySummary/ProcessorSummary
// (fixing the historical 0/0 bug).
func (d *Deployer) Inspect(ctx context.Context) (*SystemInfo, error) {
	if d.client == nil {
		return nil, errors.New("not connected: call Connect first")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	system, err := d.primarySystem()
	if err != nil {
		return nil, err
	}

	info := &SystemInfo{
		ID:           system.ID,
		Name:         system.Name,
		Model:        system.Model,
		Manufacturer: system.Manufacturer,
		SerialNumber: system.SerialNumber,
		PowerState:   string(system.PowerState),
		// Capabilities we could positively determine from the ComputerSystem (e.g.
		// UEFI boot support). Anything not detected is omitted, and the hardware
		// gate treats a required-but-undetected feature as unsupported.
		Features: detectFeatures(system),
	}
	if v := system.MemorySummary.TotalSystemMemoryGiB; v != nil {
		info.MemoryGiB = int(*v)
	}
	if v := system.ProcessorSummary.Count; v != nil {
		info.ProcessorCount = int(*v)
	}
	return info, nil
}

// Deploy performs the full virtual-media deployment flow against the discovered
// system: InsertMedia (URL-pull) -> one-time boot override -> Reset (with an
// explicit ResetType) -> Task poll to terminal. It honours context cancellation
// between and during steps.
func (d *Deployer) Deploy(ctx context.Context, req DeployRequest) (*DeployResult, error) {
	if d.client == nil {
		return nil, errors.New("not connected: call Connect first")
	}
	if strings.TrimSpace(req.ImageURL) == "" {
		return nil, errors.New("DeployRequest.ImageURL is required (InsertMedia is URL-pull)")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	report := newProgressReporter(req.Progress)

	result := &DeployResult{StartedAt: time.Now()}

	report("discovering", 10)
	system, err := d.primarySystem()
	if err != nil {
		return nil, err
	}
	result.SystemID = system.ID

	media, err := d.findCDMedia(system)
	if err != nil {
		return nil, err
	}
	result.MediaID = media.ID

	// 1. Insert media (URL-pull). The BMC fetches the ISO from ImageURL.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	report("inserting media", 30)
	insertTask, err := d.insertMedia(media, req)
	if err != nil {
		return nil, fmt.Errorf("inserting virtual media: %w", d.scrub(enrichRedfishError(err)))
	}

	// 2. One-time boot override (Cd/UEFI by default).
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	report("setting boot", 50)
	if err := d.setOneTimeBoot(system, req); err != nil {
		return nil, fmt.Errorf("setting one-time boot override: %w", d.scrub(enrichRedfishError(err)))
	}

	// 3. Reset with an explicit ResetType chosen from the current power state and
	//    the system's allowable values.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	report("resetting", 70)
	resetTask, err := d.reset(system, req)
	if err != nil {
		return nil, fmt.Errorf("resetting system: %w", d.scrub(enrichRedfishError(err)))
	}

	// 4. Poll any returned Task to a terminal state. Prefer the reset task (the
	//    operation that actually drives the install boot); fall back to the
	//    InsertMedia task if that is the only async one.
	taskURI := pickTaskURI(resetTask, insertTask)
	if taskURI != "" {
		report("polling task", 80)
		state, msgs, err := d.pollTask(ctx, taskURI, report)
		if err != nil {
			return nil, fmt.Errorf("polling deployment task: %w", d.scrub(err))
		}
		result.TaskCompleted = state == schemas.CompletedTaskState
		result.TaskState = string(state)
		result.Messages = msgs
		if state == schemas.ExceptionTaskState || state == schemas.KilledTaskState || state == schemas.CancelledTaskState {
			return result, fmt.Errorf("deployment task ended in state %q: %s", state, strings.Join(msgs, "; "))
		}
	}

	// 5. Optionally eject. Most flows leave the media mounted across the install
	//    reboot, so this is opt-in.
	if req.EjectAfter {
		if _, err := media.EjectMedia(); err != nil {
			return result, fmt.Errorf("ejecting virtual media: %w", d.scrub(err))
		}
	}

	report("completed", 100)
	result.FinishedAt = time.Now()
	return result, nil
}

// primarySystem discovers the Systems collection and returns the sole/first
// member. No IDs are hardcoded.
func (d *Deployer) primarySystem() (*schemas.ComputerSystem, error) {
	systems, err := d.client.GetService().Systems()
	if err != nil {
		return nil, fmt.Errorf("discovering systems: %w", d.scrub(err))
	}
	if len(systems) == 0 {
		return nil, errors.New("no ComputerSystem members found on the Redfish service")
	}
	return systems[0], nil
}

// findCDMedia locates a CD/DVD-capable VirtualMedia resource. Per the spec it may
// hang off the ComputerSystem or the Manager. The spec-default search order is
// System first, then each Manager; a vendor quirk profile (e.g. HPE iLO) may
// reorder this. A member with no advertised MediaTypes is accepted as a last
// resort (some BMCs omit the field).
func (d *Deployer) findCDMedia(system *schemas.ComputerSystem) (*schemas.VirtualMedia, error) {
	collections := d.defaultMediaCollections(system)
	if d.quirks.mediaSearch != nil {
		if reordered := d.quirks.mediaSearch(d, system, collections); reordered != nil {
			collections = reordered
		}
	}

	var fallback *schemas.VirtualMedia
	for _, media := range collections {
		for _, vm := range media {
			if mediaSupportsCD(vm) {
				return vm, nil
			}
			if fallback == nil && len(vm.MediaTypes) == 0 {
				fallback = vm
			}
		}
	}
	if fallback != nil {
		return fallback, nil
	}
	return nil, errors.New("no CD/DVD-capable VirtualMedia resource found on the system or its managers")
}

// defaultMediaCollections returns the spec-default VirtualMedia search set:
// System-hosted media first, then each Manager's media. quirk profiles receive
// this as their starting point.
func (d *Deployer) defaultMediaCollections(system *schemas.ComputerSystem) mediaCollections {
	collections := d.systemMediaCollections(system)
	collections = append(collections, d.managerMediaCollections()...)
	return collections
}

// systemMediaCollections returns the VirtualMedia hosted on the ComputerSystem
// (zero or one collection). Errors are swallowed: a missing System.VirtualMedia
// link simply means there is nothing to search there.
func (d *Deployer) systemMediaCollections(system *schemas.ComputerSystem) mediaCollections {
	if sysMedia, err := system.VirtualMedia(); err == nil {
		return mediaCollections{sysMedia}
	}
	return nil
}

// managerMediaCollections returns the VirtualMedia hosted on each Manager. This is
// where HPE iLO (and many BMCs) expose virtual media. Errors are swallowed per
// manager so one unreadable manager does not hide the others.
func (d *Deployer) managerMediaCollections() mediaCollections {
	var collections mediaCollections
	if managers, err := d.client.GetService().Managers(); err == nil {
		for _, m := range managers {
			if mgrMedia, err := m.VirtualMedia(); err == nil {
				collections = append(collections, mgrMedia)
			}
		}
	}
	return collections
}

// insertMedia performs the media insertion. By default this is the spec
// VirtualMedia.InsertMedia URL-pull action; a vendor profile may take over via the
// pushMedia hook (a stub seam for a future multipart push — no profile uses it
// today). The InsertMedia parameters and MediaType flow through the quirk profile
// so a vendor can tweak them without re-implementing the flow.
func (d *Deployer) insertMedia(media *schemas.VirtualMedia, req DeployRequest) (*schemas.TaskMonitorInfo, error) {
	// Future multipart-push extension point. No profile implements it yet; the
	// default (nil hook) always falls through to the URL-pull InsertMedia below.
	if d.quirks.pushMedia != nil {
		if handled, info, err := d.quirks.pushMedia(d, media, req); handled {
			return info, err
		}
	}

	inserted := true
	writeProtected := true

	mediaType := schemas.CDVirtualMediaType
	if d.quirks.mediaType != nil {
		mediaType = d.quirks.mediaType(media)
	}

	params := &schemas.VirtualMediaInsertMediaParameters{
		Image:          req.ImageURL,
		Inserted:       &inserted,
		WriteProtected: &writeProtected,
		MediaType:      mediaType,
	}
	protocol := schemas.HTTPTransferProtocolType
	if req.TransferProtocolHTTPS {
		protocol = schemas.HTTPSTransferProtocolType
	}
	params.TransferProtocolType = &protocol

	if d.quirks.tuneInsertParams != nil {
		d.quirks.tuneInsertParams(params, req)
	}

	return media.InsertMedia(params)
}

// setOneTimeBoot patches the system to boot once from the requested target/mode.
func (d *Deployer) setOneTimeBoot(system *schemas.ComputerSystem, req DeployRequest) error {
	target := bootSource(req.BootTarget)
	mode := bootMode(req.BootMode)

	boot := &schemas.Boot{
		BootSourceOverrideEnabled: schemas.OnceBootSourceOverrideEnabled,
		BootSourceOverrideTarget:  target,
		BootSourceOverrideMode:    mode,
	}
	return system.SetBoot(boot)
}

// reset powers/restarts the system using an explicit ResetType. The caller can
// force a ResetType; otherwise one is chosen from the current power state and
// validated against the system's allowable values.
func (d *Deployer) reset(system *schemas.ComputerSystem, req DeployRequest) (*schemas.TaskMonitorInfo, error) {
	rt := schemas.ResetType(req.ResetType)
	if rt == "" {
		rt = chooseResetType(system)
		// A vendor profile may prefer a different ResetType. It only runs when the
		// caller did not force one, so an explicit DeployRequest.ResetType always wins.
		if d.quirks.resetType != nil {
			rt = d.quirks.resetType(system, rt)
		}
	}
	return system.Reset(rt)
}

// pollTask GETs the Task at uri repeatedly until it reaches a terminal state or
// the context is cancelled. While polling it reports progress between
// taskPollFloor and taskPollCeil, scaling the Task's PercentComplete when present
// so callers see live movement during a long install boot.
func (d *Deployer) pollTask(ctx context.Context, uri string, report progressFunc) (schemas.TaskState, []string, error) {
	for {
		if err := ctx.Err(); err != nil {
			return "", nil, err
		}

		task, err := schemas.GetTask(d.client, uri)
		if err != nil {
			return "", nil, fmt.Errorf("getting task %s: %w", uri, d.scrub(enrichRedfishError(err)))
		}

		// Reject an unknown/garbage TaskState rather than looping forever on it. A
		// state outside the Redfish enum means the BMC returned something we cannot
		// reason about; the context still bounds the loop, but a clear error is far
		// more useful than silently spinning until the deadline.
		if !isKnownTaskState(task.TaskState) {
			return "", nil, fmt.Errorf("BMC returned an unknown Redfish TaskState %q for task %s", task.TaskState, uri)
		}

		report("polling task", taskPollPercent(task))

		if isTerminalTaskState(task.TaskState) {
			return task.TaskState, taskMessages(task), nil
		}

		select {
		case <-ctx.Done():
			return "", nil, ctx.Err()
		case <-time.After(taskPollInterval):
		}
	}
}

// scrub removes credentials from an error before it crosses the package boundary.
// gofish does not place credentials in error strings, but a creds-bearing URL
// could theoretically appear; redact basic-auth userinfo defensively.
func (d *Deployer) scrub(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if d.password != "" {
		msg = strings.ReplaceAll(msg, d.password, "[REDACTED]")
	}
	if msg == err.Error() {
		return err
	}
	return errors.New(msg)
}

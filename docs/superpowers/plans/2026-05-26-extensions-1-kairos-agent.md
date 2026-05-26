# Extensions — Plan 1 of 3: kairos-agent changes

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a new `extension` phonehome command (manual install/enable/disable/remove) and extend the existing `upgrade` / `upgrade-recovery` commands to install bundled extensions before the OS upgrade reboot. Ship as a tagged kairos-agent release that AuroraBoot will vendor.

**Architecture:** All work lives in `kairos-agent/internal/phonehome/`. A new sibling file `handlers_extension.go` holds the new dispatch logic so `handlers.go` stays focused. The dispatch shells out via `exec.Command("kairos-agent", ...)` to invoke the existing `sysext|confext` CLI subcommands (matches the pattern that `handleUpgrade` already uses for `kairos-agent upgrade`). For compound upgrades, the `extensions` field is JSON-encoded into the existing `CommandData.Args map[string]string` (no breaking schema change).

**Tech Stack:** Go 1.23+, urfave/cli v2, ginkgo/gomega for tests (existing repo conventions).

**Repository:** `/home/mudler/_git/kairos-agent`

---

## Reference: existing surfaces (read these before starting)

| File | What it does |
|---|---|
| `internal/phonehome/config.go:140-144` | `CommandData` shape: `ID`, `Command`, `Args map[string]string`. |
| `internal/phonehome/handlers.go:28-65` | `DefaultCommandHandler` — the switch statement we extend. |
| `internal/phonehome/handlers.go:89-164` | `handleUpgrade` — we extend this to install bundled extensions first. |
| `pkg/action/sysext.go:35-52` | Storage layout constants (sysextDir, confExtDir, scope sub-dirs). |
| `pkg/action/sysext.go:154-218` | `EnableExtension` — idempotent symlink creation. |
| `pkg/action/sysext.go:283-305` | `InstallExtension` — download into the persistent dir. |
| `pkg/action/sysext.go:311-367` | `RemoveExtension` — symlink + .raw teardown. |
| `main.go:1222-1488` | The sysext/confext subcommands wired into the CLI; verify flag names and order before shelling out from the handler. |
| `internal/phonehome/handlers_test.go` (if exists) or `internal/phonehome/config_test.go:130-200` | Existing test patterns for the dispatcher. |

---

## Task 1: New file scaffold and `ExtensionArgs` type

**Files:**
- Create: `internal/phonehome/handlers_extension.go`
- Create: `internal/phonehome/handlers_extension_test.go`

- [ ] **Step 1: Write the failing test for `parseExtensionArgs`**

Create `internal/phonehome/handlers_extension_test.go` with:

```go
package phonehome

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestParseExtensionArgs_Install(t *testing.T) {
	g := NewWithT(t)
	args := map[string]string{
		"type":      "sysext",
		"action":    "install",
		"name":      "tailscale-agent",
		"source":    "https://aurora/api/v1/extensions/abc/download/tailscale-agent.sysext.raw?token=k",
		"bootState": "common",
		"now":       "true",
	}
	got, err := parseExtensionArgs(args)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(got.Type).To(Equal("sysext"))
	g.Expect(got.Action).To(Equal("install"))
	g.Expect(got.Name).To(Equal("tailscale-agent"))
	g.Expect(got.BootState).To(Equal("common"))
	g.Expect(got.Now).To(BeTrue())
}

func TestParseExtensionArgs_MissingType(t *testing.T) {
	g := NewWithT(t)
	_, err := parseExtensionArgs(map[string]string{"action": "install"})
	g.Expect(err).To(MatchError(ContainSubstring("type")))
}

func TestParseExtensionArgs_InvalidType(t *testing.T) {
	g := NewWithT(t)
	_, err := parseExtensionArgs(map[string]string{"type": "blob", "action": "install", "name": "x"})
	g.Expect(err).To(MatchError(ContainSubstring("type")))
}

func TestParseExtensionArgs_MissingActionRequiredField(t *testing.T) {
	g := NewWithT(t)
	// install requires source
	_, err := parseExtensionArgs(map[string]string{"type": "sysext", "action": "install", "name": "x", "bootState": "common"})
	g.Expect(err).To(MatchError(ContainSubstring("source")))

	// enable/disable require bootState
	_, err = parseExtensionArgs(map[string]string{"type": "sysext", "action": "enable", "name": "x"})
	g.Expect(err).To(MatchError(ContainSubstring("bootState")))

	// every action requires name
	_, err = parseExtensionArgs(map[string]string{"type": "sysext", "action": "remove"})
	g.Expect(err).To(MatchError(ContainSubstring("name")))
}
```

- [ ] **Step 2: Run the test to verify it fails**

```
cd ~/_git/kairos-agent && go test ./internal/phonehome/... -run TestParseExtensionArgs -v
```

Expected: FAIL with "undefined: parseExtensionArgs"

- [ ] **Step 3: Implement `parseExtensionArgs`**

Create `internal/phonehome/handlers_extension.go`:

```go
package phonehome

import (
	"fmt"
)

// ExtensionArgs is the validated, typed shape of an `extension` command's args.
type ExtensionArgs struct {
	Type      string // "sysext" | "confext"
	Action    string // "install" | "enable" | "disable" | "remove"
	Name      string
	Source    string // required for action=install
	BootState string // required for action in {install,enable,disable}; "active"|"passive"|"recovery"|"common"
	Now       bool   // optional; default false
}

func parseExtensionArgs(in map[string]string) (ExtensionArgs, error) {
	out := ExtensionArgs{
		Type:      in["type"],
		Action:    in["action"],
		Name:      in["name"],
		Source:    in["source"],
		BootState: in["bootState"],
		Now:       in["now"] == "true",
	}
	if out.Type != "sysext" && out.Type != "confext" {
		return out, fmt.Errorf("extension: unsupported type %q (want sysext or confext)", out.Type)
	}
	switch out.Action {
	case "install", "enable", "disable", "remove":
	default:
		return out, fmt.Errorf("extension: unsupported action %q (want install|enable|disable|remove)", out.Action)
	}
	if out.Name == "" {
		return out, fmt.Errorf("extension: name is required")
	}
	if out.Action == "install" && out.Source == "" {
		return out, fmt.Errorf("extension: source is required for action=install")
	}
	if (out.Action == "install" || out.Action == "enable" || out.Action == "disable") && out.BootState == "" {
		return out, fmt.Errorf("extension: bootState is required for action=%s", out.Action)
	}
	switch out.BootState {
	case "", "active", "passive", "recovery", "common":
	default:
		return out, fmt.Errorf("extension: unsupported bootState %q", out.BootState)
	}
	return out, nil
}
```

- [ ] **Step 4: Run the tests, verify they pass**

```
cd ~/_git/kairos-agent && go test ./internal/phonehome/... -run TestParseExtensionArgs -v
```

Expected: PASS (4 subtests).

- [ ] **Step 5: Commit**

```bash
cd ~/_git/kairos-agent
git add internal/phonehome/handlers_extension.go internal/phonehome/handlers_extension_test.go
git commit -m "phonehome: add ExtensionArgs parser for new extension command"
```

---

## Task 2: Dispatch skeleton wired into `DefaultCommandHandler`

**Files:**
- Modify: `internal/phonehome/handlers.go` (add case to switch at lines 36-63)
- Modify: `internal/phonehome/handlers_extension.go` (add stub)
- Modify: `internal/phonehome/handlers_extension_test.go` (add dispatch test)

- [ ] **Step 1: Write a failing test for the dispatch wiring**

Append to `internal/phonehome/handlers_extension_test.go`:

```go
func TestDispatchExtension_PolicyGate(t *testing.T) {
	g := NewWithT(t)
	// isAllowed returns false: extension command must be rejected.
	denyAll := func(string) bool { return false }
	handler := DefaultCommandHandler("http://example", func() string { return "" }, denyAll, nil)
	_, err := handler(CommandData{ID: "c1", Command: "extension",
		Args: map[string]string{"type": "sysext", "action": "remove", "name": "x"}})
	g.Expect(err).To(MatchError(ContainSubstring("not permitted")))
}

func TestDispatchExtension_BadArgs(t *testing.T) {
	g := NewWithT(t)
	allow := func(string) bool { return true }
	handler := DefaultCommandHandler("http://example", func() string { return "" }, allow, nil)
	_, err := handler(CommandData{ID: "c1", Command: "extension",
		Args: map[string]string{"type": "wat"}})
	g.Expect(err).To(MatchError(ContainSubstring("unsupported type")))
}
```

- [ ] **Step 2: Run the test to verify it fails**

```
cd ~/_git/kairos-agent && go test ./internal/phonehome/... -run TestDispatchExtension -v
```

Expected: FAIL — `extension` falls through to the `default` arm and returns "unknown command".

- [ ] **Step 3: Add the switch case and a stub `handleExtension`**

In `internal/phonehome/handlers.go`, in the switch in `DefaultCommandHandler`, add **before** the `default:` arm (existing line 61):

```go
		case "extension":
			return handleExtension(ctx, cmd)
```

In `internal/phonehome/handlers_extension.go`, append:

```go
import (
	"context"
)

// handleExtension dispatches the manual-flow extension command. Stub for now;
// each action is implemented in subsequent tasks.
func handleExtension(ctx context.Context, cmd CommandData) (string, error) {
	args, err := parseExtensionArgs(cmd.Args)
	if err != nil {
		return "", err
	}
	_ = ctx
	_ = args
	return "", fmt.Errorf("extension: action %q not yet implemented", args.Action)
}
```

(Add `"context"` to the existing imports if not already there.)

- [ ] **Step 4: Run the dispatch tests, verify the policy + bad-args tests pass and a third lookup test now returns "not yet implemented"**

```
cd ~/_git/kairos-agent && go test ./internal/phonehome/... -run TestDispatchExtension -v
```

Expected: both tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/phonehome/handlers.go internal/phonehome/handlers_extension.go internal/phonehome/handlers_extension_test.go
git commit -m "phonehome: wire `extension` command into DefaultCommandHandler"
```

---

## Task 3: Implement `install` action (download + enable)

**Files:**
- Modify: `internal/phonehome/handlers_extension.go`
- Modify: `internal/phonehome/handlers_extension_test.go`

The `install` action requires two CLI calls because `kairos-agent sysext install` only downloads — `enable` creates the symlink (see `pkg/action/sysext.go:283-305` vs `:154-218`).

- [ ] **Step 1: Write the failing test for the install command shape**

Append to `internal/phonehome/handlers_extension_test.go`:

```go
import (
	// ...existing imports plus:
	"os/exec"
)

// commandRecorder replaces exec.Command for tests, capturing the args and
// returning a synthetic command that always succeeds.
type commandRecorder struct {
	calls [][]string
}

func (r *commandRecorder) record(name string, args ...string) *exec.Cmd {
	r.calls = append(r.calls, append([]string{name}, args...))
	// Use /bin/true so the resulting command exits 0 immediately.
	return exec.Command("/bin/true")
}

func TestHandleExtension_Install_BuildsCorrectCLIArgs(t *testing.T) {
	g := NewWithT(t)
	rec := &commandRecorder{}
	prev := execCommand
	execCommand = rec.record
	t.Cleanup(func() { execCommand = prev })

	cmd := CommandData{
		ID:      "c1",
		Command: "extension",
		Args: map[string]string{
			"type": "sysext", "action": "install",
			"name":   "tailscale-agent",
			"source": "https://aurora/api/v1/extensions/abc/download/tailscale-agent.sysext.raw?token=k",
			"bootState": "common", "now": "true",
		},
	}
	out, err := handleExtension(context.Background(), cmd)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(out).To(ContainSubstring("installed"))
	g.Expect(rec.calls).To(HaveLen(2))
	g.Expect(rec.calls[0]).To(Equal([]string{"kairos-agent", "sysext", "install",
		"https://aurora/api/v1/extensions/abc/download/tailscale-agent.sysext.raw?token=k"}))
	g.Expect(rec.calls[1]).To(Equal([]string{"kairos-agent", "sysext", "enable",
		"tailscale-agent", "--common", "--now"}))
}

func TestHandleExtension_Install_OmitsNowFlagWhenFalse(t *testing.T) {
	g := NewWithT(t)
	rec := &commandRecorder{}
	prev := execCommand
	execCommand = rec.record
	t.Cleanup(func() { execCommand = prev })

	_, err := handleExtension(context.Background(), CommandData{
		Command: "extension",
		Args: map[string]string{
			"type": "confext", "action": "install",
			"name": "fluent-bit-config",
			"source": "https://x/file?token=k",
			"bootState": "active",
		},
	})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(rec.calls).To(HaveLen(2))
	g.Expect(rec.calls[1]).To(Equal([]string{"kairos-agent", "confext", "enable",
		"fluent-bit-config", "--active"}))
}
```

- [ ] **Step 2: Run the tests, verify they fail**

```
cd ~/_git/kairos-agent && go test ./internal/phonehome/... -run TestHandleExtension_Install -v
```

Expected: FAIL — `execCommand` variable doesn't exist, `handleExtension` returns "not yet implemented".

- [ ] **Step 3: Introduce the `execCommand` indirection and implement install**

Replace the stub `handleExtension` in `internal/phonehome/handlers_extension.go` with:

```go
package phonehome

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// execCommand is a seam for tests to capture the shell-outs.
var execCommand = exec.Command

// ExtensionArgs ... (keep existing)

func parseExtensionArgs(in map[string]string) (ExtensionArgs, error) { /* existing */ }

func handleExtension(ctx context.Context, cmd CommandData) (string, error) {
	args, err := parseExtensionArgs(cmd.Args)
	if err != nil {
		return "", err
	}
	switch args.Action {
	case "install":
		return extInstall(ctx, args)
	case "enable":
		return extToggle(ctx, args, "enable")
	case "disable":
		return extToggle(ctx, args, "disable")
	case "remove":
		return extRemove(ctx, args)
	default:
		return "", fmt.Errorf("extension: unsupported action %q", args.Action)
	}
}

// extInstall is install + enable. kairos-agent's `install` only downloads the
// .raw; `enable` creates the symlink under the chosen scope. We do both so
// AuroraBoot's "Install" action card is one atomic round-trip from the operator's view.
func extInstall(ctx context.Context, a ExtensionArgs) (string, error) {
	// 1) download / overwrite the .raw
	out1, err := runCLI(ctx, a.Type, "install", a.Source)
	if err != nil {
		return out1, fmt.Errorf("extension install: %w: %s", err, out1)
	}
	// 2) enable for the chosen scope
	enableArgs := []string{a.Type, "enable", a.Name, "--" + a.BootState}
	if a.Now {
		enableArgs = append(enableArgs, "--now")
	}
	out2, err := runCLI(ctx, enableArgs...)
	if err != nil {
		return out1 + "\n" + out2, fmt.Errorf("extension enable: %w: %s", err, out2)
	}
	return fmt.Sprintf("Extension %s installed and enabled in %s\n%s\n%s",
		a.Name, a.BootState, strings.TrimSpace(out1), strings.TrimSpace(out2)), nil
}

func extToggle(ctx context.Context, a ExtensionArgs, action string) (string, error) {
	cliArgs := []string{a.Type, action, a.Name, "--" + a.BootState}
	if a.Now {
		cliArgs = append(cliArgs, "--now")
	}
	out, err := runCLI(ctx, cliArgs...)
	if err != nil {
		return out, fmt.Errorf("extension %s: %w: %s", action, err, out)
	}
	return strings.TrimSpace(out), nil
}

func extRemove(ctx context.Context, a ExtensionArgs) (string, error) {
	cliArgs := []string{a.Type, "remove", a.Name}
	if a.Now {
		cliArgs = append(cliArgs, "--now")
	}
	out, err := runCLI(ctx, cliArgs...)
	if err != nil {
		return out, fmt.Errorf("extension remove: %w: %s", err, out)
	}
	return strings.TrimSpace(out), nil
}

func runCLI(ctx context.Context, args ...string) (string, error) {
	cmd := execCommand("kairos-agent", args...)
	// execCommand returns *exec.Cmd; the ctx is informational here (we don't
	// kill mid-install — the existing handleUpgrade pattern uses background
	// context too because the upgrade must survive WS disconnects).
	_ = ctx
	out, err := cmd.CombinedOutput()
	return string(out), err
}
```

- [ ] **Step 4: Run the tests, verify they pass**

```
cd ~/_git/kairos-agent && go test ./internal/phonehome/... -run TestHandleExtension_Install -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/phonehome/handlers_extension.go internal/phonehome/handlers_extension_test.go
git commit -m "phonehome: implement extension install (download + enable)"
```

---

## Task 4: Tests for `enable`, `disable`, `remove`

**Files:**
- Modify: `internal/phonehome/handlers_extension_test.go`

The implementation already exists from Task 3 (in `extToggle` and `extRemove`); this task adds the coverage.

- [ ] **Step 1: Write the failing tests for enable/disable/remove**

Append to `internal/phonehome/handlers_extension_test.go`:

```go
func TestHandleExtension_Enable(t *testing.T) {
	g := NewWithT(t)
	rec := &commandRecorder{}
	prev := execCommand
	execCommand = rec.record
	t.Cleanup(func() { execCommand = prev })

	_, err := handleExtension(context.Background(), CommandData{
		Command: "extension",
		Args: map[string]string{
			"type": "sysext", "action": "enable",
			"name": "tailscale-agent", "bootState": "passive", "now": "false",
		},
	})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(rec.calls).To(Equal([][]string{
		{"kairos-agent", "sysext", "enable", "tailscale-agent", "--passive"},
	}))
}

func TestHandleExtension_Disable(t *testing.T) {
	g := NewWithT(t)
	rec := &commandRecorder{}
	prev := execCommand
	execCommand = rec.record
	t.Cleanup(func() { execCommand = prev })

	_, err := handleExtension(context.Background(), CommandData{
		Command: "extension",
		Args: map[string]string{
			"type": "confext", "action": "disable",
			"name": "fluent-bit-config", "bootState": "common", "now": "true",
		},
	})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(rec.calls).To(Equal([][]string{
		{"kairos-agent", "confext", "disable", "fluent-bit-config", "--common", "--now"},
	}))
}

func TestHandleExtension_Remove(t *testing.T) {
	g := NewWithT(t)
	rec := &commandRecorder{}
	prev := execCommand
	execCommand = rec.record
	t.Cleanup(func() { execCommand = prev })

	_, err := handleExtension(context.Background(), CommandData{
		Command: "extension",
		Args: map[string]string{
			"type": "sysext", "action": "remove",
			"name": "tailscale-agent", "now": "true",
		},
	})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(rec.calls).To(Equal([][]string{
		{"kairos-agent", "sysext", "remove", "tailscale-agent", "--now"},
	}))
}
```

- [ ] **Step 2: Run the tests, verify they pass**

```
cd ~/_git/kairos-agent && go test ./internal/phonehome/... -run TestHandleExtension -v
```

Expected: all four `TestHandleExtension_*` PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/phonehome/handlers_extension_test.go
git commit -m "phonehome: tests for extension enable/disable/remove"
```

---

## Task 5: Failure path — install/enable error aborts cleanly

**Files:**
- Modify: `internal/phonehome/handlers_extension_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/phonehome/handlers_extension_test.go`:

```go
// failingRecorder lets the test request a specific call to fail.
type failingRecorder struct {
	calls   [][]string
	failOn  int // index (0-based) of the call to fail; -1 means never
}

func (r *failingRecorder) record(name string, args ...string) *exec.Cmd {
	idx := len(r.calls)
	r.calls = append(r.calls, append([]string{name}, args...))
	if r.failOn == idx {
		// /bin/false exits 1 with no output.
		return exec.Command("/bin/false")
	}
	return exec.Command("/bin/true")
}

func TestHandleExtension_InstallAbortsIfDownloadFails(t *testing.T) {
	g := NewWithT(t)
	rec := &failingRecorder{failOn: 0}
	prev := execCommand
	execCommand = rec.record
	t.Cleanup(func() { execCommand = prev })

	_, err := handleExtension(context.Background(), CommandData{
		Command: "extension",
		Args: map[string]string{
			"type": "sysext", "action": "install",
			"name": "x", "source": "https://x/y", "bootState": "common",
		},
	})
	g.Expect(err).To(MatchError(ContainSubstring("extension install")))
	g.Expect(rec.calls).To(HaveLen(1)) // enable NOT attempted after install failure
}

func TestHandleExtension_InstallAbortsIfEnableFails(t *testing.T) {
	g := NewWithT(t)
	rec := &failingRecorder{failOn: 1}
	prev := execCommand
	execCommand = rec.record
	t.Cleanup(func() { execCommand = prev })

	_, err := handleExtension(context.Background(), CommandData{
		Command: "extension",
		Args: map[string]string{
			"type": "sysext", "action": "install",
			"name": "x", "source": "https://x/y", "bootState": "common",
		},
	})
	g.Expect(err).To(MatchError(ContainSubstring("extension enable")))
	g.Expect(rec.calls).To(HaveLen(2))
}
```

- [ ] **Step 2: Run the tests, verify they pass**

```
cd ~/_git/kairos-agent && go test ./internal/phonehome/... -run TestHandleExtension -v
```

Expected: both new tests PASS — the existing implementation already returns errors on non-zero CLI exit.

- [ ] **Step 3: Commit**

```bash
git add internal/phonehome/handlers_extension_test.go
git commit -m "phonehome: tests for extension install error paths"
```

---

## Task 6: Extend `handleUpgrade` — parse `args[\"extensions\"]`

**Files:**
- Modify: `internal/phonehome/handlers.go` (the existing `handleUpgrade` at lines 89-164)
- Modify: `internal/phonehome/handlers_extension.go` (add `BundledExtension` + parser)
- Modify: `internal/phonehome/handlers_extension_test.go`

- [ ] **Step 1: Write the failing test for the bundle parser**

Append to `internal/phonehome/handlers_extension_test.go`:

```go
import (
	// ... existing imports
	"encoding/json"
)

func TestParseBundledExtensions_Empty(t *testing.T) {
	g := NewWithT(t)
	got, err := parseBundledExtensions("")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(got).To(BeEmpty())
}

func TestParseBundledExtensions_Valid(t *testing.T) {
	g := NewWithT(t)
	raw, _ := json.Marshal([]BundledExtension{
		{Type: "sysext", Name: "tailscale-agent", Source: "https://x/a"},
		{Type: "confext", Name: "fluent-bit-config", Source: "https://x/b"},
	})
	got, err := parseBundledExtensions(string(raw))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(got).To(HaveLen(2))
	g.Expect(got[0].Name).To(Equal("tailscale-agent"))
	g.Expect(got[1].Type).To(Equal("confext"))
}

func TestParseBundledExtensions_RejectsBadType(t *testing.T) {
	g := NewWithT(t)
	_, err := parseBundledExtensions(`[{"type":"blob","name":"x","source":"https://x"}]`)
	g.Expect(err).To(MatchError(ContainSubstring("type")))
}

func TestParseBundledExtensions_RequiresFields(t *testing.T) {
	g := NewWithT(t)
	_, err := parseBundledExtensions(`[{"type":"sysext","source":"https://x"}]`)
	g.Expect(err).To(MatchError(ContainSubstring("name")))
	_, err = parseBundledExtensions(`[{"type":"sysext","name":"x"}]`)
	g.Expect(err).To(MatchError(ContainSubstring("source")))
}
```

- [ ] **Step 2: Run the tests, verify they fail**

```
cd ~/_git/kairos-agent && go test ./internal/phonehome/... -run TestParseBundledExtensions -v
```

Expected: FAIL — `BundledExtension` and `parseBundledExtensions` don't exist.

- [ ] **Step 3: Implement the bundle parser**

Append to `internal/phonehome/handlers_extension.go`:

```go
import (
	"encoding/json"
	// ... existing
)

// BundledExtension is one entry inside the upgrade command's `extensions` arg.
type BundledExtension struct {
	Type   string `json:"type"`   // "sysext" | "confext"
	Name   string `json:"name"`
	Source string `json:"source"` // download URI
}

// parseBundledExtensions reads the JSON-encoded array passed in
// CommandData.Args["extensions"]. Empty string => empty list, no error.
func parseBundledExtensions(raw string) ([]BundledExtension, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var list []BundledExtension
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, fmt.Errorf("extensions arg: %w", err)
	}
	for i, e := range list {
		if e.Type != "sysext" && e.Type != "confext" {
			return nil, fmt.Errorf("extensions[%d]: unsupported type %q", i, e.Type)
		}
		if e.Name == "" {
			return nil, fmt.Errorf("extensions[%d]: name is required", i)
		}
		if e.Source == "" {
			return nil, fmt.Errorf("extensions[%d]: source is required", i)
		}
	}
	return list, nil
}
```

- [ ] **Step 4: Run the tests, verify they pass**

```
cd ~/_git/kairos-agent && go test ./internal/phonehome/... -run TestParseBundledExtensions -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/phonehome/handlers_extension.go internal/phonehome/handlers_extension_test.go
git commit -m "phonehome: parse bundled extensions arg for compound upgrade"
```

---

## Task 7: Compound upgrade — install + conditional enable + abort policy

**Files:**
- Modify: `internal/phonehome/handlers.go` (extend `handleUpgrade` lines 89-164)
- Modify: `internal/phonehome/handlers_extension.go` (add `installBundledExtension` helper + `extensionEnabledAnywhere`)
- Modify: `internal/phonehome/handlers_extension_test.go`

This is the meat of the bundled flow. For each extension entry: install (download/overwrite), then check if it's already enabled at any scope on the node — only enable `--active` (without `--now`) if not already enabled anywhere.

- [ ] **Step 1: Write the failing test for `extensionEnabledAnywhere`**

Append to `internal/phonehome/handlers_extension_test.go`:

```go
import (
	// ... existing
	"os"
	"path/filepath"
)

// withTempPersistentDir sets a fake /var/lib/kairos/extensions root under t.TempDir
// and returns the path. The test patches extensionsPersistentRoot for the duration.
func withTempPersistentDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	prev := extensionsPersistentRoot
	extensionsPersistentRoot = func(extType string) string {
		return filepath.Join(dir, extType+"s") // matches kairos-agent's "extensions"/"confexts" pattern
	}
	t.Cleanup(func() { extensionsPersistentRoot = prev })
	return dir
}

func TestExtensionEnabledAnywhere_None(t *testing.T) {
	g := NewWithT(t)
	withTempPersistentDir(t)
	g.Expect(extensionEnabledAnywhere("sysext", "tailscale-agent")).To(BeFalse())
}

func TestExtensionEnabledAnywhere_PresentInActive(t *testing.T) {
	g := NewWithT(t)
	root := withTempPersistentDir(t)
	scopeDir := filepath.Join(root, "sysexts", "active")
	g.Expect(os.MkdirAll(scopeDir, 0o755)).To(Succeed())
	g.Expect(os.WriteFile(filepath.Join(scopeDir, "tailscale-agent.sysext.raw"), nil, 0o644)).To(Succeed())
	g.Expect(extensionEnabledAnywhere("sysext", "tailscale-agent")).To(BeTrue())
}

func TestExtensionEnabledAnywhere_PresentInCommon(t *testing.T) {
	g := NewWithT(t)
	root := withTempPersistentDir(t)
	scopeDir := filepath.Join(root, "sysexts", "common")
	g.Expect(os.MkdirAll(scopeDir, 0o755)).To(Succeed())
	g.Expect(os.WriteFile(filepath.Join(scopeDir, "tailscale-agent.sysext.raw"), nil, 0o644)).To(Succeed())
	g.Expect(extensionEnabledAnywhere("sysext", "tailscale-agent")).To(BeTrue())
}
```

- [ ] **Step 2: Run the tests, verify they fail**

```
cd ~/_git/kairos-agent && go test ./internal/phonehome/... -run TestExtensionEnabledAnywhere -v
```

Expected: FAIL — `extensionsPersistentRoot` and `extensionEnabledAnywhere` don't exist.

- [ ] **Step 3: Implement the persistent-dir scanner**

Append to `internal/phonehome/handlers_extension.go`:

```go
import (
	// ... existing
	"os"
	"path/filepath"
)

// extensionsPersistentRoot returns the persistent-dir base path for the given
// extension type. Test seam.
//
// Production layout (see kairos-agent/pkg/action/sysext.go:35-52):
//   /var/lib/kairos/extensions/{active,passive,recovery,common}/   (sysext)
//   /var/lib/kairos/confexts/{active,passive,recovery,common}/    (confext)
var extensionsPersistentRoot = func(extType string) string {
	if extType == "confext" {
		return "/var/lib/kairos/confexts"
	}
	return "/var/lib/kairos/extensions"
}

// extensionEnabledAnywhere reports whether a symlink for the named extension
// exists in any of the four scope dirs. Pattern match is by filename prefix
// since the on-disk filename is "<name>.<type>.raw" (per auroraboot sysext
// output convention).
func extensionEnabledAnywhere(extType, name string) bool {
	base := extensionsPersistentRoot(extType)
	for _, scope := range []string{"active", "passive", "recovery", "common"} {
		entries, err := os.ReadDir(filepath.Join(base, scope))
		if err != nil {
			continue // missing scope dir is fine
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if strings.HasPrefix(e.Name(), name+".") {
				return true
			}
		}
	}
	return false
}
```

- [ ] **Step 4: Run the scanner tests, verify they pass**

```
cd ~/_git/kairos-agent && go test ./internal/phonehome/... -run TestExtensionEnabledAnywhere -v
```

Expected: PASS.

- [ ] **Step 5: Write the failing test for `installBundledExtension`**

Append to `internal/phonehome/handlers_extension_test.go`:

```go
func TestInstallBundledExtension_NewExtensionEnablesActive(t *testing.T) {
	g := NewWithT(t)
	withTempPersistentDir(t) // empty -> not enabled anywhere
	rec := &commandRecorder{}
	prev := execCommand
	execCommand = rec.record
	t.Cleanup(func() { execCommand = prev })

	err := installBundledExtension(context.Background(), BundledExtension{
		Type: "sysext", Name: "tailscale-agent", Source: "https://x/y",
	})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(rec.calls).To(Equal([][]string{
		{"kairos-agent", "sysext", "install", "https://x/y"},
		{"kairos-agent", "sysext", "enable", "tailscale-agent", "--active"},
	}))
}

func TestInstallBundledExtension_ExistingExtensionSkipsEnable(t *testing.T) {
	g := NewWithT(t)
	root := withTempPersistentDir(t)
	scopeDir := filepath.Join(root, "sysexts", "common")
	g.Expect(os.MkdirAll(scopeDir, 0o755)).To(Succeed())
	g.Expect(os.WriteFile(filepath.Join(scopeDir, "tailscale-agent.sysext.raw"), nil, 0o644)).To(Succeed())

	rec := &commandRecorder{}
	prev := execCommand
	execCommand = rec.record
	t.Cleanup(func() { execCommand = prev })

	err := installBundledExtension(context.Background(), BundledExtension{
		Type: "sysext", Name: "tailscale-agent", Source: "https://x/y",
	})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(rec.calls).To(Equal([][]string{
		{"kairos-agent", "sysext", "install", "https://x/y"},
	}))
}
```

- [ ] **Step 6: Implement `installBundledExtension`**

Append to `internal/phonehome/handlers_extension.go`:

```go
// installBundledExtension downloads the .raw (overwriting if same name) and
// enables it at --active scope iff the extension is not already enabled at any
// scope. --now is intentionally omitted: the OS upgrade about to reboot will
// pick the extension up on the new active boot.
func installBundledExtension(ctx context.Context, e BundledExtension) error {
	if out, err := runCLI(ctx, e.Type, "install", e.Source); err != nil {
		return fmt.Errorf("install %s/%s: %w: %s", e.Type, e.Name, err, out)
	}
	if extensionEnabledAnywhere(e.Type, e.Name) {
		return nil // preserve operator's prior scope choice
	}
	if out, err := runCLI(ctx, e.Type, "enable", e.Name, "--active"); err != nil {
		return fmt.Errorf("enable %s/%s --active: %w: %s", e.Type, e.Name, err, out)
	}
	return nil
}
```

- [ ] **Step 7: Run the bundle-install tests, verify they pass**

```
cd ~/_git/kairos-agent && go test ./internal/phonehome/... -run TestInstallBundledExtension -v
```

Expected: PASS.

- [ ] **Step 8: Write the failing test for the extended `handleUpgrade`**

Add to `internal/phonehome/handlers_extension_test.go`:

```go
func TestHandleUpgrade_NoExtensions_BackwardCompat(t *testing.T) {
	g := NewWithT(t)
	withTempPersistentDir(t)
	rec := &commandRecorder{}
	prev := execCommand
	execCommand = rec.record
	t.Cleanup(func() { execCommand = prev })
	// scheduleReboot is best-effort; replace with no-op for test.
	prevR := scheduleRebootFn
	scheduleRebootFn = func() {}
	t.Cleanup(func() { scheduleRebootFn = prevR })

	_, err := handleUpgrade(context.Background(), CommandData{
		Command: "upgrade",
		Args:    map[string]string{"source": "oci:quay.io/myorg/edge-os:v4.2.0"},
	}, "http://example", "key")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(rec.calls).To(HaveLen(1)) // only the upgrade itself
	g.Expect(rec.calls[0]).To(ContainElement("upgrade"))
}

func TestHandleUpgrade_InstallsBundleBeforeUpgrade(t *testing.T) {
	g := NewWithT(t)
	withTempPersistentDir(t)
	rec := &commandRecorder{}
	prev := execCommand
	execCommand = rec.record
	t.Cleanup(func() { execCommand = prev })
	prevR := scheduleRebootFn
	scheduleRebootFn = func() {}
	t.Cleanup(func() { scheduleRebootFn = prevR })

	raw, _ := json.Marshal([]BundledExtension{
		{Type: "sysext", Name: "tailscale-agent", Source: "https://x/a"},
		{Type: "confext", Name: "fluent-bit-config", Source: "https://x/b"},
	})
	_, err := handleUpgrade(context.Background(), CommandData{
		Command: "upgrade",
		Args: map[string]string{
			"source":     "oci:quay.io/myorg/edge-os:v4.2.0",
			"extensions": string(raw),
		},
	}, "http://example", "key")
	g.Expect(err).ToNot(HaveOccurred())
	// Order: install ext1, enable ext1 (new), install ext2, enable ext2 (new), then upgrade.
	g.Expect(rec.calls).To(HaveLen(5))
	g.Expect(rec.calls[0][1:3]).To(Equal([]string{"sysext", "install"}))
	g.Expect(rec.calls[1][1:3]).To(Equal([]string{"sysext", "enable"}))
	g.Expect(rec.calls[2][1:3]).To(Equal([]string{"confext", "install"}))
	g.Expect(rec.calls[3][1:3]).To(Equal([]string{"confext", "enable"}))
	g.Expect(rec.calls[4][:1]).To(Equal([]string{"kairos-agent"}))
	g.Expect(rec.calls[4]).To(ContainElement("upgrade"))
}

func TestHandleUpgrade_AbortsBeforeUpgradeOnExtensionFailure(t *testing.T) {
	g := NewWithT(t)
	withTempPersistentDir(t)
	// Fail on the first install call.
	rec := &failingRecorder{failOn: 0}
	prev := execCommand
	execCommand = rec.record
	t.Cleanup(func() { execCommand = prev })
	prevR := scheduleRebootFn
	scheduleRebootFn = func() { t.Fatalf("scheduleReboot must not be called when an extension install fails") }
	t.Cleanup(func() { scheduleRebootFn = prevR })

	raw, _ := json.Marshal([]BundledExtension{{Type: "sysext", Name: "x", Source: "https://x/a"}})
	_, err := handleUpgrade(context.Background(), CommandData{
		Command: "upgrade",
		Args: map[string]string{
			"source":     "oci:quay.io/myorg/edge-os:v4.2.0",
			"extensions": string(raw),
		},
	}, "http://example", "key")
	g.Expect(err).To(MatchError(ContainSubstring("install sysext/x")))
	g.Expect(rec.calls).To(HaveLen(1)) // only the failed install; no enable, no upgrade
}
```

- [ ] **Step 9: Extract `scheduleReboot` into a test seam**

In `internal/phonehome/handlers.go`, replace the existing `scheduleReboot()` function (around lines 235-242) so it routes through a swappable function:

```go
// scheduleRebootFn is a seam for tests.
var scheduleRebootFn = scheduleRebootImpl

func scheduleReboot() {
	scheduleRebootFn()
}

func scheduleRebootImpl() {
	go func() {
		_ = exec.Command("sync").Run()
		time.Sleep(10 * time.Second)
		_ = exec.Command("reboot").Run()
	}()
}
```

(Replace the original `scheduleReboot` body — keep the `//nosec` comments inline on the exec lines.)

Also route the existing `exec.Command("kairos-agent", args...)` call inside `handleUpgrade` (line 150) through the `execCommand` seam introduced in Task 3, so the bundle tests can assert the upgrade call as well:

```go
out, err := execCommand("kairos-agent", args...).CombinedOutput() //nosec G204
```

- [ ] **Step 10: Extend `handleUpgrade` to install bundled extensions first**

In `handleUpgrade`, immediately after the existing source-resolution block (after line 142, where `args` for the upgrade call has been built but before `exec.Command`), insert:

```go
	// Install bundled extensions before the OS upgrade. Each install is
	// idempotent (kairos-agent install overwrites the .raw in place), so
	// retrying the same compound command after a partial failure is safe.
	bundled, err := parseBundledExtensions(cmd.Args["extensions"])
	if err != nil {
		return "", err
	}
	for _, e := range bundled {
		if err := installBundledExtension(ctx, e); err != nil {
			// Do NOT proceed to the OS upgrade if any extension fails.
			return "", err
		}
	}
```

- [ ] **Step 11: Run the upgrade tests, verify they pass**

```
cd ~/_git/kairos-agent && go test ./internal/phonehome/... -run TestHandleUpgrade -v
```

Expected: PASS for the three new tests plus any pre-existing upgrade tests still green.

- [ ] **Step 12: Run the full package test suite to verify no regressions**

```
cd ~/_git/kairos-agent && go test ./internal/phonehome/... -v
```

Expected: all green.

- [ ] **Step 13: Commit**

```bash
git add internal/phonehome/handlers.go internal/phonehome/handlers_extension.go internal/phonehome/handlers_extension_test.go
git commit -m "phonehome: install bundled extensions before OS upgrade"
```

---

## Task 8: `upgrade-recovery` parity

**Files:**
- Modify: `internal/phonehome/handlers_extension.go` (variant of `installBundledExtension`)
- Modify: `internal/phonehome/handlers.go` (route `upgrade-recovery` through the same parse/install loop with `--recovery` scope)
- Modify: `internal/phonehome/handlers_extension_test.go`

`handleUpgrade` is shared by `upgrade` and `upgrade-recovery` (see the existing switch case at handlers.go:46). The bundle loop runs for both, but recovery-mode extensions should enable at `--recovery` scope, not `--active`.

- [ ] **Step 1: Write the failing test**

Add to `internal/phonehome/handlers_extension_test.go`:

```go
func TestHandleUpgradeRecovery_EnablesAtRecoveryScope(t *testing.T) {
	g := NewWithT(t)
	withTempPersistentDir(t)
	rec := &commandRecorder{}
	prev := execCommand
	execCommand = rec.record
	t.Cleanup(func() { execCommand = prev })
	prevR := scheduleRebootFn
	scheduleRebootFn = func() {}
	t.Cleanup(func() { scheduleRebootFn = prevR })

	raw, _ := json.Marshal([]BundledExtension{{Type: "sysext", Name: "rescue-tools", Source: "https://x/r"}})
	_, err := handleUpgrade(context.Background(), CommandData{
		Command: "upgrade-recovery",
		Args: map[string]string{
			"source":     "oci:quay.io/myorg/edge-os:v4.2.0",
			"recovery":   "true",
			"extensions": string(raw),
		},
	}, "http://example", "key")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(rec.calls).To(HaveLen(3))
	g.Expect(rec.calls[1]).To(Equal([]string{"kairos-agent", "sysext", "enable", "rescue-tools", "--recovery"}))
}
```

- [ ] **Step 2: Run the test, verify it fails**

```
cd ~/_git/kairos-agent && go test ./internal/phonehome/... -run TestHandleUpgradeRecovery -v
```

Expected: FAIL — the current `installBundledExtension` hard-codes `--active`.

- [ ] **Step 3: Generalize `installBundledExtension` with a scope parameter**

In `internal/phonehome/handlers_extension.go`, change the helper signature:

```go
// installBundledExtension downloads (overwriting) and conditionally enables
// the extension at the given scope. scope is "active" for `upgrade`, "recovery"
// for `upgrade-recovery`.
func installBundledExtension(ctx context.Context, e BundledExtension, scope string) error {
	if out, err := runCLI(ctx, e.Type, "install", e.Source); err != nil {
		return fmt.Errorf("install %s/%s: %w: %s", e.Type, e.Name, err, out)
	}
	if extensionEnabledAnywhere(e.Type, e.Name) {
		return nil
	}
	if out, err := runCLI(ctx, e.Type, "enable", e.Name, "--"+scope); err != nil {
		return fmt.Errorf("enable %s/%s --%s: %w: %s", e.Type, e.Name, scope, err, out)
	}
	return nil
}
```

- [ ] **Step 4: Update the caller in `handleUpgrade` to pick the scope**

In `internal/phonehome/handlers.go`, in `handleUpgrade`, change the bundle loop to:

```go
	scope := "active"
	if cmd.Command == "upgrade-recovery" {
		scope = "recovery"
	}
	for _, e := range bundled {
		if err := installBundledExtension(ctx, e, scope); err != nil {
			return "", err
		}
	}
```

- [ ] **Step 5: Update the Task 7 tests' caller to pass `"active"` to `installBundledExtension`**

`TestInstallBundledExtension_*` now needs the scope arg:

```go
err := installBundledExtension(context.Background(), BundledExtension{...}, "active")
```

(Two locations — update both Task 7 install-bundle tests.)

- [ ] **Step 6: Run the full package, verify everything passes**

```
cd ~/_git/kairos-agent && go test ./internal/phonehome/... -v
```

Expected: all green including the new recovery test.

- [ ] **Step 7: Commit**

```bash
git add internal/phonehome/handlers.go internal/phonehome/handlers_extension.go internal/phonehome/handlers_extension_test.go
git commit -m "phonehome: upgrade-recovery installs bundled extensions at --recovery scope"
```

---

## Task 9: Whole-repo verification + lint

**Files:** none (verification only)

- [ ] **Step 1: Run the full Go test suite**

```
cd ~/_git/kairos-agent && go test ./... -count=1
```

Expected: all packages green. If any pre-existing tests fail unrelated to this change, investigate before continuing.

- [ ] **Step 2: Run gofmt / goimports / go vet**

```
cd ~/_git/kairos-agent && gofmt -l internal/phonehome/ && go vet ./internal/phonehome/...
```

Expected: no output from `gofmt -l` (no unformatted files), `go vet` exits 0.

- [ ] **Step 3: Build the binary to confirm it links**

```
cd ~/_git/kairos-agent && go build -o /tmp/kairos-agent ./
```

Expected: produces `/tmp/kairos-agent` with no errors.

- [ ] **Step 4: Sanity-check the new help text appears**

```
/tmp/kairos-agent sysext --help
/tmp/kairos-agent confext --help
```

Expected: both show the same `list / enable / disable / install / remove` subcommands as before — we did not change the CLI surface, only the phonehome dispatcher that drives it.

- [ ] **Step 5: Run linter (if available in repo CI config)**

If the repo has a `golangci.yml` or runs `golangci-lint` in CI:

```
cd ~/_git/kairos-agent && golangci-lint run ./internal/phonehome/...
```

Expected: no issues. If lint is not configured locally, skip.

---

## Task 10: Tag a release

**Files:** none — this is a maintainer step.

- [ ] **Step 1: Verify the working tree is clean**

```
cd ~/_git/kairos-agent && git status
```

Expected: "nothing to commit, working tree clean".

- [ ] **Step 2: Push the branch and open a PR**

The PR description must include:

```markdown
## Summary
- Add `extension` phonehome command (install/enable/disable/remove for sysext + confext)
- Extend `upgrade` / `upgrade-recovery` to install bundled extensions before the OS upgrade reboot
- New extension command is opt-in (destructive); existing upgrade gate covers the ride-along

## Test plan
- [ ] `go test ./internal/phonehome/...` passes
- [ ] `go build ./` produces a binary
- [ ] Manual smoke: build a sysext via auroraboot, send via phonehome, verify it lands under /var/lib/kairos/extensions/

🤖 Generated with [Claude Code](https://claude.com/claude-code)
```

- [ ] **Step 3: After PR merge, tag and release**

This step is run by the maintainer after CI is green and the PR is merged:

```bash
cd ~/_git/kairos-agent
git fetch origin
git checkout main && git pull
# pick the next version per the repo's semver convention (e.g., v0.X.Y)
git tag -a vX.Y.Z -m "vX.Y.Z: phonehome extension command + bundled upgrade"
git push origin vX.Y.Z
```

GitHub Actions will build artifacts. AuroraBoot's Plan 2 will then bump its `go.mod` to this tag.

---

## Spec coverage check

| Spec section | Covered by |
|---|---|
| Two delivery flows | Tasks 1-7 (manual) and 7-8 (bundled) |
| `extension` command (manual) | Tasks 1-5 |
| Extended `upgrade` with `extensions[]` | Tasks 6-7 |
| `upgrade-recovery` parity | Task 8 |
| JSON-encoded `extensions` arg (string-valued map) | Task 6 |
| Install action = install + enable (two CLI calls) | Task 3 |
| Conditional `enable --active` (skip if already enabled) | Task 7 |
| Abort before OS upgrade on extension failure | Tasks 5, 7 |
| No `--now` during compound upgrade | Task 7 |
| Idempotent re-installs | Task 7 (implicit — overwrites .raw) |
| Policy gating (`extension` not in safe defaults) | Inherited from existing `DefaultCommandHandler` policy gate; verified in Task 2 |

## Out of scope for this plan

- Tagging convention / release machinery — defer to maintainer.
- Changes to `pkg/action/sysext.go` itself — none required; the existing functions are sufficient.
- Multi-extension parallelism — extensions install sequentially per the spec.

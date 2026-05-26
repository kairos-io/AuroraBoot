# Extensions UI & Lifecycle — Design

Date: 2026-05-26
Status: Approved for implementation
Owner: @mudler

## Goal

Expose **sysext** and **confext** building in the AuroraBoot UI and give them the same fleet lifecycle that OS artifacts already have: build → store → push to nodes (install / upgrade / remove). Today the CLI commands `auroraboot sysext` and `auroraboot confext` exist, but there is no UI, no DB record, no fleet delivery, and no phonehome command on the agent side.

## Non-goals

- Editing an existing extension in place (extensions are rebuilt, like artifacts).
- A pluggable plugin registry for templates (use the same hard-coded template list pattern as `ArtifactBuilder.tsx`).
- Multi-arch builds in one record (one extension record = one architecture, mirroring how artifacts work today).
- New CLI flags beyond what's needed for the hierarchies free-list (see *CLI changes* below).
- Integration with external OCI registries beyond what artifacts already support.
- A new SecureBootKeySet-equivalent store. We **reuse** `SecureBootKeySet` for signing.

## Architecture overview

```
┌────────────────┐   POST /api/v1/extensions     ┌──────────────────────┐
│  Extensions    │ ────────────────────────────▶ │  ExtensionHandler    │
│  Builder (UI)  │                                │  (pkg/handlers)      │
└────────────────┘                                └──────────┬───────────┘
                                                             │
                                                             ▼
                                                  ┌─────────────────────┐
                                                  │ ExtensionBuilder    │
                                                  │ (pkg/builder)       │
                                                  │  • optional docker  │
                                                  │    build of derived │
                                                  │    image            │
                                                  │  • auroraboot       │
                                                  │    sysext|confext   │
                                                  │  • write .raw to    │
                                                  │    artifactsDir     │
                                                  └──────────┬──────────┘
                                                             │
                                                             ▼
                                                  ┌─────────────────────┐
                                                  │ ExtensionStore (DB) │
                                                  └─────────────────────┘

┌────────────────┐  POST /api/v1/nodes/:id/commands ┌────────────────┐
│ Install dialog │ ───────────────────────────────▶ │ CommandHandler │
│ (UI)           │   { command: "extension", args } └───────┬────────┘
└────────────────┘                                          │
                                                            ▼
                                                  ┌─────────────────────┐
                                                  │ Phonehome WS        │
                                                  └──────────┬──────────┘
                                                             │
                                                             ▼   on the node
                                                  ┌─────────────────────┐
                                                  │ kairos-agent        │
                                                  │ phonehome handler   │
                                                  │  case "extension":  │
                                                  │   → sysext install  │
                                                  │   → confext remove  │
                                                  │   …                 │
                                                  └─────────────────────┘
```

## UI surfaces

A new top-level **Extensions** entry in the side nav, placed between **Artifacts** and **Nodes**. Three routes:

- `/extensions` — list.
- `/extensions/new` — three-step builder (Source → Configure → Review).
- `/extensions/:id` — detail page (phase, logs, download, Install action).

The list page mirrors the Artifacts list visually (orange-accented page header, filter pills, dense table) but with extension-specific columns: Name · Type · Arch · Version · Signed · Phase · Updated · Actions. Empty state shows a one-line value prop, a primary **Build extension** button, and three template chips that pre-fill the builder.

The Type column carries a chip — sky-500 family for `sysext`, violet-500 family for `confext` — used consistently wherever the type appears (list, detail, install dialog).

The builder Step 1 has three Source modes, in order: **From artifact** (default tab when at least one Ready artifact exists), **Base image**, **Dockerfile**. "From artifact" picks an existing AuroraBoot artifact and optionally appends Dockerfile-style steps that get wrapped in `FROM <artifact-image>` before the sysext extractor runs. The picker shows a cross-check strip that compares the user-selected hierarchies against the artifact's stored `extensionHierarchies` — green when supported, amber when missing, with an "Add it to the artifact →" link that clones the artifact prefilled with the missing path.

The builder Step 2 carries: Extension type (sysext/confext), Architecture (amd64/arm64/riscv64), Version (free string used server-side to decide staleness), Signing keyset picker (reuses `SecureBootKeySet`), and — sysext-only — an Additional Hierarchies chip input. `/usr` is implicit and lives in help text, not as a pinned chip. The `service-reload` toggle joins the hierarchies card so all sysext-only knobs live together. A warning callout appears whenever any non-default hierarchy is present, reminding the operator that the target node must declare those paths in `SYSTEMD_SYSEXT_HIERARCHIES`.

The builder Step 3 (Review) shows a summary table plus an equivalent-CLI panel so power users see the exact command AuroraBoot will run.

The install dialog opens from the detail page or a list row's Install action. It's a single modal with a target picker (Group / Labels / Specific nodes), four action cards (Install / Enable / Disable / Remove), a Boot Scope row (Active / Passive / Recovery / Common with info tooltips), and an "Activate immediately" toggle. Above the Cancel/Send row, the dialog shows a pre-action summary diff ("12 nodes will upgrade v1.72.1 → v1.74.0, 10 first-install, 2 offline"). The JSON payload preview is folded behind a `<details>` labeled "Show payload" so the Send button remains the bottom-right anchor.

The existing **ArtifactBuilder** gains a folded disclosure inside its Access & Security card: "Pre-configure for system extensions · Optional · advanced". When expanded, it offers chip inputs for additional `SYSTEMD_SYSEXT_HIERARCHIES` and (nested under another disclosure) `SYSTEMD_CONFEXT_HIERARCHIES`, plus a "What this bakes" disclosure that displays the cloud-config snippet AuroraBoot will append to the artifact. There is no orange "NEW" card here — the discoverability comes from being in the right neighborhood, not from accent color.

The existing **Allowed remote commands** picker in `ArtifactBuilder` gains a new row, `extension`, placed in the **Destructive — opt in per fleet** group alongside `exec`, `reset`, `apply-cloud-config`. The chip uses the destructive (red) palette; there is no orange NEW badge — a small "NEW · destructive" pill suffices.

## Data model

A new GORM-managed table `extensions`:

```
id                  uuid (pk)
name                string  // user-facing identifier, unique per server
type                string  // "sysext" | "confext"
phase               string  // "Pending" | "Building" | "Ready" | "Error"
message             string  // error excerpt or progress hint
arch                string  // "amd64" | "arm64" | "riscv64"
version             string  // user-supplied, stored verbatim
source_mode         string  // "artifact" | "image" | "dockerfile"
source_artifact_id  uuid    // when source_mode = "artifact"
source_image        string  // when source_mode in {"image","artifact"}
dockerfile          text    // when source_mode in {"dockerfile","artifact" w/ steps}
extra_steps         text    // when source_mode = "artifact" with appended steps
signing_keyset_id   uuid    // FK SecureBootKeySet (nullable)
hierarchies         []string // additional hierarchies (sysext only; /usr implicit)
service_reload      bool     // sysext-only flag
container_image     string   // resolved OCI ref used as the build input
raw_filename        string   // produced .raw filename, relative to artifactsDir
created_at, updated_at
```

The existing `Artifact` table gains two nullable columns:

```
extension_hierarchies json  // {"sysext": ["/opt","/srv"], "confext": []}
```

Stored verbatim from the ArtifactBuilder; used by the Extensions builder's cross-check.

## API

```
GET  /api/v1/extensions                          → []Extension
POST /api/v1/extensions                          → Extension      (start build)
GET  /api/v1/extensions/:id                      → Extension
PATCH /api/v1/extensions/:id                     → Extension      (name only)
DELETE /api/v1/extensions/:id                    → ()
GET  /api/v1/extensions/:id/logs                 → text
POST /api/v1/extensions/:id/cancel               → ()
GET  /api/v1/extensions/:id/download/:filename   → file (Bearer)
GET  /api/v1/extensions/:id/file?token=…         → file (signed URL, used by nodes)
```

`POST /api/v1/extensions` body matches the builder form:

```jsonc
{
  "name": "tailscale-agent",
  "type": "sysext",
  "arch": "amd64",
  "version": "v1.74.0",
  "source": {
    "mode": "artifact",            // "artifact" | "image" | "dockerfile"
    "artifactId": "…",             // when mode=artifact
    "baseImage": "ubuntu:24.04",   // when mode=image
    "dockerfile": "FROM …",        // when mode=dockerfile
    "extraSteps": "RUN …"          // optional, when mode=artifact
  },
  "signingKeySetId": "…",          // optional
  "hierarchies": ["/opt", "/srv"], // sysext-only; /usr implicit; "/" and "/usr" rejected
  "serviceReload": false            // sysext-only
}
```

`hierarchies` validation (client and server):

- must start with `/`,
- must not contain `..`,
- must not be exactly `/` or `/usr` (case-sensitive — Linux filesystem semantics),
- length ≤ 256,
- normalized: trailing slashes stripped, duplicates deduped, list re-ordered alphabetically before persistence so the produced regex is deterministic.

Server returns 400 with `{field: "hierarchies[N]", message: "…"}` on the first failing entry.

The signed-URL endpoint mirrors how artifact image downloads work today: server signs a short-lived URL bound to the requesting node's bearer token, and `kairos-agent` follows it via the existing `http(s):` URI support in `kairos-agent sysext install`.

## Phonehome command

One new command name: **`extension`**. Single dispatch in `kairos-agent/internal/phonehome/handlers.go`:

```go
case "extension":
    return handleExtension(ctx, cmd, serverURL, apiKey())
```

with `cmd.Args` carrying:

```jsonc
{
  "type":      "sysext",   // "sysext" | "confext"
  "action":    "install",  // "install" | "enable" | "disable" | "remove"
  "name":      "tailscale-agent",
  "source":    "https://aurora/api/v1/extensions/3f9c…/file?token=…", // install only
  "bootState": "common",   // "active" | "passive" | "recovery" | "common"
  "now":       true
}
```

Dispatch maps directly to the existing CLI:

```
install → kairos-agent <type> install <source>
enable  → kairos-agent <type> enable  <name> --<bootState> [--now]
disable → kairos-agent <type> disable <name> --<bootState> [--now]
remove  → kairos-agent <type> remove  <name>            [--now]
```

Arg requirements per action: `source` is required for `install` and ignored otherwise; `bootState` is required for `enable` and `disable`, ignored otherwise; `name` is required for everything except `install` (the install URI carries it), but the server populates it for consistency. The agent's handler validates these before shelling out and returns a structured error if anything is missing.

`extension` is **not** in the phonehome safe-default allow list (`upgrade`, `upgrade-recovery`, `reboot`, `unregister`). It is treated as destructive (it can drop arbitrary OCI content into `/usr`, `/etc`, or declared hierarchies) and must be explicitly enabled per fleet in the ArtifactBuilder's allowed-commands picker.

A second install over the same name is the upgrade path — there is no separate `upgrade` action. The Install dialog labels the action card "Install" and the pre-action summary differentiates first-install from upgrade based on what AuroraBoot last successfully delivered to each node.

## CLI changes

Extend `internal/cmd/sysext.go`:

- Add a repeatable flag `--include-path` that appends to the extractor allowlist.
- Keep `--with-opt` as a deprecated alias for `--include-path=/opt` (no removal in this PR; emit a warning when used). This preserves any existing scripts.
- The handler assembles the regex from `/usr` plus each `--include-path` entry, in lexical order, and surfaces a debug log line so reproducible builds are auditable.

No changes to the `confext` command (its allowlist is `/etc`-only by definition).

## Build pipeline

`pkg/builder` gains an `ExtensionBuilder`:

1. Resolve the source image:
   - `mode=image`: use as-is.
   - `mode=artifact`: read `Artifact.containerImage` of `source.artifactId`.
   - `mode=dockerfile`: docker build the user's Dockerfile, tag with a build UUID.
   - `mode=artifact` + `extraSteps`: synthesize a Dockerfile `FROM <artifact-image>\n<extraSteps>` and docker build.
2. Invoke `auroraboot sysext|confext` against the resolved image, passing `--arch`, optional signing flags from the keyset's `db.key`/`db.pem`, the `--include-path` entries derived from `hierarchies`, and `--service-reload` when set.
3. Write the resulting `.raw` to `artifactsDir/extensions/<id>/<name>.<type>.raw`.
4. Update the DB record with `raw_filename` and `container_image`, set `phase = Ready` or `Error` with `message`.

The same WebSocket log stream used by Artifact builds is reused — logs are tagged with `kind=extension` and `id=…` so the UI can subscribe by record.

## Signing keysets

The existing `SecureBootKeySet` record carries `keysDir`, which today holds the UKI secure-boot key material. AuroraBoot reads `db.key` and `db.pem` from that directory and passes them as `--private-key` and `--certificate` to the sysext CLI. If those files are missing, the build records `phase = Error` with a message that names the missing files — operators can then regenerate the keyset or upload the missing material. No new key store is introduced.

## ArtifactBuilder integration

`ArtifactBuilder.tsx` gains, inside the existing Access & Security card, a `<details>` block "Pre-configure for system extensions · Optional · advanced". When open:

- A chip input for additional `SYSTEMD_SYSEXT_HIERARCHIES` (`/usr` always included, surfaced as help text).
- A nested `<details>` for additional `SYSTEMD_CONFEXT_HIERARCHIES`.
- A nested `<details>` "What this bakes" showing the cloud-config snippet, default-open so the operator can see what AuroraBoot will add to their image.

The hierarchies are submitted in the artifact create payload as:

```jsonc
"extensionHierarchies": {
  "sysext": ["/opt", "/srv"],
  "confext": []
}
```

The backend stores the value and, at build time, appends a stage to the baked cloud-config:

```yaml
stages:
  initramfs:
    - files:
        - path: /etc/systemd/system/systemd-sysext.service.d/99-aurora-hierarchies.conf
          permissions: 0644
          content: |
            [Service]
            Environment=SYSTEMD_SYSEXT_HIERARCHIES=/usr:/opt:/srv
        - path: /etc/systemd/system/systemd-confext.service.d/99-aurora-hierarchies.conf
          permissions: 0644
          content: |
            [Service]
            Environment=SYSTEMD_CONFEXT_HIERARCHIES=/etc
```

(The confext drop-in is only emitted when the confext list is non-empty.)

## Acceptance criteria (polish bar)

The implementation must satisfy these gates, derived from the design critique and polish passes. Each is grouped by surface.

**Spacing & alignment.** No raw pixel gaps in new files; all spacing uses Tailwind tokens. Card padding mirrors `ArtifactBuilder.tsx`. Step indicator is the same component used by ArtifactBuilder. Form rows follow the existing 6px label-to-input / 4px input-to-help rhythm.

**Typography.** Allowed sizes: `text-[11px]`, `text-xs`, `text-sm`, `text-base`, `text-xl`. Any deviation requires an inline comment. All card titles use the `CardTitle` component, all labels use `Label`, all buttons use `Button` — no new primitives. Monospace is restricted to paths, OCI refs, identifiers, JSON.

**Color discipline.** Orange `#EE5007` is reserved for primary brand action. Sky-500 family is `sysext`. Violet-500 family is `confext`. Green-500 family is Ready / supported / safe-default. Amber-500 family is Building / warning. Red-500 family is Error / destructive — including the new `extension` allowed-command chip. All borders go through `border-border`, all surfaces through `bg-muted/N` or equivalent tokens; no raw rgba values. Contrast verified against WCAG AA for both light and dark themes.

**Interaction states.** Every interactive element has default / hover / focus-visible / active / disabled states. Type pills on Step 2 are real `<button>` elements with `aria-pressed`. Hierarchy chips' `×` is a real `<button aria-label="Remove /opt">`. Quick-add chips have visible hover and press feedback. Template cards keyboard-select with Enter/Space. The Next button is disabled until per-step validation passes. The Build button shows the existing `Loader2` spinner during async submission. Disconnection during build surfaces a thin amber "Reconnecting…" banner above the logs.

**Micro-interactions.** Transitions limited to `transform` and `opacity`, 150–200ms, `ease-out`. `prefers-reduced-motion` honored — Building row progress bar transitions off. Chip insertion is 120ms fade + 4px translate-y; removal is instant.

**Copy.** Sentence case for labels ("Base image", not "Base Image"). No periods on single-line labels; help text gets periods. Imperative button verbs. "Reload systemd-sysext now" becomes "Activate immediately". Boot-scope buttons get `InfoTooltip` siblings explaining active/passive/recovery/common. Terminology locked: extension (noun), sysext/confext when type matters, install/upgrade/remove for actions; never "deploy".

**Edge cases.** Empty `/extensions` shows a one-sentence value prop, primary Build button, and three template chips — Tailscale, Fluent-bit, Nvidia container toolkit — matching the templates exposed in the builder Step 1. Error rows in the list show an inline excerpt of the build error and a Retry row action. Long names truncate with `title=` for full text on hover. Multi-arch mismatches surface a pre-build chip "Source image has no <arch> manifest" and disable Build until confirmed. Phase Building has a determinate progress bar; Pending has an indeterminate spinner.

**Forms.** Validation focuses and scrolls to the first invalid field (reuse ArtifactBuilder's pattern). Hierarchy chip validation enforces start-with-`/`, no `..`, not exactly `/` or `/usr`, length ≤ 256. Server returns structured 400 errors.

**Accessibility.** All chips carry accessible names. Disclosures (`<details>` / Radix Collapsible) are keyboard-operable. Install dialog's payload preview has an `aria-label`. Focus returns to the row's Install trigger on dialog close. The cross-check strip differentiates green/amber/red also by glyph (`✓` / `⚠` / `✕`).

**Hygiene.** All icons from `lucide-react`, sized `h-4 w-4` inline / `h-5 w-5` in card headers. No TODOs in shipped code. Every interactive element has at least a render test under `ui/src/test/`. Validation rules have table-driven backend tests under `pkg/handlers/`.

## Out of scope (follow-up)

- Multi-arch builds in one record.
- Inline edit-and-rebuild of an existing extension (clone is the path).
- Plugin templates loaded from disk.
- Removing `--with-opt` (kept as a deprecated alias).
- A dedicated "Extensions" tab on the Group detail page (the list page filter is enough for v1).

## Test plan

Frontend:

- Render tests for ExtensionList, ExtensionBuilder (each step), InstallExtensionDialog, ArtifactBuilder's new disclosure.
- Per-step validation tests for ExtensionBuilder.
- A snapshot test of the install-dialog payload shape.

Backend:

- Handler tests for the new `POST /api/v1/extensions` validating all `hierarchies` rules.
- Builder tests covering each source mode (artifact, image, dockerfile, artifact+steps).
- Phonehome handler tests for the `extension` command across all four actions and across both types.
- Migration test ensuring the new `extensions` table and `artifacts.extension_hierarchies` column are created on a fresh DB.

Agent:

- Already covered by existing `sysext.go` tests. Add a phonehome dispatch test mirroring the existing `upgrade` test, covering the new `extension` command and policy gating.

End-to-end:

- One e2e test under `e2e/` that builds an extension from a base image, installs it on a node booted from a paired artifact, and verifies the resulting symlink under `/var/lib/sysext/active`.

## Phasing

Single PR, but landed in this internal order to keep diffs reviewable:

1. Backend: `extensions` table, store, handler, builder, signed-URL endpoint.
2. CLI: `--include-path` flag with `--with-opt` deprecation.
3. Agent: phonehome `extension` command and dispatch.
4. Frontend: API client, list page, builder wizard, detail page, install dialog, ArtifactBuilder disclosure.
5. Tests + docs.

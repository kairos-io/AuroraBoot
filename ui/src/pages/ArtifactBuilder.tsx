import React, { useCallback, useEffect, useRef, useState, type FormEvent } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import {
  createArtifact,
  getArtifact,
  listSecureBootKeySets,
  uploadOverlayFiles,
  type CreateArtifactInput,
  type SecureBootKeySet,
} from "@/api/artifacts";
import { listGroups, type Group } from "@/api/groups";
import {
  PHONEHOME_SAFE_DEFAULTS,
  PHONEHOME_DESTRUCTIVE_COMMANDS,
} from "@/lib/buildConfig";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { PageHeader } from "@/components/PageHeader";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  ChevronDown,
  ChevronRight,
  AlertTriangle,
  Upload,
  X,
  Loader2,
  Check,
  Disc3,
  Wifi,
  ShieldCheck,
  HardDrive,
  Cloud,
  CloudCog,
  Package,
  Download,
  FileUp,
  Cpu,
  Server,
  CircuitBoard,
  Layers,
} from "lucide-react";
import { InfoTooltip } from "@/components/InfoTooltip";
import { toast } from "@/hooks/useToast";
import {
  BUILD_CONFIG_KIND,
  BUILD_CONFIG_VERSION,
  downloadBuildConfig,
  payloadFromBuilder,
} from "@/lib/buildConfig";

type OutputField = "iso" | "netboot" | "uki" | "rawDisk" | "cloudImage" | "gce" | "vhd" | "tar";
type OutputTone = "install" | "disk" | "archive";
type OutputCardDef = {
  field: OutputField;
  label: string;
  desc: string;
  icon: React.ComponentType<{ className?: string }>;
};

// Tone classes for the ambient "Outputs" chip row under the step indicator.
// Kept in sync with ArtifactDetail.tsx's outputCategories tones so a build
// looks the same wherever you see it.
const OUTPUT_TONE_CLASSES: Record<OutputTone, string> = {
  install: "border-[#EE5007]/30 bg-[#EE5007]/10 text-[#C73F00]",
  disk: "border-sky-500/30 bg-sky-500/10 text-sky-700",
  archive: "border-border bg-muted/60 text-foreground",
};

const OUTPUT_GROUPS: { title: string; tone: OutputTone; items: OutputCardDef[] }[] = [
  {
    title: "Install Media",
    tone: "install",
    items: [
      { field: "iso", label: "ISO", desc: "Bootable installer image", icon: Disc3 },
      { field: "netboot", label: "Netboot", desc: "PXE network boot artifacts", icon: Wifi },
      { field: "uki", label: "UKI", desc: "Signed Unified Kernel Image", icon: ShieldCheck },
    ],
  },
  {
    title: "Disk Images",
    tone: "disk",
    items: [
      { field: "rawDisk", label: "Raw Disk", desc: "Flat .raw disk image", icon: HardDrive },
      { field: "cloudImage", label: "Cloud Image", desc: "Generic cloud disk", icon: Cloud },
      { field: "gce", label: "Google Cloud", desc: "GCE-compatible image", icon: CloudCog },
      { field: "vhd", label: "Azure (VHD)", desc: "Azure VHD image", icon: CloudCog },
    ],
  },
  {
    title: "Archives",
    tone: "archive",
    items: [
      { field: "tar", label: "TAR", desc: "OCI container tarball", icon: Package },
    ],
  },
];

type ArchDef = {
  value: "amd64" | "arm64";
  label: string;
  desc: string;
  icon: React.ComponentType<{ className?: string }>;
};

const ARCHES: ArchDef[] = [
  {
    value: "amd64",
    label: "AMD64",
    desc: "Intel / AMD 64-bit. VMs, servers, most laptops.",
    icon: Cpu,
  },
  {
    value: "arm64",
    label: "ARM64",
    desc: "64-bit ARM. Raspberry Pi, Nvidia Jetson, Apple Silicon.",
    icon: Cpu,
  },
];

type ModelDef = {
  value: string;
  label: string;
  desc: string;
  archs: Array<"amd64" | "arm64">;
  icon: React.ComponentType<{ className?: string }>;
};

const MODELS: ModelDef[] = [
  {
    value: "generic",
    label: "Generic",
    desc: "VMs, generic boards, cloud instances.",
    archs: ["amd64", "arm64"],
    icon: Server,
  },
  {
    value: "rpi3",
    label: "Raspberry Pi 3",
    desc: "Raspberry Pi 3 boards.",
    archs: ["arm64"],
    icon: CircuitBoard,
  },
  {
    value: "rpi4",
    label: "Raspberry Pi 4",
    desc: "Raspberry Pi 4 boards.",
    archs: ["arm64"],
    icon: CircuitBoard,
  },
  {
    value: "nvidia-agx-orin",
    label: "Nvidia AGX Orin",
    desc: "Nvidia Jetson AGX Orin developer kits.",
    archs: ["arm64"],
    icon: Cpu,
  },
  {
    value: "nvidia-orin-nx",
    label: "Nvidia Orin NX",
    desc: "Nvidia Jetson Orin NX modules.",
    archs: ["arm64"],
    icon: Cpu,
  },
];

function modelsForArch(arch: string): ModelDef[] {
  return MODELS.filter((m) => m.archs.includes(arch as "amd64" | "arm64"));
}

type VariantDef = {
  value: "core" | "standard";
  label: string;
  desc: string;
  icon: React.ComponentType<{ className?: string }>;
};

const VARIANTS: VariantDef[] = [
  {
    value: "core",
    label: "Core",
    desc: "Minimal OS, cloud-init only. Bring your own workloads.",
    icon: Package,
  },
  {
    value: "standard",
    label: "Standard",
    desc: "OS bundled with a Kubernetes distribution (K3s or K0s).",
    icon: Layers,
  },
];

interface BuildTemplate {
  name: string;
  description: string;
  values: Partial<CreateArtifactInput>;
}

const TEMPLATES: BuildTemplate[] = [
  {
    name: "Hadron + K3s",
    description: "Hadron Linux with K3s, generic model, ISO",
    values: {
      baseImage: "quay.io/kairos/hadron:v0.0.4-core-amd64-generic-v4.0.3",
      kairosVersion: "v4.0.3",
      model: "generic",
      arch: "amd64",
      variant: "core",
      kubernetesDistro: "k3s",
      outputs: { iso: true, cloudImage: false, netboot: false, rawDisk: false, tar: false, gce: false, vhd: false, uki: false, fips: false, trustedBoot: false },
    },
  },
  {
    name: "Ubuntu 24.04",
    description: "Ubuntu base, auto-kairosified, ISO",
    values: {
      baseImage: "ubuntu:24.04",
      kairosVersion: "v4.0.3",
      model: "generic",
      arch: "amd64",
      variant: "core",
      outputs: { iso: true, cloudImage: false, netboot: false, rawDisk: false, tar: false, gce: false, vhd: false, uki: false, fips: false, trustedBoot: false },
    },
  },
  {
    name: "Fedora 40",
    description: "Fedora base, auto-kairosified, ISO",
    values: {
      baseImage: "fedora:40",
      kairosVersion: "v4.0.3",
      model: "generic",
      arch: "amd64",
      variant: "core",
      outputs: { iso: true, cloudImage: false, netboot: false, rawDisk: false, tar: false, gce: false, vhd: false, uki: false, fips: false, trustedBoot: false },
    },
  },
  {
    name: "openSUSE Leap 15.6",
    description: "openSUSE Leap base, auto-kairosified, ISO",
    values: {
      baseImage: "opensuse/leap:15.6",
      kairosVersion: "v4.0.3",
      model: "generic",
      arch: "amd64",
      variant: "core",
      outputs: { iso: true, cloudImage: false, netboot: false, rawDisk: false, tar: false, gce: false, vhd: false, uki: false, fips: false, trustedBoot: false },
    },
  },
  {
    name: "Debian 12",
    description: "Debian Bookworm base, auto-kairosified, ISO",
    values: {
      baseImage: "debian:12",
      kairosVersion: "v4.0.3",
      model: "generic",
      arch: "amd64",
      variant: "core",
      outputs: { iso: true, cloudImage: false, netboot: false, rawDisk: false, tar: false, gce: false, vhd: false, uki: false, fips: false, trustedBoot: false },
    },
  },
  {
    name: "Alpine 3.21",
    description: "Alpine Linux base, auto-kairosified, ISO",
    values: {
      baseImage: "alpine:3.21",
      kairosVersion: "v4.0.3",
      model: "generic",
      arch: "amd64",
      variant: "core",
      outputs: { iso: true, cloudImage: false, netboot: false, rawDisk: false, tar: false, gce: false, vhd: false, uki: false, fips: false, trustedBoot: false },
    },
  },
  {
    name: "Rocky Linux 9",
    description: "Rocky Linux 9 base, auto-kairosified, ISO",
    values: {
      baseImage: "rockylinux:9",
      kairosVersion: "v4.0.3",
      model: "generic",
      arch: "amd64",
      variant: "core",
      outputs: { iso: true, cloudImage: false, netboot: false, rawDisk: false, tar: false, gce: false, vhd: false, uki: false, fips: false, trustedBoot: false },
    },
  },
  {
    name: "Custom",
    description: "Start from scratch",
    values: {},
  },
];

const EMPTY_OUTPUTS = {
  iso: false,
  cloudImage: false,
  netboot: false,
  rawDisk: false,
  tar: false,
  gce: false,
  vhd: false,
  uki: false,
  fips: false,
  trustedBoot: false,
};

const EMPTY_SIGNING = {
  ukiKeySetId: "",
  ukiSecureBootKey: "",
  ukiSecureBootCert: "",
  ukiTpmPcrKey: "",
  ukiPublicKeysDir: "",
  ukiSecureBootEnroll: "if-safe",
};

const EMPTY_PROVISIONING = {
  autoInstall: true,
  registerAuroraBoot: true,
  targetGroupId: "",
  // Start with the safe-default command set pre-selected; the UI always
  // submits this list verbatim so the baked cloud-config is authoritative.
  allowedCommands: [...PHONEHOME_SAFE_DEFAULTS] as string[],
};

// Default artifact version when the user doesn't provide one. This value
// ends up in the image as /etc/kairos-release's KAIROS_RELEASE and is
// what auroraboot compares against when deciding whether a node needs an
// upgrade, so we give brand-new builds a sensible starting version
// instead of leaving the field blank.
const DEFAULT_ARTIFACT_VERSION = "v1.0";

const EMPTY_FORM: CreateArtifactInput = {
  name: "",
  baseImage: "",
  kairosVersion: DEFAULT_ARTIFACT_VERSION,
  model: "generic",
  arch: "amd64",
  variant: "core",
  kubernetesDistro: "",
  kubernetesVersion: "",
  dockerfile: "",
  overlayRootfs: "",
  extendCmdline: "",
  kairosInitImage: "",
  outputs: { ...EMPTY_OUTPUTS },
  signing: { ...EMPTY_SIGNING },
  provisioning: { ...EMPTY_PROVISIONING },
  cloudConfig: "",
};

type UserMode = "default" | "custom" | "none";

// FieldError pairs a validation error with the form field and wizard step
// it belongs to, so we can jump the user straight to the offending input
// instead of just showing a red list at the top of the page.
type FieldError = { field: string; step: number; message: string };

const STEPS = ["Source", "Configure", "Output", "Review"];

// Per-command descriptions surfaced below each checkbox. Kept short so the
// row matches the "Auto-install" / "Register" rhythm in the parent card —
// a single line of secondary copy, not a paragraph.
const COMMAND_DESCRIPTIONS: Record<string, string> = {
  upgrade: "Pull a new OS image and reboot into it.",
  "upgrade-recovery": "Update the recovery partition without rebooting.",
  reboot: "Reboot the node on demand.",
  unregister: "Stop the phone-home service and clean up its files (used by Delete).",
  exec: "Run arbitrary shell commands on the node.",
  reset: "Factory-reset the node, wiping persistent data.",
  "apply-cloud-config": "Write a new cloud-config to the OEM partition.",
};

// AllowedCommandsPicker renders phonehome.allowed_commands as two stacked
// groups of checkboxes. Safe commands are listed first at the card's baseline
// rhythm; the destructive group sits below a divider so operators have to
// cross a visible boundary before enabling anything that can wipe or own a
// node. The list is submitted verbatim — an empty selection is valid
// (observe-only) but we flag it because it's almost always a mistake.
function AllowedCommandsPicker({
  value,
  onChange,
}: {
  value: string[];
  onChange: (next: string[]) => void;
}) {
  const set = new Set(value);
  const toggle = (cmd: string) => {
    const next = new Set(set);
    if (next.has(cmd)) {
      next.delete(cmd);
    } else {
      next.add(cmd);
    }
    // Preserve the canonical order (safe → destructive) so the emitted YAML
    // is stable regardless of click order.
    const ordered = [
      ...PHONEHOME_SAFE_DEFAULTS,
      ...PHONEHOME_DESTRUCTIVE_COMMANDS,
    ].filter((c) => next.has(c));
    onChange(ordered);
  };

  const commandRow = (cmd: string) => (
    <div key={cmd}>
      <label className="flex items-center gap-2 text-sm font-medium">
        <input
          type="checkbox"
          checked={set.has(cmd)}
          onChange={() => toggle(cmd)}
          className="rounded border-input"
        />
        <code className="font-mono">{cmd}</code>
      </label>
      <p className="text-xs text-muted-foreground mt-1 ml-6">
        {COMMAND_DESCRIPTIONS[cmd]}
      </p>
    </div>
  );

  return (
    <div>
      <Label className="text-xs">
        Allowed remote commands
        <InfoTooltip>
          Baked into <code className="font-mono">phonehome.allowed_commands</code> in the
          node's cloud-config. Commands not ticked here are refused by the
          node, even if AuroraBoot requests them.
        </InfoTooltip>
      </Label>
      <p className="text-xs text-muted-foreground mt-1">
        Commands not listed here will be denied by the node.
      </p>

      <div className="mt-3 space-y-3">
        {PHONEHOME_SAFE_DEFAULTS.map(commandRow)}
      </div>

      <div className="mt-4 pt-3 border-t border-border/60 space-y-3">
        <p className="text-xs text-muted-foreground">
          Destructive — opt in per fleet.
        </p>
        {PHONEHOME_DESTRUCTIVE_COMMANDS.map(commandRow)}
      </div>

      {value.length === 0 && (
        <div className="mt-3 rounded-md border border-amber-400/60 bg-amber-50 px-3 py-2 text-xs text-amber-900 dark:border-amber-400/40 dark:bg-amber-950/60 dark:text-amber-100">
          No commands selected — the node will still register and send
          heartbeats, but no remote management will be possible. If you
          don&apos;t need any management, untick{" "}
          <strong>Register with AuroraBoot</strong> instead.
        </div>
      )}
    </div>
  );
}

export function ArtifactBuilder() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const [groups, setGroups] = useState<Group[]>([]);
  const [keySets, setKeySets] = useState<SecureBootKeySet[]>([]);
  const [selectedTemplate, setSelectedTemplate] = useState("");
  const [buildMode, setBuildMode] = useState<"image" | "dockerfile">("image");
  const [form, setForm] = useState<CreateArtifactInput>({ ...EMPTY_FORM, outputs: { ...EMPTY_OUTPUTS }, signing: { ...EMPTY_SIGNING }, provisioning: { ...EMPTY_PROVISIONING } });
  const [cloneSource, setCloneSource] = useState("");
  const [customModel, setCustomModel] = useState(false);
  const [ukiKeyMode, setUkiKeyMode] = useState<"keyset" | "manual">("keyset");
  const [errors, setErrors] = useState<string[]>([]);
  const [overlayFiles, setOverlayFiles] = useState<string[]>([]);
  const [overlayUploading, setOverlayUploading] = useState(false);
  const [dragOver, setDragOver] = useState(false);
  const overlayInputRef = useRef<HTMLInputElement>(null);
  const [step, setStep] = useState(0);

  // Refs to the fields validation can complain about. Populated via
  // bindRef(key). Non-focusable entries (e.g. the outputs container) are
  // scrolled into view but not focused.
  const fieldRefs = useRef<Record<string, HTMLElement | null>>({});
  const bindRef = useCallback(
    (key: string) => (el: HTMLElement | null) => {
      fieldRefs.current[key] = el;
    },
    [],
  );
  // When non-null, the effect below scrolls + focuses this field after the
  // next render. Cleared back to null in the effect so it fires exactly
  // once per focus request.
  const [focusTarget, setFocusTarget] = useState<string | null>(null);

  // User/SSH config state
  const [userMode, setUserMode] = useState<UserMode>("default");
  const [username, setUsername] = useState("kairos");
  const [password, setPassword] = useState("kairos");
  const [sshKeys, setSshKeys] = useState("");

  // Advanced cloud-config
  const [advancedConfig, setAdvancedConfig] = useState("");
  const [showAdvanced, setShowAdvanced] = useState(false);

  const importInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    listGroups().then(setGroups).catch(() => {});
    listSecureBootKeySets().then(setKeySets).catch(() => {});
  }, []);

  // After focusFirstError queues a focusTarget and setStep has re-rendered
  // the new step, scroll the field into view and focus it. We clear
  // focusTarget after firing so the effect doesn't re-run on unrelated
  // renders (e.g. when the user types in the field we just focused).
  useEffect(() => {
    if (!focusTarget) return;
    const el = fieldRefs.current[focusTarget];
    if (!el) return;
    el.scrollIntoView({ block: "center", behavior: "smooth" });
    if (typeof (el as HTMLElement).focus === "function") {
      (el as HTMLElement).focus({ preventScroll: true });
    }
    setFocusTarget(null);
  }, [step, focusTarget]);

  // --- Export / Import build config -------------------------------------------------
  // The exported JSON is self-contained and portable: groups and secure-boot
  // key sets are referenced by *name*, not by local database ID, so a config
  // shared between two auroraboot instances can find its referents by name.
  // Raw secrets (manual UKI key/cert paths, advanced cloud-config passwords)
  // are intentionally included as-is because users need them on the target
  // instance too; if you're sharing a config more widely, strip them first.
  function handleExportConfig() {
    const payload = payloadFromBuilder({
      form,
      buildMode,
      groups,
      keySets,
      userMode,
      username,
      sshKeys,
      advancedConfig,
    });
    const filename = downloadBuildConfig(payload);
    toast(`Exported ${filename}`, "success");
  }

  function handleImportClick() {
    importInputRef.current?.click();
  }

  async function handleImportFile(ev: React.ChangeEvent<HTMLInputElement>) {
    const file = ev.target.files?.[0];
    // Reset the input so the same file can be re-selected later.
    if (ev.target) ev.target.value = "";
    if (!file) return;

    let parsed: any;
    try {
      parsed = JSON.parse(await file.text());
    } catch {
      toast("Import failed: not valid JSON", "error");
      return;
    }
    if (parsed?.kind !== BUILD_CONFIG_KIND) {
      toast("Import failed: not a auroraboot build config", "error");
      return;
    }
    if (parsed.version !== BUILD_CONFIG_VERSION) {
      toast(`Import failed: unsupported version ${parsed.version}`, "error");
      return;
    }

    const src = parsed.source || {};
    const prov = parsed.provisioning || {};
    const out = parsed.outputs || {};
    const sign = parsed.signing || {};

    const resolvedGroupId = prov.targetGroupName
      ? groups.find((g) => g.name === prov.targetGroupName)?.id || ""
      : "";
    const resolvedKeySetId = sign.ukiKeySetName
      ? keySets.find((k) => k.name === sign.ukiKeySetName)?.id || ""
      : "";

    setForm({
      ...EMPTY_FORM,
      name: parsed.name || "",
      baseImage: src.baseImage || "",
      kairosVersion: src.kairosVersion || "",
      arch: src.arch || "amd64",
      model: src.model || "generic",
      variant: src.variant || "core",
      kubernetesDistro: src.kubernetesDistro || "",
      kubernetesVersion: src.kubernetesVersion || "",
      dockerfile: parsed.dockerfile || "",
      overlayRootfs: parsed.overlayRootfs || "",
      extendCmdline: src.extendCmdline || "",
      kairosInitImage: src.kairosInitImage || "",
      outputs: { ...EMPTY_OUTPUTS, ...out },
      signing: {
        ...EMPTY_SIGNING,
        ukiKeySetId: resolvedKeySetId,
        ukiSecureBootEnroll: sign.ukiSecureBootEnroll || "if-safe",
      },
      provisioning: {
        ...EMPTY_PROVISIONING,
        autoInstall: prov.autoInstall ?? true,
        registerAuroraBoot: prov.registerAuroraBoot ?? true,
        targetGroupId: resolvedGroupId,
        // Legacy exports may omit allowedCommands; fall back to safe defaults.
        allowedCommands: prov.allowedCommands
          ? [...prov.allowedCommands]
          : [...PHONEHOME_SAFE_DEFAULTS],
      },
    });
    setBuildMode(parsed.buildMode === "dockerfile" ? "dockerfile" : "image");
    // Reveal the Image Source / Dockerfile card on the Source step. Without
    // this the card stays hidden (it only shows when a template is picked or
    // a clone is loaded), so the imported baseImage/dockerfile looks like it
    // didn't load even though the form state is correct.
    setSelectedTemplate("Custom");
    setCustomModel(false);
    setUkiKeyMode(resolvedKeySetId ? "keyset" : "keyset");
    setUserMode((prov.userMode as UserMode) || "default");
    setUsername(prov.username || "kairos");
    setPassword("kairos");
    setSshKeys(prov.sshKeys || "");
    setAdvancedConfig(parsed.advancedCloudConfig || "");
    setShowAdvanced(Boolean(parsed.advancedCloudConfig));
    setStep(0);

    const warnings: string[] = [];
    if (prov.targetGroupName && !resolvedGroupId) {
      warnings.push(`group "${prov.targetGroupName}" not found`);
    }
    if (sign.ukiKeySetName && !resolvedKeySetId) {
      warnings.push(`key set "${sign.ukiKeySetName}" not found`);
    }
    if (warnings.length > 0) {
      toast(`Imported with warnings: ${warnings.join("; ")}`, "info");
    } else {
      toast("Config imported", "success");
    }
  }

  // Clone pre-fill
  useEffect(() => {
    const cloneId = searchParams.get("clone");
    if (cloneId) {
      getArtifact(cloneId).then((a) => {
        setCloneSource(a.name || a.id.slice(0, 8));
        setForm({
          ...EMPTY_FORM,
          name: `Copy of ${a.name || a.id.slice(0, 8)}`,
          baseImage: a.baseImage,
          kairosVersion: a.kairosVersion,
          model: a.model,
          arch: a.arch || "amd64",
          variant: a.variant || "core",
          kubernetesDistro: a.kubernetesDistro || "",
          kubernetesVersion: a.kubernetesVersion || "",
          dockerfile: a.dockerfile || "",
          extendCmdline: a.extendCmdline || "",
          kairosInitImage: a.kairosInitImage || "",
          outputs: {
            iso: a.iso,
            cloudImage: a.cloudImage,
            netboot: a.netboot,
            rawDisk: a.rawDisk ?? false,
            tar: a.tar ?? false,
            gce: a.gce ?? false,
            vhd: a.vhd ?? false,
            uki: a.uki ?? false,
            fips: a.fips,
            trustedBoot: a.trustedBoot,
          },
          signing: { ...EMPTY_SIGNING },
          provisioning: {
            autoInstall: a.autoInstall ?? true,
            registerAuroraBoot: a.registerAuroraBoot ?? true,
            targetGroupId: a.targetGroupId || "",
            // Cloned artifacts start with the safe default list — the artifact
            // record doesn't persist allowedCommands, only the baked YAML does.
            allowedCommands: [...PHONEHOME_SAFE_DEFAULTS],
          },
        });
        if (a.dockerfile) setBuildMode("dockerfile");
        if (a.cloudConfig) {
          setAdvancedConfig(a.cloudConfig);
          setShowAdvanced(true);
          setUserMode("none");
        }
        setStep(3);
      }).catch(() => {});
    }
  }, [searchParams]);

  function update(field: keyof CreateArtifactInput, value: unknown) {
    setForm((prev) => ({ ...prev, [field]: value }));
  }

  function updateOutput(field: keyof typeof EMPTY_OUTPUTS, value: boolean) {
    setForm((prev) => ({ ...prev, outputs: { ...prev.outputs, [field]: value } }));
  }

  function updateSigning(field: keyof typeof EMPTY_SIGNING, value: string) {
    setForm((prev) => ({ ...prev, signing: { ...prev.signing, [field]: value } }));
  }

  function updateProvisioning(field: keyof typeof EMPTY_PROVISIONING, value: unknown) {
    setForm((prev) => ({ ...prev, provisioning: { ...prev.provisioning, [field]: value } }));
  }

  // When arch changes, reset model to first available compatible with the new arch.
  function handleArchChange(arch: string) {
    const models = modelsForArch(arch);
    update("arch", arch);
    if (!customModel) {
      // Keep current model if still compatible, otherwise fall back to 'generic' / first.
      const stillCompatible = models.some((m) => m.value === form.model);
      if (!stillCompatible && models.length > 0) {
        update("model", models[0].value);
      }
    }
  }

  // Frontend cloud-config preview — must match the backend's buildCloudConfig
  // in internal/handlers/artifacts.go. The backend is the source of truth at
  // build time; this function only powers the Review-step preview.
  function buildCloudConfig(): string {
    const lines: string[] = ["#cloud-config"];

    if (form.provisioning.autoInstall) {
      lines.push("install:");
      lines.push("  auto: true");
      lines.push("  device: auto");
      lines.push("  reboot: true");
    }

    if (form.provisioning.registerAuroraBoot) {
      const groupName = groups.find((g) => g.id === form.provisioning.targetGroupId)?.name || "";
      lines.push("phonehome:");
      lines.push(`  url: "<server-url>"`);
      lines.push(`  registration_token: "<token>"`);
      lines.push(`  group: "${groupName}"`);
      const allowed = form.provisioning.allowedCommands ?? [];
      if (allowed.length === 0) {
        lines.push("  allowed_commands: []");
      } else {
        lines.push("  allowed_commands:");
        for (const cmd of allowed) {
          lines.push(`    - ${cmd}`);
        }
      }
    }

    if (userMode !== "none") {
      const u = userMode === "default" || !username ? "kairos" : username;
      const p = userMode === "default" || !password ? "kairos" : password;
      const sshList = sshKeys.split("\n").map((s) => s.trim()).filter(Boolean);
      const stage = sshList.length > 0 ? "network" : "initramfs";

      lines.push("stages:");
      lines.push(`  ${stage}:`);
      lines.push("    - users:");
      lines.push(`        ${u}:`);
      lines.push(`          passwd: ${p}`);
      lines.push("          groups:");
      lines.push("            - admin");

      if (sshList.length > 0) {
        lines.push("          ssh_authorized_keys:");
        sshList.forEach((key) => {
          lines.push(`            - "${key}"`);
        });
      }
    }

    let cc = lines.join("\n") + "\n";

    if (advancedConfig.trim()) {
      let extra = advancedConfig.trim();
      if (extra.startsWith("#cloud-config")) {
        extra = extra.replace(/^#cloud-config\s*\n?/, "");
      }
      cc += "\n" + extra + "\n";
    }

    return cc;
  }

  // computeErrors produces a structured list: each error knows which wizard
  // step it belongs to and which field ref to focus. The top-of-page red
  // banner keeps rendering from the existing `errors: string[]` state,
  // derived from `fieldErrors.map(e => e.message)`.
  //
  // Pass an explicit step to scope to a single step (Next button flow);
  // leave undefined to validate the whole form (final Submit flow).
  function computeErrors(scope?: number): FieldError[] {
    const errs: FieldError[] = [];

    // Step 0 — Source
    if (scope === undefined || scope === 0) {
      if (buildMode === "image" && !form.baseImage.trim()) {
        errs.push({ field: "baseImage", step: 0, message: "Base image is required." });
      }
      if (buildMode === "dockerfile" && !form.dockerfile?.trim()) {
        errs.push({ field: "dockerfile", step: 0, message: "Dockerfile is required." });
      }
    }

    // Step 1 — Configure
    // Artifact version is no longer required: if the user leaves it blank
    // we stub DEFAULT_ARTIFACT_VERSION at submit time so a brand-new
    // build still has a usable version for upgrade tracking.
    if (scope === undefined || scope === 1) {
      if (form.variant === "standard" && !form.kubernetesDistro?.trim()) {
        errs.push({
          field: "kubernetesDistro",
          step: 1,
          message: "Kubernetes distro is required for standard variant.",
        });
      }
    }

    // Step 2 — Output
    if (scope === undefined || scope === 2) {
      const hasOutput = Object.values(form.outputs).some(Boolean);
      if (!hasOutput) {
        errs.push({
          field: "outputs",
          step: 2,
          message: "At least one output format must be selected.",
        });
      }
      if (form.outputs.uki) {
        if (ukiKeyMode === "manual") {
          if (!form.signing.ukiSecureBootKey.trim() || !form.signing.ukiSecureBootCert.trim()) {
            errs.push({
              field: "ukiSecureBootKey",
              step: 2,
              message: "UKI secure boot key and cert are required when UKI is enabled.",
            });
          }
        } else if (!form.signing.ukiKeySetId) {
          errs.push({
            field: "ukiKeySetId",
            step: 2,
            message: "A secure boot key set must be selected when UKI is enabled.",
          });
        }
      }
    }

    return errs;
  }

  // Per-step validation used by the Next button. Returns true when the
  // current step is clean; side-effects: sets the error banner and queues
  // a focus/scroll to the first invalid field on the same step.
  function validateStep(s: number): boolean {
    const errs = computeErrors(s);
    setErrors(errs.map((e) => e.message));
    if (errs.length > 0) focusFirstError(errs);
    return errs.length === 0;
  }

  // Two-phase focus: set the wizard step, then let the effect below run
  // once React has mounted the new step's DOM, and finally focus the first
  // invalid field. The detour through `focusTarget` state is required
  // because the target node doesn't exist yet in the same tick we change
  // the step.
  function focusFirstError(errs: FieldError[]) {
    if (errs.length === 0) return;
    const first = errs[0];
    setStep(first.step);
    setFocusTarget(first.field);
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    // Only allow submission from the Review step (prevents accidental Enter-key submits
    // from inputs or Radix component keyboard handlers on earlier steps).
    if (step !== 3) return;
    const fieldErrors = computeErrors();
    if (fieldErrors.length > 0) {
      setErrors(fieldErrors.map((e) => e.message));
      focusFirstError(fieldErrors);
      return;
    }
    setErrors([]);

    const input: CreateArtifactInput = {
      name: form.name || undefined,
      baseImage: form.baseImage,
      // Belt and braces: if the user cleared the Version field after we
      // prefilled it, still send the default so the backend never gets a
      // blank version.
      kairosVersion: form.kairosVersion.trim() || DEFAULT_ARTIFACT_VERSION,
      model: form.model,
      arch: form.arch,
      variant: form.variant,
      kubernetesDistro: form.variant === "standard" ? form.kubernetesDistro : undefined,
      kubernetesVersion: form.variant === "standard" ? form.kubernetesVersion : undefined,
      dockerfile: buildMode === "dockerfile" ? form.dockerfile : undefined,
      overlayRootfs: form.overlayRootfs || undefined,
      extendCmdline: form.extendCmdline || undefined,
      kairosInitImage: form.kairosInitImage || undefined,
      outputs: { ...form.outputs },
      signing: { ...form.signing },
      provisioning: {
        ...form.provisioning,
        userMode,
        username: userMode === "custom" ? username : undefined,
        password: userMode === "custom" ? password : undefined,
        sshKeys: userMode !== "none" && sshKeys.trim() ? sshKeys : undefined,
      },
      // Send only the user's extra YAML — backend builds the canonical document.
      cloudConfig: advancedConfig.trim() || undefined,
    };

    const result = await createArtifact(input);
    navigate(`/artifacts/${result.id}`);
  }

  const availableModels = modelsForArch(form.arch);

  const selectedOutputs = Object.entries(form.outputs)
    .filter(([, v]) => v)
    .map(([k]) => k);

  // Count only actual output formats (not security modifiers)
  const selectedOutputCount = OUTPUT_GROUPS.flatMap((g) => g.items).filter(
    (i) => form.outputs[i.field],
  ).length;

  // Flattened list of currently-picked outputs (with their tone) for the
  // ambient chip row under the step indicator. Rebuilt each render from
  // the same data the Output step uses, so the two views can't drift.
  const selectedOutputItems = OUTPUT_GROUPS.flatMap((g) =>
    g.items
      .filter((i) => form.outputs[i.field])
      .map((i) => ({ ...i, tone: g.tone })),
  );

  return (
    <div>
      <PageHeader
        title={cloneSource ? `Clone: ${cloneSource}` : "Build Artifact"}
        description={cloneSource ? "Modify and rebuild from an existing artifact" : "Configure and build a new OS artifact"}
      >
        <input
          ref={importInputRef}
          type="file"
          accept="application/json,.json"
          className="hidden"
          onChange={handleImportFile}
        />
        <Button type="button" variant="outline" size="sm" onClick={handleImportClick}>
          <FileUp className="h-4 w-4 mr-2" />
          Import
        </Button>
        <Button type="button" variant="outline" size="sm" onClick={handleExportConfig}>
          <Download className="h-4 w-4 mr-2" />
          Export
        </Button>
      </PageHeader>

      {/* Step indicator */}
      <div className="flex items-center gap-2 mb-4">
        {STEPS.map((name, i) => (
          <React.Fragment key={name}>
            {i > 0 && <div className={`flex-1 h-px ${i <= step ? "bg-[#EE5007]" : "bg-border"}`} />}
            <button
              type="button"
              onClick={() => setStep(i)}
              className={`flex items-center gap-2 text-sm ${i === step ? "text-[#EE5007] font-medium" : i < step ? "text-foreground" : "text-muted-foreground"}`}
            >
              <span className={`h-7 w-7 rounded-full flex items-center justify-center text-xs border ${i === step ? "border-[#EE5007] bg-[#EE5007] text-white" : i < step ? "border-[#EE5007] text-[#EE5007]" : "border-muted-foreground"}`}>
                {i < step ? "\u2713" : i + 1}
              </span>
              <span className="hidden md:inline">{name}</span>
            </button>
          </React.Fragment>
        ))}
      </div>

      {/* Ambient preview of the outputs the user has selected. Hidden when
          no outputs are picked so the row doesn't render a dangling label. */}
      {selectedOutputItems.length > 0 && (
        <div className="flex flex-wrap items-center gap-1.5 mb-6">
          <span className="text-xs text-muted-foreground shrink-0">Outputs:</span>
          {selectedOutputItems.map((item) => {
            const Icon = item.icon;
            return (
              <span
                key={item.field}
                className={`inline-flex items-center gap-1 text-[11px] font-medium px-2 py-0.5 rounded-full border ${OUTPUT_TONE_CLASSES[item.tone]}`}
              >
                <Icon className="h-3 w-3" />
                {item.label}
              </span>
            );
          })}
        </div>
      )}

      {/* Validation errors */}
      {errors.length > 0 && (
        <div className="mb-6 rounded-md bg-red-500/10 border border-red-500/25 p-4">
          <ul className="list-disc list-inside text-sm text-red-700 space-y-1">
            {errors.map((err, i) => (
              <li key={i}>{err}</li>
            ))}
          </ul>
        </div>
      )}

      <form onSubmit={handleSubmit}>
        {/* Step 0: Source */}
        {step === 0 && (
          <div>
            <div className="mb-6 max-w-md">
              <Label className="mb-2 block text-sm font-medium">
                Name
                <InfoTooltip>
                  Friendly identifier shown throughout AuroraBoot. Not used in the generated artifact filenames.
                </InfoTooltip>
              </Label>
              <Input
                placeholder="e.g. Production v4.0.3 + custom agent"
                value={form.name || ""}
                onChange={(e) => update("name", e.target.value)}
              />
              <p className="text-xs text-muted-foreground mt-1">
                A friendly name to identify this artifact.
              </p>
            </div>

            {!cloneSource && (
              <div className="mb-6">
                <Label className="mb-2 block text-sm font-medium">Start from a template</Label>
                <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
                  {TEMPLATES.map((t) => (
                    <Card
                      key={t.name}
                      className={`cursor-pointer transition-colors ${
                        selectedTemplate === t.name
                          ? "border-[#EE5007] bg-[#EE5007]/5 ring-1 ring-[#EE5007]/20"
                          : "hover:border-[#FF7442]/40"
                      }`}
                      onClick={() => {
                        setSelectedTemplate(t.name);
                        setForm((prev) => ({
                          ...EMPTY_FORM,
                          ...t.values,
                          name: prev.name, // preserve user-typed name
                          outputs: { ...EMPTY_OUTPUTS, ...t.values.outputs },
                          signing: { ...EMPTY_SIGNING, ...t.values.signing },
                          provisioning: { ...EMPTY_PROVISIONING, ...t.values.provisioning },
                        }));
                        setCustomModel(false);
                        // Custom stays on Source step; real templates advance to Configure
                        if (t.name !== "Custom") setStep(1);
                      }}
                    >
                      <CardContent className="p-3">
                        <p className="font-medium text-sm">{t.name}</p>
                        <p className="text-xs text-muted-foreground mt-1">{t.description}</p>
                      </CardContent>
                    </Card>
                  ))}
                </div>
              </div>
            )}

            {(selectedTemplate === "Custom" || cloneSource) && (
            <Card>
              <CardHeader>
                <CardTitle className="text-sm">Image Source</CardTitle>
              </CardHeader>
              <CardContent className="grid gap-4">
                <div className="flex gap-2">
                  <Button
                    type="button"
                    size="sm"
                    variant={buildMode === "image" ? "default" : "outline"}
                    onClick={() => { setBuildMode("image"); update("dockerfile", ""); }}
                  >
                    Base Image
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    variant={buildMode === "dockerfile" ? "default" : "outline"}
                    onClick={() => { setBuildMode("dockerfile"); }}
                  >
                    Dockerfile
                  </Button>
                </div>

                {buildMode === "image" ? (
                  <div className="grid gap-2">
                    <Label>
                      Base Image
                      <InfoTooltip>
                        Plain images (e.g. ubuntu:24.04) are auto-kairosified. Pre-built Kairos images are used directly.
                      </InfoTooltip>
                    </Label>
                    <Input
                      ref={bindRef("baseImage")}
                      placeholder="e.g. quay.io/kairos/hadron:v0.0.4-core-amd64-generic-v4.0.3"
                      value={form.baseImage}
                      onChange={(e) => update("baseImage", e.target.value)}
                    />
                  </div>
                ) : (
                  <div className="grid gap-2">
                    <Label>
                      Dockerfile
                      <InfoTooltip>
                        Build from a Dockerfile instead of a base image. Multi-stage is allowed; the final stage must produce a Kairos-derivable rootfs.{" "}
                        <a
                          href="https://kairos.io/docs/reference/build-from-scratch/"
                          target="_blank"
                          rel="noopener noreferrer"
                          className="underline"
                        >
                          Docs
                        </a>
                      </InfoTooltip>
                    </Label>
                    <Textarea
                      ref={bindRef("dockerfile")}
                      placeholder={"FROM golang:1.26-alpine AS builder\nRUN ...\n\nFROM quay.io/kairos/hadron:v0.0.4-core-amd64-generic-v4.0.3\nCOPY --from=builder /app /usr/sbin/app"}
                      value={form.dockerfile || ""}
                      onChange={(e) => update("dockerfile", e.target.value)}
                      rows={8}
                      className="font-mono text-sm"
                    />
                  </div>
                )}
              </CardContent>
            </Card>
            )}
          </div>
        )}

        {/* Step 1: Configure */}
        {step === 1 && (
          <div className="grid gap-6">
            {/* Architecture */}
            <Card>
              <CardHeader>
                <CardTitle className="text-sm">
                  Architecture
                  <InfoTooltip>
                    CPU architecture of the target hardware. Determines which base rootfs is pulled and which systemd-boot stub is used for UKI builds.
                  </InfoTooltip>
                </CardTitle>
              </CardHeader>
              <CardContent className="grid gap-3 md:grid-cols-2">
                {ARCHES.map((a) => {
                  const Icon = a.icon;
                  const selected = form.arch === a.value;
                  return (
                    <button
                      key={a.value}
                      type="button"
                      onClick={() => handleArchChange(a.value)}
                      className={`text-left rounded-lg border p-4 transition-colors ${
                        selected
                          ? "border-[#EE5007] bg-[#EE5007]/5 ring-1 ring-[#EE5007]"
                          : "border-border hover:border-[#EE5007]/50 hover:bg-muted/40"
                      }`}
                    >
                      <div className="flex items-start gap-3">
                        <Icon className={`h-5 w-5 mt-0.5 ${selected ? "text-[#EE5007]" : "text-muted-foreground"}`} />
                        <div className="flex-1">
                          <div className="flex items-center gap-2">
                            <span className="font-medium">{a.label}</span>
                            {selected && <Check className="h-4 w-4 text-[#EE5007]" />}
                          </div>
                          <p className="text-xs text-muted-foreground mt-1">{a.desc}</p>
                        </div>
                      </div>
                    </button>
                  );
                })}
              </CardContent>
            </Card>

            {/* Model */}
            <Card>
              <CardHeader>
                <CardTitle className="text-sm flex items-center justify-between">
                  <span>
                    Model
                    <InfoTooltip>
                      Hardware model determines board-specific drivers and firmware bundled into the image. Only models compatible with the selected architecture are shown.
                    </InfoTooltip>
                  </span>
                  <button
                    type="button"
                    className="text-xs font-normal text-muted-foreground hover:text-foreground underline-offset-4 hover:underline"
                    onClick={() => {
                      if (customModel) {
                        setCustomModel(false);
                        const models = modelsForArch(form.arch);
                        if (models.length > 0) update("model", models[0].value);
                      } else {
                        setCustomModel(true);
                        update("model", "");
                      }
                    }}
                  >
                    {customModel ? "Use presets" : "Custom\u2026"}
                  </button>
                </CardTitle>
              </CardHeader>
              <CardContent>
                {customModel ? (
                  <Input
                    placeholder="e.g. generic"
                    value={form.model}
                    onChange={(e) => update("model", e.target.value)}
                  />
                ) : (
                  <div className="grid gap-3 md:grid-cols-2">
                    {availableModels.map((m) => {
                      const Icon = m.icon;
                      const selected = form.model === m.value;
                      return (
                        <button
                          key={m.value}
                          type="button"
                          onClick={() => update("model", m.value)}
                          className={`text-left rounded-lg border p-4 transition-colors ${
                            selected
                              ? "border-[#EE5007] bg-[#EE5007]/5 ring-1 ring-[#EE5007]"
                              : "border-border hover:border-[#EE5007]/50 hover:bg-muted/40"
                          }`}
                        >
                          <div className="flex items-start gap-3">
                            <Icon className={`h-5 w-5 mt-0.5 ${selected ? "text-[#EE5007]" : "text-muted-foreground"}`} />
                            <div className="flex-1">
                              <div className="flex items-center gap-2">
                                <span className="font-medium">{m.label}</span>
                                {selected && <Check className="h-4 w-4 text-[#EE5007]" />}
                              </div>
                              <p className="text-xs text-muted-foreground mt-1">{m.desc}</p>
                            </div>
                          </div>
                        </button>
                      );
                    })}
                  </div>
                )}
              </CardContent>
            </Card>

            {/* Variant */}
            <Card>
              <CardHeader>
                <CardTitle className="text-sm">
                  Variant
                  <InfoTooltip>
                    Core is a minimal OS with only cloud-init. Standard bundles a Kubernetes distribution (K3s or K0s) into the image.
                  </InfoTooltip>
                </CardTitle>
              </CardHeader>
              <CardContent className="grid gap-3 md:grid-cols-2">
                {VARIANTS.map((v) => {
                  const Icon = v.icon;
                  const selected = form.variant === v.value;
                  return (
                    <button
                      key={v.value}
                      type="button"
                      onClick={() => update("variant", v.value)}
                      className={`text-left rounded-lg border p-4 transition-colors ${
                        selected
                          ? "border-[#EE5007] bg-[#EE5007]/5 ring-1 ring-[#EE5007]"
                          : "border-border hover:border-[#EE5007]/50 hover:bg-muted/40"
                      }`}
                    >
                      <div className="flex items-start gap-3">
                        <Icon className={`h-5 w-5 mt-0.5 ${selected ? "text-[#EE5007]" : "text-muted-foreground"}`} />
                        <div className="flex-1">
                          <div className="flex items-center gap-2">
                            <span className="font-medium">{v.label}</span>
                            {selected && <Check className="h-4 w-4 text-[#EE5007]" />}
                          </div>
                          <p className="text-xs text-muted-foreground mt-1">{v.desc}</p>
                        </div>
                      </div>
                    </button>
                  );
                })}
              </CardContent>
            </Card>

            {/* Kubernetes fields (only when variant=standard) */}
            {form.variant === "standard" && (
              <Card>
                <CardHeader>
                  <CardTitle className="text-sm">Kubernetes</CardTitle>
                </CardHeader>
                <CardContent className="grid gap-4">
                  <div className="grid gap-2">
                    <Label>
                      Distribution
                      <InfoTooltip>
                        k3s or k0s, bundled into the image for offline installs.{" "}
                        <a
                          href="https://kairos.io/docs/reference/image_matrix/"
                          target="_blank"
                          rel="noopener noreferrer"
                          className="underline"
                        >
                          Docs
                        </a>
                      </InfoTooltip>
                    </Label>
                    <div className="flex gap-2">
                      {(["k3s", "k0s"] as const).map((d, idx) => (
                        <Button
                          key={d}
                          ref={idx === 0 ? bindRef("kubernetesDistro") : undefined}
                          type="button"
                          size="sm"
                          variant={form.kubernetesDistro === d ? "default" : "outline"}
                          onClick={() => update("kubernetesDistro", d)}
                        >
                          {d.toUpperCase()}
                        </Button>
                      ))}
                    </div>
                  </div>
                  <div className="grid gap-2">
                    <Label>
                      Version (optional)
                      <InfoTooltip>
                        Pinned Kubernetes version. Leave empty to take whatever the chosen distro defaults to.
                      </InfoTooltip>
                    </Label>
                    <Input
                      placeholder="e.g. v1.28.0"
                      value={form.kubernetesVersion || ""}
                      onChange={(e) => update("kubernetesVersion", e.target.value)}
                    />
                  </div>
                </CardContent>
              </Card>
            )}

            {/* Version */}
            <Card>
              <CardHeader>
                <CardTitle className="text-sm">Version</CardTitle>
              </CardHeader>
              <CardContent>
                <div className="grid gap-2">
                  <Label>
                    Artifact version
                    <InfoTooltip>
                      The version <em>you</em> are releasing with this build. It gets
                      baked into the image and is how AuroraBoot decides whether a node is
                      up-to-date or needs an upgrade.
                      <br />
                      <br />
                      Defaults to <code>v1.0</code> if you leave it blank. Bump it every
                      time you ship a new build so your fleet can pick up the change.{" "}
                      <a
                        href="https://kairos.io/docs/upgrade/"
                        target="_blank"
                        rel="noopener noreferrer"
                      >
                        Upgrade docs
                      </a>
                    </InfoTooltip>
                  </Label>
                  <Input
                    ref={bindRef("kairosVersion")}
                    placeholder="v1.0"
                    value={form.kairosVersion}
                    onChange={(e) => update("kairosVersion", e.target.value)}
                  />
                  <p className="text-xs text-muted-foreground">
                    Used for upgrade tracking across your fleet. Leave blank to use{" "}
                    <code className="font-mono">v1.0</code>.
                  </p>
                </div>
              </CardContent>
            </Card>

            {/* Access & Security */}
            <Card>
              <CardHeader>
                <CardTitle className="text-sm">Access &amp; Security</CardTitle>
              </CardHeader>
              <CardContent className="grid gap-4">
                <div className="grid gap-2">
                  <Label>
                    User Setup
                    <InfoTooltip>
                      How the default login is provisioned on first boot: none, a default <code>kairos</code>/<code>kairos</code> user, or a custom one.{" "}
                      <a
                        href="https://kairos.io/docs/reference/configuration/"
                        target="_blank"
                        rel="noopener noreferrer"
                        className="underline"
                      >
                        Docs
                      </a>
                    </InfoTooltip>
                  </Label>
                  <div className="flex gap-2">
                    {(["default", "custom", "none"] as const).map((mode) => (
                      <Button
                        key={mode}
                        type="button"
                        size="sm"
                        variant={userMode === mode ? "default" : "outline"}
                        onClick={() => {
                          setUserMode(mode);
                          if (mode === "default") {
                            setUsername("kairos");
                            setPassword("kairos");
                          }
                        }}
                      >
                        {mode === "default" ? "Default User" : mode === "custom" ? "Custom User" : "No User"}
                      </Button>
                    ))}
                  </div>
                </div>

                {userMode !== "none" && (
                  <div className="grid grid-cols-2 gap-3">
                    <div className="grid gap-2">
                      <Label>
                        Username
                        <InfoTooltip>
                          Login user created at first boot.
                        </InfoTooltip>
                      </Label>
                      <Input
                        value={username}
                        onChange={(e) => setUsername(e.target.value)}
                        disabled={userMode === "default"}
                      />
                    </div>
                    <div className="grid gap-2">
                      <Label>
                        Password
                        <InfoTooltip>
                          Stored as plain text in the generated cloud-config. Prefer SSH keys for anything you actually care about.
                        </InfoTooltip>
                      </Label>
                      <Input
                        value={password}
                        onChange={(e) => setPassword(e.target.value)}
                        disabled={userMode === "default"}
                      />
                    </div>
                  </div>
                )}

                {userMode === "none" && (
                  <div className="rounded-md bg-amber-500/10 border border-amber-500/25 p-3">
                    <p className="text-sm text-amber-700">
                      No user will be created. You must configure access via the advanced cloud-config section in the Output step.
                    </p>
                  </div>
                )}

                {userMode !== "none" && (
                  <div className="grid gap-2">
                    <Label>
                      SSH Authorized Keys (optional)
                      <InfoTooltip>
                        One key per line. Kairos also accepts <code>github:user</code> and <code>gitlab:user</code> shortcuts.{" "}
                        <a
                          href="https://kairos.io/docs/reference/configuration/"
                          target="_blank"
                          rel="noopener noreferrer"
                          className="underline"
                        >
                          Docs
                        </a>
                      </InfoTooltip>
                    </Label>
                    <Textarea
                      placeholder={"ssh-rsa AAAAB3... user@host\nssh-ed25519 AAAA... other@host"}
                      value={sshKeys}
                      onChange={(e) => setSshKeys(e.target.value)}
                      rows={3}
                      className="font-mono text-xs"
                    />
                    <p className="text-xs text-muted-foreground">One key per line.</p>
                  </div>
                )}
              </CardContent>
            </Card>
          </div>
        )}

        {/* Step 2: Output */}
        {step === 2 && (
          <div className="grid gap-6 lg:grid-cols-5">
            {/* LEFT: Outputs (primary) */}
            <div className="lg:col-span-3 space-y-6">
              <Card ref={bindRef("outputs")}>
                <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-3">
                  <CardTitle className="text-sm">Outputs</CardTitle>
                  <span className="text-xs text-muted-foreground">
                    {selectedOutputCount} selected
                  </span>
                </CardHeader>
                <CardContent className="space-y-6">
                  {OUTPUT_GROUPS.map((group) => (
                    <div key={group.title}>
                      <p className="text-xs font-medium text-muted-foreground mb-3 uppercase tracking-wide">
                        {group.title}
                      </p>
                      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                        {group.items.map((item) => {
                          const Icon = item.icon;
                          const checked = !!form.outputs[item.field];
                          return (
                            <button
                              key={item.field}
                              type="button"
                              onClick={() => updateOutput(item.field, !checked)}
                              className={`relative text-left rounded-lg border p-3 transition-colors ${
                                checked
                                  ? "border-[#EE5007] bg-[#EE5007]/5 ring-1 ring-[#EE5007]/20"
                                  : "border-border hover:border-[#FF7442]/40"
                              }`}
                            >
                              {checked && (
                                <span className="absolute top-2 right-2 h-4 w-4 rounded-full bg-[#EE5007] text-white flex items-center justify-center">
                                  <Check className="h-3 w-3" strokeWidth={3} />
                                </span>
                              )}
                              <Icon
                                className={`h-5 w-5 mb-2 ${
                                  checked ? "text-[#EE5007]" : "text-muted-foreground"
                                }`}
                              />
                              <p className="font-medium text-sm">{item.label}</p>
                              <p className="text-xs text-muted-foreground mt-0.5">{item.desc}</p>
                            </button>
                          );
                        })}
                      </div>
                    </div>
                  ))}

                  {form.outputs.uki && (
                    <div className="rounded-md bg-amber-500/10 border border-amber-500/25 p-3 flex gap-2">
                      <AlertTriangle className="h-4 w-4 text-amber-600 shrink-0 mt-0.5" />
                      <p className="text-sm text-amber-700">
                        UKI requires secure boot signing keys — configure them in the panel below.
                      </p>
                    </div>
                  )}
                </CardContent>
              </Card>

              {/* UKI Signing Keys (only when UKI selected) */}
              {form.outputs.uki && (
                <Card>
                  <CardHeader>
                    <CardTitle className="text-sm flex items-center gap-2">
                      <ShieldCheck className="h-4 w-4 text-[#EE5007]" />
                      UKI Signing Keys
                    </CardTitle>
                  </CardHeader>
                  <CardContent className="space-y-4">
                    <div className="flex gap-2">
                      <Button
                        type="button"
                        size="sm"
                        variant={ukiKeyMode === "keyset" ? "default" : "outline"}
                        onClick={() => setUkiKeyMode("keyset")}
                      >
                        Saved Key Set
                      </Button>
                      <Button
                        type="button"
                        size="sm"
                        variant={ukiKeyMode === "manual" ? "default" : "outline"}
                        onClick={() => setUkiKeyMode("manual")}
                      >
                        Manual Paths
                      </Button>
                    </div>

                    {ukiKeyMode === "keyset" ? (
                      <Select
                        value={form.signing.ukiKeySetId || "__none__"}
                        onValueChange={(v) => updateSigning("ukiKeySetId", v === "__none__" ? "" : v)}
                      >
                        <SelectTrigger ref={bindRef("ukiKeySetId")}>
                          <SelectValue placeholder="Select key set" />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="__none__">Select a key set...</SelectItem>
                          {keySets.map((ks) => (
                            <SelectItem key={ks.id} value={ks.id}>
                              {ks.name}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    ) : (
                      <div className="grid gap-3">
                        <div className="grid gap-1">
                          <Label className="text-xs">
                            Secure Boot Key
                            <InfoTooltip>
                              PEM private key that signs the UKI. Must match the enrolled PK/KEK/db on the target firmware.{" "}
                              <a
                                href="https://kairos.io/docs/reference/auroraboot/"
                                target="_blank"
                                rel="noopener noreferrer"
                                className="underline"
                              >
                                Docs
                              </a>
                            </InfoTooltip>
                          </Label>
                          <Input
                            ref={bindRef("ukiSecureBootKey")}
                            placeholder="/path/to/sb.key"
                            value={form.signing.ukiSecureBootKey}
                            onChange={(e) => updateSigning("ukiSecureBootKey", e.target.value)}
                            className="font-mono text-xs"
                          />
                        </div>
                        <div className="grid gap-1">
                          <Label className="text-xs">
                            Secure Boot Cert
                            <InfoTooltip>
                              PEM certificate paired with the signing key. Used at sign time and enrolled into the firmware.
                            </InfoTooltip>
                          </Label>
                          <Input
                            placeholder="/path/to/sb.pem"
                            value={form.signing.ukiSecureBootCert}
                            onChange={(e) => updateSigning("ukiSecureBootCert", e.target.value)}
                            className="font-mono text-xs"
                          />
                        </div>
                        <div className="grid gap-1">
                          <Label className="text-xs">
                            TPM PCR Key
                            <InfoTooltip>
                              Private key used to sign the EFI PCR policy. PEM path or a PKCS11 URI.
                            </InfoTooltip>
                          </Label>
                          <Input
                            placeholder="/path/to/pcr.key"
                            value={form.signing.ukiTpmPcrKey}
                            onChange={(e) => updateSigning("ukiTpmPcrKey", e.target.value)}
                            className="font-mono text-xs"
                          />
                        </div>
                        <div className="grid gap-1">
                          <Label className="text-xs">Public Keys Dir</Label>
                          <Input
                            placeholder="/path/to/public-keys/"
                            value={form.signing.ukiPublicKeysDir}
                            onChange={(e) => updateSigning("ukiPublicKeysDir", e.target.value)}
                            className="font-mono text-xs"
                          />
                        </div>
                        <div className="grid gap-1">
                          <Label className="text-xs">Enrollment Policy</Label>
                          <Select
                            value={form.signing.ukiSecureBootEnroll}
                            onValueChange={(v) => updateSigning("ukiSecureBootEnroll", v)}
                          >
                            <SelectTrigger>
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectItem value="if-safe">if-safe</SelectItem>
                              <SelectItem value="force">force</SelectItem>
                              <SelectItem value="manual">manual</SelectItem>
                            </SelectContent>
                          </Select>
                        </div>
                      </div>
                    )}
                  </CardContent>
                </Card>
              )}
            </div>

            {/* RIGHT: Build options (secondary) */}
            <div className="lg:col-span-2 space-y-6">
              {/* Provisioning */}
              <Card>
                <CardHeader className="pb-3">
                  <CardTitle className="text-sm">Provisioning</CardTitle>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div>
                    <label className="flex items-center gap-2 text-sm font-medium">
                      <input
                        type="checkbox"
                        checked={form.provisioning.autoInstall}
                        onChange={(e) => updateProvisioning("autoInstall", e.target.checked)}
                        className="rounded border-input"
                      />
                      Auto-install
                    </label>
                    <p className="text-xs text-muted-foreground mt-1 ml-6">
                      Install to disk on first boot, no interaction needed.
                    </p>
                    {/* When the user disables auto-install we nudge them toward
                        the upstream Kairos docs so they know how to drive the
                        install by hand — otherwise a booted image just sits at
                        the live prompt and looks broken. Colors are picked for
                        WCAG AA contrast against the light/dark backgrounds. */}
                    {!form.provisioning.autoInstall && (
                      <div className="mt-2 ml-6 rounded-md border border-amber-400/60 bg-amber-50 px-3 py-2 text-xs text-amber-900 dark:border-amber-400/40 dark:bg-amber-950/60 dark:text-amber-100">
                        With auto-install off, the image boots into a live
                        environment and you install it manually.{" "}
                        <a
                          href="https://kairos.io/docs/installation/"
                          target="_blank"
                          rel="noopener noreferrer"
                          className="font-semibold underline underline-offset-2 hover:text-amber-950 dark:hover:text-white"
                        >
                          Read the installation guide →
                        </a>
                      </div>
                    )}
                  </div>

                  <div>
                    <label className="flex items-center gap-2 text-sm font-medium">
                      <input
                        type="checkbox"
                        checked={form.provisioning.registerAuroraBoot}
                        onChange={(e) => updateProvisioning("registerAuroraBoot", e.target.checked)}
                        className="rounded border-input"
                      />
                      Register with AuroraBoot
                    </label>
                    <p className="text-xs text-muted-foreground mt-1 ml-6">
                      Node will phone home to this AuroraBoot on boot.
                    </p>
                  </div>

                  {form.provisioning.registerAuroraBoot && (
                    <div className="grid gap-2 pt-1">
                      <Label className="text-xs">
                        Target Group
                        <InfoTooltip>
                          Assigns the registered node to a fleet group on first boot. Groups are managed from the Groups page.
                        </InfoTooltip>
                      </Label>
                      <Select
                        value={form.provisioning.targetGroupId || "__none__"}
                        onValueChange={(v) => updateProvisioning("targetGroupId", v === "__none__" ? "" : v)}
                      >
                        <SelectTrigger>
                          <SelectValue placeholder="Select group (optional)" />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="__none__">None</SelectItem>
                          {groups.map((g) => (
                            <SelectItem key={g.id} value={g.id}>
                              {g.name}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                  )}

                  {form.provisioning.registerAuroraBoot && (
                    <AllowedCommandsPicker
                      value={form.provisioning.allowedCommands ?? []}
                      onChange={(next) => updateProvisioning("allowedCommands", next)}
                    />
                  )}
                </CardContent>
              </Card>

              {/* Security */}
              <Card>
                <CardHeader className="pb-3">
                  <CardTitle className="text-sm">Security</CardTitle>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div>
                    <label className="flex items-center gap-2 text-sm font-medium">
                      <input
                        type="checkbox"
                        checked={!!form.outputs.fips}
                        onChange={(e) => updateOutput("fips", e.target.checked)}
                        className="rounded border-input"
                      />
                      FIPS
                    </label>
                    <p className="text-xs text-muted-foreground mt-1 ml-6">
                      Build with FIPS-validated cryptographic modules.
                    </p>
                  </div>
                  <div>
                    <label className="flex items-center gap-2 text-sm font-medium">
                      <input
                        type="checkbox"
                        checked={!!form.outputs.trustedBoot}
                        onChange={(e) => updateOutput("trustedBoot", e.target.checked)}
                        className="rounded border-input"
                      />
                      Trusted Boot
                    </label>
                    <p className="text-xs text-muted-foreground mt-1 ml-6">
                      Enable measured boot with TPM verification.
                    </p>
                  </div>
                </CardContent>
              </Card>

              {/* Overlay Files */}
              <Card>
                <CardHeader className="pb-3">
                  <CardTitle className="text-sm">Overlay Files</CardTitle>
                </CardHeader>
                <CardContent>
                  <div
                    onDragOver={(e) => { e.preventDefault(); setDragOver(true); }}
                    onDragLeave={() => setDragOver(false)}
                    onDrop={async (e) => {
                      e.preventDefault();
                      setDragOver(false);
                      setOverlayUploading(true);
                      try {
                        const path = await uploadOverlayFiles(e.dataTransfer.files);
                        update("overlayRootfs", path);
                        setOverlayFiles(Array.from(e.dataTransfer.files).map(f => f.name));
                      } catch { /* ignore */ }
                      setOverlayUploading(false);
                    }}
                    className={`border-2 border-dashed rounded-lg p-5 text-center cursor-pointer transition-colors ${
                      dragOver ? "border-[#EE5007] bg-[#EE5007]/5" : "border-muted-foreground/25 hover:border-muted-foreground/50"
                    }`}
                    onClick={() => overlayInputRef.current?.click()}
                  >
                    {overlayUploading ? (
                      <div className="flex items-center justify-center gap-2">
                        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
                        <span className="text-sm text-muted-foreground">Uploading...</span>
                      </div>
                    ) : overlayFiles.length > 0 ? (
                      <div className="space-y-2">
                        <div className="flex flex-wrap gap-1.5 justify-center">
                          {overlayFiles.map((name) => (
                            <span key={name} className="text-xs bg-secondary px-2 py-1 rounded font-mono">{name}</span>
                          ))}
                        </div>
                        <button
                          type="button"
                          className="text-xs text-red-500 hover:text-red-700 flex items-center gap-1 mx-auto"
                          onClick={(e) => {
                            e.stopPropagation();
                            update("overlayRootfs", "");
                            setOverlayFiles([]);
                          }}
                        >
                          <X className="h-3 w-3" /> Clear files
                        </button>
                      </div>
                    ) : (
                      <>
                        <Upload className="h-7 w-7 mx-auto text-muted-foreground/40 mb-2" />
                        <p className="text-sm text-muted-foreground">Drop files or a .tar.gz here</p>
                        <p className="text-xs text-muted-foreground/60 mt-1">Overlaid on top of the rootfs</p>
                      </>
                    )}
                    <input
                      ref={overlayInputRef}
                      type="file"
                      multiple
                      className="hidden"
                      onChange={async (e) => {
                        if (!e.target.files?.length) return;
                        setOverlayUploading(true);
                        try {
                          const path = await uploadOverlayFiles(e.target.files);
                          update("overlayRootfs", path);
                          setOverlayFiles(Array.from(e.target.files).map(f => f.name));
                        } catch { /* ignore */ }
                        setOverlayUploading(false);
                        e.target.value = "";
                      }}
                    />
                  </div>
                </CardContent>
              </Card>

              {/* Advanced — single collapsible card containing both Cloud Config + Kairos Init Image */}
              <Card>
                <button
                  type="button"
                  className="w-full flex items-center justify-between px-6 py-4 text-sm font-medium hover:bg-muted/30 transition-colors rounded-t-lg"
                  onClick={() => setShowAdvanced(!showAdvanced)}
                >
                  <span className="flex items-center gap-2">
                    {showAdvanced ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
                    Advanced
                  </span>
                  <span className="text-xs text-muted-foreground">Cloud config &amp; init image</span>
                </button>
                {showAdvanced && (
                  <CardContent className="space-y-5 pt-0">
                    <div className="grid gap-2">
                      <Label className="text-xs">
                        Cloud Config (appended)
                        <InfoTooltip>
                          Raw YAML appended to the generated #cloud-config. Use for custom stages, units, or packages.{" "}
                          <a
                            href="https://kairos.io/docs/reference/configuration/"
                            target="_blank"
                            rel="noopener noreferrer"
                            className="underline"
                          >
                            Docs
                          </a>
                        </InfoTooltip>
                      </Label>
                      <Textarea
                        placeholder={"# Additional cloud-config\nstages:\n  boot:\n    - name: disable-ssh\n      commands:\n        - systemctl disable sshd"}
                        value={advancedConfig}
                        onChange={(e) => setAdvancedConfig(e.target.value)}
                        rows={6}
                        className="font-mono text-xs"
                      />
                      <p className="text-xs text-muted-foreground">
                        Appended to the generated config. AuroraBoot registration is auto-injected by the server.
                      </p>
                    </div>
                    <div className="grid gap-2">
                      <Label className="text-xs">
                        Extend Cmdline
                        <InfoTooltip>
                          Extra kernel command line arguments. Advanced — leave empty to use the default.{" "}
                          <a
                            href="https://kairos.io/docs/reference/kairos-factory/"
                            target="_blank"
                            rel="noopener noreferrer"
                            className="underline"
                          >
                            Docs
                          </a>
                        </InfoTooltip>
                      </Label>
                      <Input
                        placeholder="e.g. intel_iommu=on"
                        value={form.extendCmdline || ""}
                        onChange={(e) => update("extendCmdline", e.target.value)}
                        className="font-mono text-xs"
                      />
                      <p className="text-xs text-muted-foreground">
                        Extra kernel command line arguments.
                      </p>
                    </div>
                    <div className="grid gap-2">
                      <Label className="text-xs">
                        Kairos Init Image
                        <InfoTooltip>
                          Override the kairos-init container used during the build. Advanced — leave empty to use the default.{" "}
                          <a
                            href="https://kairos.io/docs/reference/kairos-factory/"
                            target="_blank"
                            rel="noopener noreferrer"
                            className="underline"
                          >
                            Docs
                          </a>
                        </InfoTooltip>
                      </Label>
                      <Input
                        placeholder="e.g. quay.io/kairos/kairos-init:latest"
                        value={form.kairosInitImage || ""}
                        onChange={(e) => update("kairosInitImage", e.target.value)}
                        className="font-mono text-xs"
                      />
                      <p className="text-xs text-muted-foreground">
                        Override the default kairos-init image used when kairosifying base distros.
                      </p>
                    </div>
                  </CardContent>
                )}
              </Card>
            </div>
          </div>
        )}

        {/* Step 3: Review */}
        {step === 3 && (
          <div className="grid gap-6">
            <Card>
              <CardHeader>
                <CardTitle className="text-sm">Review Build Configuration</CardTitle>
              </CardHeader>
              <CardContent className="grid gap-4">
                {/* Source */}
                <div>
                  <p className="text-xs font-medium text-muted-foreground mb-1 uppercase tracking-wide">Source</p>
                  <div className="grid gap-1 text-sm">
                    {form.name && (
                      <div className="flex gap-2">
                        <span className="text-muted-foreground w-28 shrink-0">Name:</span>
                        <span>{form.name}</span>
                      </div>
                    )}
                    <div className="flex gap-2">
                      <span className="text-muted-foreground w-28 shrink-0">Image source:</span>
                      <span className="font-mono text-xs break-all">
                        {buildMode === "dockerfile" ? "(Dockerfile)" : form.baseImage || "\u2014"}
                      </span>
                    </div>
                  </div>
                </div>

                {/* Configuration */}
                <div className="border-t pt-3">
                  <p className="text-xs font-medium text-muted-foreground mb-1 uppercase tracking-wide">Configuration</p>
                  <div className="grid gap-1 text-sm">
                    <div className="flex gap-2">
                      <span className="text-muted-foreground w-28 shrink-0">Architecture:</span>
                      <span>{form.arch}</span>
                    </div>
                    <div className="flex gap-2">
                      <span className="text-muted-foreground w-28 shrink-0">Model:</span>
                      <span>{form.model}</span>
                    </div>
                    <div className="flex gap-2">
                      <span className="text-muted-foreground w-28 shrink-0">Variant:</span>
                      <span>{form.variant}</span>
                    </div>
                    {form.variant === "standard" && (
                      <>
                        <div className="flex gap-2">
                          <span className="text-muted-foreground w-28 shrink-0">K8s Distro:</span>
                          <span>{form.kubernetesDistro || "\u2014"}</span>
                        </div>
                        {form.kubernetesVersion && (
                          <div className="flex gap-2">
                            <span className="text-muted-foreground w-28 shrink-0">K8s Version:</span>
                            <span>{form.kubernetesVersion}</span>
                          </div>
                        )}
                      </>
                    )}
                    <div className="flex gap-2">
                      <span className="text-muted-foreground w-28 shrink-0">Version:</span>
                      <span>{form.kairosVersion || DEFAULT_ARTIFACT_VERSION}</span>
                    </div>
                  </div>
                </div>

                {/* Outputs */}
                <div className="border-t pt-3">
                  <p className="text-xs font-medium text-muted-foreground mb-2 uppercase tracking-wide">Outputs</p>
                  {selectedOutputs.length > 0 ? (
                    <div className="flex flex-wrap gap-1.5">
                      {selectedOutputs.map((o) => (
                        <span key={o} className="inline-flex items-center rounded-full bg-[#EE5007]/10 px-2.5 py-0.5 text-xs font-medium text-[#EE5007]">
                          {o}
                        </span>
                      ))}
                    </div>
                  ) : (
                    <p className="text-sm text-muted-foreground">No outputs selected</p>
                  )}
                </div>

                {/* Provisioning */}
                <div className="border-t pt-3">
                  <p className="text-xs font-medium text-muted-foreground mb-1 uppercase tracking-wide">Provisioning</p>
                  <div className="grid gap-1 text-sm">
                    <div className="flex gap-2">
                      <span className="text-muted-foreground w-28 shrink-0">Auto-install:</span>
                      <span>{form.provisioning.autoInstall ? "Yes" : "No"}</span>
                    </div>
                    <div className="flex gap-2">
                      <span className="text-muted-foreground w-28 shrink-0">Register:</span>
                      <span>{form.provisioning.registerAuroraBoot ? "Yes" : "No"}</span>
                    </div>
                    {form.provisioning.registerAuroraBoot && form.provisioning.targetGroupId && (
                      <div className="flex gap-2">
                        <span className="text-muted-foreground w-28 shrink-0">Target Group:</span>
                        <span>{groups.find((g) => g.id === form.provisioning.targetGroupId)?.name || form.provisioning.targetGroupId}</span>
                      </div>
                    )}
                    {form.provisioning.registerAuroraBoot && (
                      <div className="flex gap-2">
                        <span className="text-muted-foreground w-28 shrink-0">Allowed cmds:</span>
                        {(form.provisioning.allowedCommands ?? []).length === 0 ? (
                          <span className="text-amber-700 dark:text-amber-300">Observe-only (no commands)</span>
                        ) : (
                          <span className="font-mono text-xs">
                            {(form.provisioning.allowedCommands ?? []).join(", ")}
                          </span>
                        )}
                      </div>
                    )}
                  </div>
                </div>

                {/* Cloud Config Preview */}
                {(advancedConfig.trim() || userMode !== "default") && (
                  <div className="border-t pt-3">
                    <p className="text-xs font-medium text-muted-foreground mb-1 uppercase tracking-wide">Cloud Config Preview</p>
                    <pre className="text-xs font-mono bg-muted/50 rounded p-3 overflow-x-auto max-h-40 overflow-y-auto whitespace-pre-wrap">
                      {buildCloudConfig().slice(0, 500)}{buildCloudConfig().length > 500 ? "\n..." : ""}
                    </pre>
                  </div>
                )}
              </CardContent>
            </Card>
          </div>
        )}

        {/* Navigation */}
        <div className="flex justify-between mt-8">
          {step > 0 ? (
            <Button type="button" variant="outline" onClick={() => setStep(step - 1)}>Back</Button>
          ) : (
            <Button type="button" variant="outline" onClick={() => navigate("/artifacts")}>Cancel</Button>
          )}
          <div className="flex-1" />
          {step < 3 ? (
            <Button
              type="button"
              onClick={() => { if (validateStep(step)) setStep(step + 1); }}
              className="bg-[#EE5007] hover:bg-[#FF7442] text-white"
            >
              Next
            </Button>
          ) : (
            <Button
              type="button"
              onClick={() => { void handleSubmit({ preventDefault: () => {} } as FormEvent); }}
              className="bg-[#EE5007] hover:bg-[#FF7442] text-white"
            >
              Start Build
            </Button>
          )}
        </div>
      </form>
    </div>
  );
}

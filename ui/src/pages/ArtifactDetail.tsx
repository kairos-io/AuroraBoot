import { useEffect, useState, useRef, useCallback, useMemo } from "react";
import { useParams, useNavigate } from "react-router-dom";
import {
  getArtifact,
  getArtifactLogs,
  cancelArtifact,
  deleteArtifact,
  updateArtifact,
  artifactDownloadUrl,
  type Artifact,
} from "@/api/artifacts";
import { StatusBadge } from "@/components/StatusBadge";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Download,
  XCircle,
  Trash2,
  Bookmark,
  Pencil,
  Check,
  Copy,
  Rocket,
  FileDown,
  Box,
  Cpu,
  Server,
  Layers,
  Disc3,
  Wifi,
  ShieldCheck,
  HardDrive,
  Cloud,
  CloudCog,
  Package,
  FileCode,
  Tag,
  UserCheck,
  Clock,
  AlertTriangle,
  CheckCircle2,
  Terminal as TerminalIcon,
  ArrowDown,
  WrapText,
  Loader2,
} from "lucide-react";
import { DeployDialog } from "@/components/DeployDialog";
import { ansiToHtml } from "@/lib/ansi";
import { listGroups, type Group } from "@/api/groups";
import { downloadBuildConfig, payloadFromArtifact } from "@/lib/buildConfig";
import { toast } from "@/hooks/useToast";
import { useUIWebSocket } from "@/hooks/useUIWebSocket";

// formatDuration renders a build duration as a compact, humane string.
// Examples: "47s", "4m 21s", "1h 12m".
function formatDuration(totalSeconds: number): string {
  if (totalSeconds <= 0) return "0s";
  const h = Math.floor(totalSeconds / 3600);
  const m = Math.floor((totalSeconds % 3600) / 60);
  const s = totalSeconds % 60;
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m ${s}s`;
  return `${s}s`;
}

// Color classes for the three output category tones.
const TONE_CLASSES: Record<"orange" | "blue" | "neutral", string> = {
  orange: "border-[#EE5007]/30 bg-[#EE5007]/10 text-[#C73F00]",
  blue: "border-sky-500/30 bg-sky-500/10 text-sky-700",
  neutral: "border-border bg-muted/60 text-foreground",
};

export function ArtifactDetail() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [artifact, setArtifact] = useState<Artifact | null>(null);
  const [groups, setGroups] = useState<Group[]>([]);
  const [logs, setLogs] = useState<string>("");
  const [cancelling, setCancelling] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [editingName, setEditingName] = useState(false);
  const [showDeploy, setShowDeploy] = useState(false);
  const [nameInput, setNameInput] = useState("");
  const [confirmOpen, setConfirmOpen] = useState(false);
  const logsContainerRef = useRef<HTMLDivElement>(null);
  const [followLogs, setFollowLogs] = useState(true);
  const [wrapLogs, setWrapLogs] = useState(true);
  const [now, setNow] = useState(() => Date.now());

  const fetchArtifact = useCallback(() => {
    if (!id) return;
    getArtifact(id).then(setArtifact).catch(() => {});
  }, [id]);

  const fetchLogs = useCallback(() => {
    if (!id) return;
    getArtifactLogs(id)
      .then((text) => setLogs(typeof text === "string" ? text : String(text)))
      .catch(() => {});
  }, [id]);

  // Initial fetch
  useEffect(() => {
    fetchArtifact();
    fetchLogs();
    listGroups().then(setGroups).catch(() => {});
  }, [fetchArtifact, fetchLogs]);

  function handleExportConfig() {
    if (!artifact) return;
    const payload = payloadFromArtifact(artifact, groups);
    const filename = downloadBuildConfig(payload);
    toast(`Exported ${filename}`, "success");
  }

  // Auto-poll artifact every 3s while building
  useEffect(() => {
    if (!artifact || (artifact.phase !== "Pending" && artifact.phase !== "Building")) return;
    const interval = setInterval(fetchArtifact, 3000);
    return () => clearInterval(interval);
  }, [artifact, fetchArtifact]);

  // Live log streaming via the shared UI WebSocket. We subscribe once per
  // mount and filter incoming messages by build id. The initial snapshot
  // on mount (fetchLogs in the combined useEffect above) covers history,
  // and a one-shot re-fetch on reconnect covers any chunks that flushed
  // while the socket was down.
  const { connected: wsConnected } = useUIWebSocket((msg) => {
    if (msg.type !== "build-log" || !id) return;
    const payload = msg.data as { id?: string; chunk?: string } | undefined;
    if (!payload || payload.id !== id || !payload.chunk) return;
    setLogs((prev) => prev + payload.chunk);
  });

  // Re-sync the full log body once per WebSocket (re)connection, to cover
  // any chunks that flushed while the socket was down. We intentionally
  // do NOT depend on `artifact` here — otherwise every 3s artifact poll
  // would retrigger this effect and turn it into a log poll. We read the
  // current artifact via a ref instead.
  const artifactRef = useRef<Artifact | null>(null);
  artifactRef.current = artifact;
  const lastWsConnected = useRef(false);
  useEffect(() => {
    const justConnected = wsConnected && !lastWsConnected.current;
    lastWsConnected.current = wsConnected;
    if (!justConnected) return;
    const a = artifactRef.current;
    if (!a || (a.phase !== "Pending" && a.phase !== "Building")) return;
    fetchLogs();
  }, [wsConnected, fetchLogs]);

  // When the build transitions to a terminal phase, do one last HTTP
  // fetch to pick up any trailing chunks that raced the final flush.
  const lastPhaseRef = useRef<string | undefined>(undefined);
  useEffect(() => {
    const phase = artifact?.phase;
    const prev = lastPhaseRef.current;
    lastPhaseRef.current = phase;
    if (!phase || prev === phase) return;
    if (phase === "Ready" || phase === "Error") {
      fetchLogs();
    }
  }, [artifact?.phase, fetchLogs]);

  // Auto-scroll the log container (not the page) to the bottom when new
  // content arrives and follow-tail is enabled. scrollIntoView() would
  // scroll every ancestor scroll container including <html>, yanking the
  // whole page down past the header every few seconds during a live build.
  useEffect(() => {
    if (!followLogs) return;
    const el = logsContainerRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
  }, [logs, followLogs]);

  // If the user scrolls the log pane up by hand, stop auto-following so the
  // view doesn't yank them back to the bottom mid-read. Re-enable when they
  // scroll back near the bottom.
  useEffect(() => {
    const el = logsContainerRef.current;
    if (!el) return;
    const onScroll = () => {
      const distanceFromBottom = el.scrollHeight - el.scrollTop - el.clientHeight;
      if (distanceFromBottom > 40) {
        setFollowLogs(false);
      } else if (distanceFromBottom < 10) {
        setFollowLogs(true);
      }
    };
    el.addEventListener("scroll", onScroll, { passive: true });
    return () => el.removeEventListener("scroll", onScroll);
  }, []);

  // Tick every second during active builds so the live duration indicator
  // actually moves forward in real time.
  useEffect(() => {
    if (!artifact || (artifact.phase !== "Pending" && artifact.phase !== "Building")) return;
    const interval = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(interval);
  }, [artifact]);

  async function handleCancel() {
    if (!id) return;
    setCancelling(true);
    try {
      await cancelArtifact(id);
      fetchArtifact();
    } finally {
      setCancelling(false);
    }
  }

  async function handleDelete() {
    if (!id) return;
    setDeleting(true);
    try {
      await deleteArtifact(id);
      navigate("/artifacts");
    } finally {
      setDeleting(false);
    }
  }

  // Log statistics for the header toolbar. Memoize so we don't recompute on
  // unrelated state changes during a build (the log string can be large).
  // Declared before the early-return so React's hook order stays stable
  // between the "loading" and "loaded" renders.
  const { logLineCount, logLines } = useMemo(() => {
    if (!logs) return { logLineCount: 0, logLines: [] as string[] };
    const lines = logs.split("\n");
    return { logLineCount: lines.length, logLines: lines };
  }, [logs]);

  if (!artifact) {
    return (
      <div className="flex items-center justify-center py-16 text-muted-foreground">
        Loading...
      </div>
    );
  }

  const isActive = artifact.phase === "Pending" || artifact.phase === "Building";
  const artifactFiles = artifact.artifacts || [];

  function extractFilename(path: string): string {
    const parts = path.split("/");
    return parts[parts.length - 1] || path;
  }

  // Identity line: everything you'd want at a glance — what's being built,
  // from what base, on which hardware target. Empty pieces are simply
  // skipped so dockerfile-mode builds don't render a dangling "null ·".
  const identityParts = [
    artifact.baseImage,
    artifact.kairosVersion,
    [artifact.arch || "amd64", artifact.model || "generic", artifact.variant || "core"]
      .filter(Boolean)
      .join("/"),
  ].filter(Boolean);

  function copyId() {
    navigator.clipboard.writeText(artifact!.id).then(
      () => toast("Copied build ID", "success"),
      () => toast("Copy failed", "error"),
    );
  }

  // Duration string: live-updating while building, frozen once terminal.
  const createdMs = artifact.createdAt ? new Date(artifact.createdAt).getTime() : 0;
  const updatedMs = artifact.updatedAt ? new Date(artifact.updatedAt).getTime() : 0;
  const endMs = isActive ? now : updatedMs;
  const durationSec = createdMs > 0 && endMs > createdMs ? Math.floor((endMs - createdMs) / 1000) : 0;
  const durationText = formatDuration(durationSec);

  function handleCopyLogs() {
    if (!logs) return;
    navigator.clipboard.writeText(logs).then(
      () => toast("Copied build logs", "success"),
      () => toast("Copy failed", "error"),
    );
  }

  function handleDownloadLogs() {
    if (!logs) return;
    const blob = new Blob([logs], { type: "text/plain" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    const slug = (artifact!.name || artifact!.id.slice(0, 8)).trim().toLowerCase().replace(/[^a-z0-9-]+/g, "-");
    a.download = `${slug}.log`;
    a.click();
    URL.revokeObjectURL(url);
  }

  function scrollLogsToBottom() {
    setFollowLogs(true);
    requestAnimationFrame(() => {
      const el = logsContainerRef.current;
      if (el) el.scrollTop = el.scrollHeight;
    });
  }

  // Categorized output badges — same grouping as the Builder's Output step
  // so users see a consistent story (install media / disk images / archive).
  const outputCategories = [
    {
      title: "Install media",
      tone: "orange" as const,
      items: [
        { on: artifact.iso, label: "ISO", icon: Disc3 },
        { on: artifact.netboot, label: "Netboot", icon: Wifi },
        { on: artifact.uki, label: "UKI", icon: ShieldCheck },
      ],
    },
    {
      title: "Disk images",
      tone: "blue" as const,
      items: [
        { on: artifact.rawDisk, label: "Raw disk", icon: HardDrive },
        { on: artifact.cloudImage, label: "Cloud image", icon: Cloud },
        { on: artifact.gce, label: "Google Cloud", icon: CloudCog },
        { on: artifact.vhd, label: "Azure (VHD)", icon: CloudCog },
      ],
    },
    {
      title: "Archives",
      tone: "neutral" as const,
      items: [{ on: artifact.tar, label: "TAR", icon: Package }],
    },
  ];
  const hasSecurityFlags = artifact.fips || artifact.trustedBoot;
  const targetGroupName = artifact.targetGroupId
    ? groups.find((g) => g.id === artifact.targetGroupId)?.name
    : undefined;

  return (
    <div className="space-y-6">
      {/* Breadcrumb */}
      <div className="flex items-center gap-2 text-sm text-muted-foreground mb-4">
        <button onClick={() => navigate("/artifacts")} className="hover:text-foreground">Artifacts</button>
        <span>/</span>
        <span className="text-foreground">{artifact.name || artifact.id.slice(0, 8)}</span>
      </div>

      {/* Header */}
      <div className="flex items-start gap-4">
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            {editingName ? (
              <form
                className="flex items-center gap-2"
                onSubmit={async (e) => {
                  e.preventDefault();
                  await updateArtifact(id!, { name: nameInput });
                  setEditingName(false);
                  fetchArtifact();
                }}
              >
                <Input
                  value={nameInput}
                  onChange={(e) => setNameInput(e.target.value)}
                  className="h-9 text-2xl font-bold w-80"
                  autoFocus
                  placeholder="Artifact name..."
                />
                <Button type="submit" variant="ghost" size="icon" className="h-7 w-7">
                  <Check className="h-4 w-4" />
                </Button>
              </form>
            ) : (
              <>
                <h1 className="text-2xl font-bold truncate leading-tight">
                  {artifact.name || (
                    <span className="font-mono">{artifact.id.slice(0, 12)}</span>
                  )}
                </h1>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-7 w-7 shrink-0 text-muted-foreground hover:text-foreground"
                  title="Rename"
                  onClick={() => {
                    setNameInput(artifact.name || "");
                    setEditingName(true);
                  }}
                >
                  <Pencil className="h-3.5 w-3.5" />
                </Button>
              </>
            )}
          </div>
          {/* Identity subtitle — at-a-glance context for what's being built. */}
          {identityParts.length > 0 && (
            <p className="text-sm text-muted-foreground mt-1 flex items-center gap-2 flex-wrap">
              {identityParts.map((part, i) => (
                <span key={i} className="inline-flex items-center gap-1">
                  {i > 0 && <span className="text-muted-foreground/50">·</span>}
                  <span className="font-mono text-xs bg-muted/60 px-1.5 py-0.5 rounded">{part}</span>
                </span>
              ))}
            </p>
          )}
          {/* Build id + copy button, always present but unobtrusive. */}
          <div className="flex items-center gap-1.5 mt-1.5">
            <span className="text-[11px] text-muted-foreground font-mono">{artifact.id}</span>
            <button
              type="button"
              onClick={copyId}
              className="text-muted-foreground/60 hover:text-foreground transition-colors"
              title="Copy build ID"
            >
              <Copy className="h-3 w-3" />
            </button>
          </div>
        </div>
        <Button
          variant={artifact.saved ? "default" : "outline"}
          size="sm"
          className={artifact.saved ? "bg-[#EE5007] hover:bg-[#FF7442] text-white" : ""}
          onClick={async () => {
            await updateArtifact(id!, { saved: !artifact.saved });
            fetchArtifact();
          }}
        >
          <Bookmark className={`h-4 w-4 mr-2 ${artifact.saved ? "fill-current" : ""}`} />
          {artifact.saved ? "Saved" : "Save"}
        </Button>
        <Button
          variant="outline"
          size="sm"
          onClick={() => navigate(`/artifacts/new?clone=${artifact.id}`)}
        >
          <Copy className="h-4 w-4 mr-2" />
          Clone Build
        </Button>
        <Button variant="outline" size="sm" onClick={handleExportConfig}>
          <FileDown className="h-4 w-4 mr-2" />
          Export Config
        </Button>
        {!isActive && artifact.phase === "Ready" && (
          <Button
            size="sm"
            className="bg-[#EE5007] hover:bg-[#FF7442] text-white"
            onClick={() => setShowDeploy(true)}
          >
            <Rocket className="h-4 w-4 mr-2" /> Deploy
          </Button>
        )}
        {isActive && (
          <Button
            variant="destructive"
            size="sm"
            onClick={handleCancel}
            disabled={cancelling}
          >
            <XCircle className="h-4 w-4 mr-2" />
            {cancelling ? "Cancelling..." : "Cancel Build"}
          </Button>
        )}
        {!isActive && (
          <Button
            variant="destructive"
            size="sm"
            onClick={() => setConfirmOpen(true)}
            disabled={deleting}
          >
            <Trash2 className="h-4 w-4 mr-2" />
            {deleting ? "Deleting..." : "Delete"}
          </Button>
        )}
      </div>

      {/* Status band */}
      <div className="flex items-center gap-4 flex-wrap">
        <StatusBadge status={artifact.phase} />
        {isActive && (
          <span className="inline-flex items-center gap-1.5 text-sm text-[#EE5007]">
            <span className="relative flex h-2 w-2">
              <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-[#EE5007] opacity-75" />
              <span className="relative inline-flex rounded-full h-2 w-2 bg-[#EE5007]" />
            </span>
            Building · {durationText}
          </span>
        )}
        {!isActive && durationSec > 0 && (
          <span className="inline-flex items-center gap-1.5 text-sm text-muted-foreground">
            <Clock className="h-3.5 w-3.5" />
            {artifact.phase === "Ready" ? "Built in" : "Ran for"} {durationText}
          </span>
        )}
        <span className="text-sm text-muted-foreground">
          Created {artifact.createdAt ? new Date(artifact.createdAt).toLocaleString() : "-"}
        </span>
      </div>

      {/* Error hero: promoted when a build failed, with the failure reason
          and a one-click "Clone & retry" path so the user isn't left
          staring at a log trace without a next step. */}
      {artifact.phase === "Error" && artifact.message && (
        <div className="rounded-xl border border-red-500/30 bg-red-500/5 p-5 animate-fade-up">
          <div className="flex items-start gap-4">
            <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-red-500/15 text-red-600 dark:text-red-400">
              <AlertTriangle className="h-5 w-5" />
            </div>
            <div className="flex-1 min-w-0">
              <h3 className="font-semibold text-sm text-red-900 dark:text-red-300">
                Build failed
              </h3>
              <p className="mt-1 font-mono text-xs text-red-800/90 dark:text-red-300/90 break-words">
                {artifact.message}
              </p>
              <div className="mt-4 flex flex-wrap gap-2">
                <Button
                  size="sm"
                  className="bg-[#EE5007] hover:bg-[#FF7442] text-white"
                  onClick={() => navigate(`/artifacts/new?clone=${artifact.id}`)}
                >
                  <Copy className="h-4 w-4 mr-2" />
                  Clone &amp; retry
                </Button>
                <a
                  href="#build-logs"
                  className="inline-flex items-center text-sm text-red-700 dark:text-red-400 hover:underline"
                  onClick={(e) => {
                    e.preventDefault();
                    logsContainerRef.current?.scrollIntoView({ behavior: "smooth", block: "start" });
                  }}
                >
                  Jump to logs
                </a>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Success hero: when a build is Ready, elevate the downloadable
          outputs to a prominent card above the configuration. Users come
          here specifically to grab the files; it deserves the top slot. */}
      {artifact.phase === "Ready" && artifactFiles.length > 0 && (
        <Card className="border-emerald-500/30 overflow-hidden animate-fade-up">
          {/* High-contrast header: solid emerald-50 / emerald-950 pair
              (and emerald-950/80 / emerald-100 for dark mode) so the
              title and subtitle are actually legible against the fill. */}
          <div className="flex items-center justify-between gap-3 border-b border-emerald-500/30 bg-emerald-50 dark:bg-emerald-950/60 px-5 py-3">
            <div className="flex items-center gap-2.5">
              <div className="flex h-8 w-8 items-center justify-center rounded-full bg-emerald-600 text-white">
                <CheckCircle2 className="h-4 w-4" />
              </div>
              <div>
                <h3 className="font-semibold text-sm text-emerald-950 dark:text-emerald-100">
                  Build succeeded
                </h3>
                <p className="text-[11px] text-emerald-900/80 dark:text-emerald-200/80">
                  {artifactFiles.length} file{artifactFiles.length === 1 ? "" : "s"} ready to download
                </p>
              </div>
            </div>
            <Button
              size="sm"
              className="bg-[#EE5007] hover:bg-[#FF7442] text-white"
              onClick={() => setShowDeploy(true)}
            >
              <Rocket className="h-4 w-4 mr-2" />
              Deploy
            </Button>
          </div>
          <CardContent className="p-0">
            <ul className="divide-y">
              {artifactFiles.map((filePath) => {
                const filename = extractFilename(filePath);
                return (
                  <li key={filePath}>
                    <a
                      href={artifactDownloadUrl(id!, filename)}
                      download
                      className="flex items-center gap-3 px-5 py-3 transition-colors hover:bg-muted/40 focus:outline-none focus-visible:bg-muted/60"
                    >
                      <Download className="h-4 w-4 text-emerald-600 dark:text-emerald-400 shrink-0" />
                      <span className="flex-1 font-mono text-xs truncate">{filename}</span>
                      <span className="text-[11px] text-muted-foreground">Click to download</span>
                    </a>
                  </li>
                );
              })}
            </ul>
            {artifact.containerImage && (
              <div className="border-t px-5 py-3 bg-muted/20">
                <p className="text-[11px] uppercase tracking-wide text-muted-foreground mb-1.5">
                  Container image (for upgrades)
                </p>
                <a
                  href={`/api/v1/artifacts/${id}/image?token=${localStorage.getItem("daedalus_token") || ""}`}
                  download
                  className="inline-flex items-center gap-2 text-xs font-mono text-[#EE5007] hover:underline break-all"
                >
                  <Download className="h-3.5 w-3.5" />
                  {artifact.containerImage}
                  <span className="text-muted-foreground">(docker tar)</span>
                </a>
              </div>
            )}
          </CardContent>
        </Card>
      )}

      {/* Configuration — split into Source / Outputs / Provisioning sections
          so readers can scan each concern independently instead of hunting
          through a dense 6-column grid. */}
      <Card>
        <CardContent className="p-6 space-y-6">
          {/* Source */}
          <section>
            <div className="flex items-center gap-2 mb-3">
              <Box className="h-4 w-4 text-muted-foreground" />
              <h3 className="text-sm font-semibold">Source</h3>
            </div>
            <dl className="grid grid-cols-1 md:grid-cols-3 gap-x-6 gap-y-4 text-sm">
              {artifact.dockerfile ? (
                <div className="md:col-span-3">
                  <dt className="text-xs uppercase tracking-wide text-muted-foreground mb-1.5 flex items-center gap-1.5">
                    <FileCode className="h-3 w-3" />
                    Dockerfile
                  </dt>
                  <dd>
                    <pre className="font-mono text-xs bg-muted/40 border rounded-md p-3 overflow-x-auto max-h-40">
                      {artifact.dockerfile}
                    </pre>
                  </dd>
                </div>
              ) : (
                <div className="md:col-span-2">
                  <dt className="text-xs uppercase tracking-wide text-muted-foreground mb-1">Base image</dt>
                  <dd className="font-mono text-xs break-all">{artifact.baseImage || "—"}</dd>
                </div>
              )}
              <div>
                <dt className="text-xs uppercase tracking-wide text-muted-foreground mb-1">Version</dt>
                <dd className="flex items-center gap-1.5">
                  <Tag className="h-3.5 w-3.5 text-muted-foreground" />
                  <span>{artifact.kairosVersion || "—"}</span>
                </dd>
              </div>
              <div>
                <dt className="text-xs uppercase tracking-wide text-muted-foreground mb-1">Architecture</dt>
                <dd className="flex items-center gap-1.5">
                  <Cpu className="h-3.5 w-3.5 text-muted-foreground" />
                  <span>{(artifact.arch || "amd64").toUpperCase()}</span>
                </dd>
              </div>
              <div>
                <dt className="text-xs uppercase tracking-wide text-muted-foreground mb-1">Model</dt>
                <dd className="flex items-center gap-1.5">
                  <Server className="h-3.5 w-3.5 text-muted-foreground" />
                  <span>{artifact.model || "generic"}</span>
                </dd>
              </div>
              <div>
                <dt className="text-xs uppercase tracking-wide text-muted-foreground mb-1">Variant</dt>
                <dd className="flex items-center gap-1.5">
                  <Layers className="h-3.5 w-3.5 text-muted-foreground" />
                  <span className="capitalize">{artifact.variant || "core"}</span>
                  {artifact.variant === "standard" && artifact.kubernetesDistro && (
                    <span className="text-xs text-muted-foreground">
                      · {artifact.kubernetesDistro.toUpperCase()}
                      {artifact.kubernetesVersion ? ` ${artifact.kubernetesVersion}` : ""}
                    </span>
                  )}
                </dd>
              </div>
            </dl>
          </section>

          {/* Outputs */}
          <section className="border-t pt-5">
            <div className="flex items-center gap-2 mb-3">
              <Package className="h-4 w-4 text-muted-foreground" />
              <h3 className="text-sm font-semibold">Outputs</h3>
            </div>
            <div className="space-y-3">
              {outputCategories.map((cat) => {
                const selected = cat.items.filter((i) => i.on);
                if (selected.length === 0) return null;
                return (
                  <div key={cat.title} className="flex items-center gap-3 flex-wrap">
                    <span className="text-xs text-muted-foreground w-28 shrink-0">{cat.title}</span>
                    <div className="flex gap-2 flex-wrap">
                      {selected.map((item) => {
                        const Icon = item.icon;
                        return (
                          <span
                            key={item.label}
                            className={`inline-flex items-center gap-1.5 text-xs font-medium px-2.5 py-1 rounded-md border ${TONE_CLASSES[cat.tone]}`}
                          >
                            <Icon className="h-3.5 w-3.5" />
                            {item.label}
                          </span>
                        );
                      })}
                    </div>
                  </div>
                );
              })}
              {hasSecurityFlags && (
                <div className="flex items-center gap-3 flex-wrap">
                  <span className="text-xs text-muted-foreground w-28 shrink-0">Security</span>
                  <div className="flex gap-2 flex-wrap">
                    {artifact.fips && (
                      <span className="inline-flex items-center gap-1.5 text-xs font-medium px-2.5 py-1 rounded-md border border-purple-500/30 bg-purple-500/10 text-purple-700">
                        <ShieldCheck className="h-3.5 w-3.5" />
                        FIPS
                      </span>
                    )}
                    {artifact.trustedBoot && (
                      <span className="inline-flex items-center gap-1.5 text-xs font-medium px-2.5 py-1 rounded-md border border-purple-500/30 bg-purple-500/10 text-purple-700">
                        <ShieldCheck className="h-3.5 w-3.5" />
                        Trusted Boot
                      </span>
                    )}
                  </div>
                </div>
              )}
            </div>
          </section>

          {/* Provisioning */}
          <section className="border-t pt-5">
            <div className="flex items-center gap-2 mb-3">
              <UserCheck className="h-4 w-4 text-muted-foreground" />
              <h3 className="text-sm font-semibold">Provisioning</h3>
            </div>
            <div className="grid grid-cols-1 md:grid-cols-3 gap-x-6 gap-y-3 text-sm">
              <div className="flex items-center gap-2">
                {artifact.autoInstall ? (
                  <Check className="h-4 w-4 text-emerald-600" />
                ) : (
                  <XCircle className="h-4 w-4 text-amber-600" />
                )}
                <span>{artifact.autoInstall ? "Auto-install on boot" : "Manual install"}</span>
              </div>
              <div className="flex items-center gap-2">
                {artifact.registerDaedalus ? (
                  <Check className="h-4 w-4 text-emerald-600" />
                ) : (
                  <XCircle className="h-4 w-4 text-amber-600" />
                )}
                <span>
                  {artifact.registerDaedalus ? "Auto-register with Daedalus" : "No auto-registration"}
                </span>
              </div>
              {targetGroupName && (
                <div className="flex items-center gap-2">
                  <Tag className="h-4 w-4 text-muted-foreground" />
                  <span>
                    Group: <span className="font-medium">{targetGroupName}</span>
                  </span>
                </div>
              )}
            </div>
          </section>
        </CardContent>
      </Card>

      {/* Build Logs — terminal with a toolbar, follow-tail, wrap toggle,
          copy + download, live indicator, and line numbers. */}
      <Card className="overflow-hidden">
        <div className="flex items-center justify-between gap-3 border-b bg-muted/30 px-4 py-2.5">
          <div className="flex items-center gap-2 min-w-0">
            <TerminalIcon className="h-4 w-4 text-muted-foreground shrink-0" />
            <h3 className="text-sm font-semibold">Build logs</h3>
            {isActive && wsConnected && (
              <span className="inline-flex items-center gap-1.5 text-[11px] text-[#EE5007]">
                <span className="relative flex h-1.5 w-1.5">
                  <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-[#EE5007] opacity-75" />
                  <span className="relative inline-flex rounded-full h-1.5 w-1.5 bg-[#EE5007]" />
                </span>
                Live
              </span>
            )}
            {isActive && !wsConnected && (
              <span
                className="inline-flex items-center gap-1.5 text-[11px] text-amber-700 dark:text-amber-400"
                title="Lost connection to the log stream. Auto-reconnecting…"
              >
                <Loader2 className="h-3 w-3 animate-spin" />
                Reconnecting…
              </span>
            )}
            {logLineCount > 0 && (
              <span className="text-[11px] text-muted-foreground tabular-nums">
                {logLineCount.toLocaleString()} lines
              </span>
            )}
          </div>
          <div className="flex items-center gap-1">
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="h-7 w-7"
              title={wrapLogs ? "Disable wrap" : "Enable wrap"}
              onClick={() => setWrapLogs((v) => !v)}
            >
              <WrapText className={`h-3.5 w-3.5 ${wrapLogs ? "text-[#EE5007]" : "text-muted-foreground"}`} />
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="h-7 w-7"
              title={followLogs ? "Pause auto-scroll" : "Jump to bottom"}
              onClick={() => {
                if (followLogs) {
                  setFollowLogs(false);
                } else {
                  scrollLogsToBottom();
                }
              }}
            >
              <ArrowDown className={`h-3.5 w-3.5 ${followLogs ? "text-[#EE5007]" : "text-muted-foreground"}`} />
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="h-7 w-7"
              title="Copy logs"
              disabled={!logs}
              onClick={handleCopyLogs}
            >
              <Copy className="h-3.5 w-3.5 text-muted-foreground" />
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="h-7 w-7"
              title="Download .log"
              disabled={!logs}
              onClick={handleDownloadLogs}
            >
              <FileDown className="h-3.5 w-3.5 text-muted-foreground" />
            </Button>
          </div>
        </div>
        <div
          ref={logsContainerRef}
          className="terminal-output font-mono text-[12.5px] leading-5 overflow-auto max-h-[32rem] min-h-[20rem]"
        >
          {logLines.length > 0 ? (
            // Line-numbered log rendering. We pre-split the string once and
            // render each line with a sticky gutter column. ansiToHtml
            // runs per-line so the gutter number doesn't inherit SGR state
            // from a preceding line.
            <div className={`flex ${wrapLogs ? "" : "w-max min-w-full"}`}>
              <div className="sticky left-0 select-none text-right pr-3 pl-4 py-3 text-muted-foreground/50 bg-black/20 border-r border-white/5 tabular-nums">
                {logLines.map((_, i) => (
                  <div key={i}>{i + 1}</div>
                ))}
              </div>
              <div className={`py-3 pr-4 pl-3 flex-1 ${wrapLogs ? "whitespace-pre-wrap break-words" : "whitespace-pre"}`}>
                {logLines.map((line, i) => (
                  <div
                    key={i}
                    dangerouslySetInnerHTML={{ __html: ansiToHtml(line) || "&nbsp;" }}
                  />
                ))}
              </div>
            </div>
          ) : (
            <div className="flex items-center justify-center gap-2 py-16 text-muted-foreground/80 text-sm">
              {isActive ? (
                <>
                  <Loader2 className="h-4 w-4 animate-spin" />
                  Waiting for logs…
                </>
              ) : (
                <span>No logs recorded for this build.</span>
              )}
            </div>
          )}
        </div>
      </Card>

      {/* Non-Ready downloads fallback: if a non-terminal build somehow
          has files attached (partial products, aborted mid-stream, etc.)
          still expose them, but in a plain card — the success hero above
          is reserved for Ready builds. */}
      {artifact.phase !== "Ready" && artifactFiles.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Partial outputs</CardTitle>
          </CardHeader>
          <CardContent>
            <ul className="space-y-2">
              {artifactFiles.map((filePath) => {
                const filename = extractFilename(filePath);
                return (
                  <li key={filePath} className="flex items-center gap-3">
                    <Download className="h-4 w-4 text-muted-foreground" />
                    <a
                      href={artifactDownloadUrl(id!, filename)}
                      className="text-sm font-mono hover:underline text-[#EE5007]"
                      download
                    >
                      {filename}
                    </a>
                  </li>
                );
              })}
            </ul>
          </CardContent>
        </Card>
      )}

      {showDeploy && (
        <DeployDialog
          artifactId={id!}
          artifactFiles={artifactFiles}
          hasNetboot={artifact.netboot}
          onClose={() => setShowDeploy(false)}
        />
      )}

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title="Delete Artifact"
        description="Are you sure you want to delete this artifact? This cannot be undone."
        confirmLabel="Delete"
        destructive
        onConfirm={handleDelete}
      />
    </div>
  );
}

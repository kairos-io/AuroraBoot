import { useEffect, useState, useCallback, useRef } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { listArtifacts, deleteArtifact, clearFailedArtifacts, updateArtifact, type Artifact } from "@/api/artifacts";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { StatusBadge } from "@/components/StatusBadge";
import { PageHeader } from "@/components/PageHeader";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Plus, Trash2, Package, Bookmark, Copy, ChevronDown } from "lucide-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Badge } from "@/components/ui/badge";

// Valid values for each URL-driven axis. Any other value falls back to the
// first option so a malformed share link (or a stale schema from a rename)
// still renders a sensible page.
const TAB_VALUES = ["all", "saved"] as const;
const STATUS_VALUES = ["all", "building", "ready", "error"] as const;
type TabValue = (typeof TAB_VALUES)[number];
type StatusValue = (typeof STATUS_VALUES)[number];

const STATUS_LABELS: Record<StatusValue, string> = {
  all: "All statuses",
  building: "Building",
  ready: "Ready",
  error: "Failed",
};

function coerceTab(raw: string | null): TabValue {
  return (TAB_VALUES as readonly string[]).includes(raw ?? "")
    ? (raw as TabValue)
    : "all";
}

function coerceStatus(raw: string | null): StatusValue {
  return (STATUS_VALUES as readonly string[]).includes(raw ?? "")
    ? (raw as StatusValue)
    : "all";
}

export function Artifacts() {
  const [artifacts, setArtifacts] = useState<Artifact[]>([]);
  const [sp, setSp] = useSearchParams();
  const tab = coerceTab(sp.get("tab"));
  const status = coerceStatus(sp.get("status"));
  // Local mirror of the search query so typing stays snappy; the URL gets
  // debounced-updated so the browser history doesn't fill with keystrokes.
  const [searchDraft, setSearchDraft] = useState(sp.get("q") ?? "");
  const search = sp.get("q") ?? "";
  const [confirmState, setConfirmState] = useState<{ open: boolean; action: () => void; title: string; description: string }>({ open: false, action: () => {}, title: "", description: "" });
  const navigate = useNavigate();

  // Keep the search URL param in sync with the draft, but debounced. Using
  // `replace: true` avoids pushing a new history entry on every keystroke.
  const searchTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => {
    if (searchTimer.current) clearTimeout(searchTimer.current);
    searchTimer.current = setTimeout(() => {
      patchParams({ q: searchDraft || undefined });
    }, 250);
    return () => {
      if (searchTimer.current) clearTimeout(searchTimer.current);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [searchDraft]);

  // If the URL's ?q= changes externally (back/forward), sync the draft so
  // the input reflects the browser's idea of the current filter.
  useEffect(() => {
    const urlQ = sp.get("q") ?? "";
    if (urlQ !== searchDraft) setSearchDraft(urlQ);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sp]);

  // patchParams merges into the existing URLSearchParams and drops keys
  // whose value is the default/empty so the URL stays short when no
  // filters are active.
  function patchParams(patch: Record<string, string | undefined>) {
    const next = new URLSearchParams(sp);
    for (const [k, v] of Object.entries(patch)) {
      if (!v || v === "all") next.delete(k);
      else next.set(k, v);
    }
    setSp(next, { replace: true });
  }

  const fetchArtifacts = useCallback(() => {
    listArtifacts().then(setArtifacts).catch(() => {});
  }, []);

  useEffect(() => {
    fetchArtifacts();
  }, [fetchArtifacts]);

  // Auto-poll every 5s while any artifact is Pending or Building
  useEffect(() => {
    const hasActive = artifacts.some(
      (a) => a.phase === "Pending" || a.phase === "Building"
    );
    if (!hasActive) return;

    const interval = setInterval(fetchArtifacts, 5000);
    return () => clearInterval(interval);
  }, [artifacts, fetchArtifacts]);

  const filtered = artifacts
    .filter((a) => (tab === "saved" ? a.saved : true))
    .filter((a) => {
      if (status === "all") return true;
      if (status === "building") return a.phase === "Pending" || a.phase === "Building";
      if (status === "ready") return a.phase === "Ready";
      if (status === "error") return a.phase === "Error";
      return true;
    })
    .filter((a) => {
      if (!search) return true;
      const q = search.toLowerCase();
      return (
        (a.name || "").toLowerCase().includes(q) ||
        a.baseImage.toLowerCase().includes(q)
      );
    });
  const savedCount = artifacts.filter((a) => a.saved).length;
  const hasAnyFilter = tab !== "all" || status !== "all" || !!search;

  return (
    <div>
      <PageHeader title="Artifacts" description="Build and manage OS artifacts">
        {artifacts.some((a) => a.phase === "Error") && (
          <Button
            variant="outline"
            onClick={() => {
              setConfirmState({
                open: true,
                title: "Delete Failed Artifacts",
                description: "Are you sure you want to delete all failed artifacts? This cannot be undone.",
                action: async () => {
                  await clearFailedArtifacts();
                  fetchArtifacts();
                },
              });
            }}
          >
            <Trash2 className="h-4 w-4 mr-2" />
            Clear Failed
          </Button>
        )}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button className="bg-[#EE5007] hover:bg-[#FF7442] text-white">
              <Plus className="h-4 w-4 mr-2" />
              Build New
              <ChevronDown className="h-4 w-4 ml-1" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onClick={() => navigate("/artifacts/new")}>
              Kairos image
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => navigate("/artifacts/new-hadron")}>
              Hadron image
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </PageHeader>

      <div className="flex flex-wrap items-center gap-3 mb-4">
        <Tabs value={tab} onValueChange={(v) => patchParams({ tab: v === "all" ? undefined : v })}>
          <TabsList>
            <TabsTrigger value="all">All Builds</TabsTrigger>
            <TabsTrigger value="saved">
              Saved{savedCount > 0 && ` (${savedCount})`}
            </TabsTrigger>
          </TabsList>
        </Tabs>

        <Select
          value={status}
          onValueChange={(v) => patchParams({ status: v === "all" ? undefined : v })}
        >
          <SelectTrigger className="h-9 w-[11rem]">
            <SelectValue placeholder="All statuses" />
          </SelectTrigger>
          <SelectContent>
            {STATUS_VALUES.map((s) => (
              <SelectItem key={s} value={s}>
                {STATUS_LABELS[s]}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Input
          placeholder="Search artifacts..."
          value={searchDraft}
          onChange={(e) => setSearchDraft(e.target.value)}
          className="max-w-sm"
        />
      </div>

      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="w-10"></TableHead>
            <TableHead>Name</TableHead>
            <TableHead>Kind</TableHead>
            <TableHead>Base Image</TableHead>
            <TableHead>Phase</TableHead>
            <TableHead>Created</TableHead>
            <TableHead>Artifacts</TableHead>
            <TableHead className="w-12"></TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {filtered.length === 0 ? (
            <TableRow>
              <TableCell colSpan={8} className="text-center py-12">
                {hasAnyFilter ? (
                  <div className="flex flex-col items-center gap-3 py-10">
                    <Package className="h-10 w-10 text-muted-foreground/40" />
                    <div className="text-center">
                      <p className="text-sm font-medium text-muted-foreground">
                        {status !== "all"
                          ? `No ${STATUS_LABELS[status].toLowerCase()} builds`
                          : tab === "saved"
                          ? "No saved artifacts match this filter"
                          : "No matching artifacts"}
                      </p>
                      <p className="text-xs text-muted-foreground/70 mt-1">
                        {search
                          ? "Try adjusting your search query."
                          : "Clear the active filter to see all builds."}
                      </p>
                    </div>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => {
                        setSearchDraft("");
                        setSp(new URLSearchParams(), { replace: true });
                      }}
                    >
                      Clear filters
                    </Button>
                  </div>
                ) : (
                  <div className="flex flex-col items-center gap-3 py-16">
                    <Package className="h-12 w-12 text-muted-foreground/30" />
                    <div className="text-center">
                      <p className="font-medium">No artifacts yet</p>
                      <p className="text-sm text-muted-foreground mt-1">
                        Build your first OS image to deploy across your fleet.
                      </p>
                    </div>
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button className="mt-2 bg-[#EE5007] hover:bg-[#FF7442] text-white">
                          <Plus className="h-4 w-4 mr-2" /> Build First Artifact
                          <ChevronDown className="h-4 w-4 ml-1" />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="center">
                        <DropdownMenuItem onClick={() => navigate("/artifacts/new")}>
                          Kairos image
                        </DropdownMenuItem>
                        <DropdownMenuItem onClick={() => navigate("/artifacts/new-hadron")}>
                          Hadron image
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </div>
                )}
              </TableCell>
            </TableRow>
          ) : (
            filtered.map((artifact) => (
              <TableRow
                key={artifact.id}
                className="cursor-pointer hover:bg-[#EE5007]/5"
                onClick={() => navigate(`/artifacts/${artifact.id}`)}
              >
                <TableCell>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-7 w-7"
                    onClick={async (e) => {
                      e.stopPropagation();
                      await updateArtifact(artifact.id, { saved: !artifact.saved });
                      fetchArtifacts();
                    }}
                  >
                    <Bookmark
                      className={`h-4 w-4 ${
                        artifact.saved
                          ? "fill-[#EE5007] text-[#EE5007]"
                          : "text-muted-foreground/40"
                      }`}
                    />
                  </Button>
                </TableCell>
                <TableCell>
                  <div className="min-w-0">
                    <p className="text-sm font-medium truncate max-w-xs">
                      {artifact.name || (
                        <span className="text-muted-foreground font-mono text-xs">
                          {artifact.id.slice(0, 8)}
                        </span>
                      )}
                    </p>
                    {artifact.name && (
                      <p className="text-xs text-muted-foreground font-mono">
                        {artifact.id.slice(0, 8)}
                      </p>
                    )}
                  </div>
                </TableCell>
                <TableCell>
                  {artifact.kind === "hadron" ? (
                    <Badge variant="secondary" className="text-[10px]">hadron</Badge>
                  ) : (
                    <Badge variant="outline" className="text-[10px]">kairos</Badge>
                  )}
                </TableCell>
                <TableCell className="text-sm max-w-xs">
                  {artifact.kind === "hadron" ? (
                    (() => {
                      // Surface the hadron composition inline: base ref plus
                      // small counters for firmware and layers so operators
                      // can spot "this row bundles 3 firmware / 2 layers"
                      // without opening the artifact detail. Falls back to
                      // baseImage when the spec blob is malformed.
                      let base = artifact.baseImage || "";
                      let fw = 0;
                      let ly = 0;
                      let platforms: string[] = [];
                      try {
                        const spec = artifact.hadronSpec ? JSON.parse(artifact.hadronSpec) : {};
                        if (spec.baseImage) base = spec.baseImage;
                        fw = Array.isArray(spec.firmware) ? spec.firmware.length : 0;
                        ly = Array.isArray(spec.layers) ? spec.layers.length : 0;
                        if (Array.isArray(spec.platforms)) platforms = spec.platforms;
                      } catch { /* ignore */ }
                      if (platforms.length === 0) platforms = ["linux/amd64"];
                      return (
                        <div className="flex flex-col gap-1 min-w-0">
                          <span className="truncate">{base || "—"}</span>
                          <div className="flex gap-1 flex-wrap">
                            <Badge variant="outline" className="text-[10px]" title="Firmware layers">
                              {fw} {fw > 1 ? "firmwares" : "firmware"}
                            </Badge>
                            <Badge variant="outline" className="text-[10px]" title="Software layers">
                              {ly} {ly > 1 ? "layers" : "layer"}
                            </Badge>
                            {platforms.map((p) => (
                              <Badge
                                key={p}
                                variant="secondary"
                                className="text-[10px] font-mono"
                                title={p}
                              >
                                {p.replace(/^linux\//, "")}
                              </Badge>
                            ))}
                          </div>
                        </div>
                      );
                    })()
                  ) : (
                    <div className="flex flex-col gap-1 min-w-0">
                      <span className="truncate">{artifact.baseImage || "-"}</span>
                      {artifact.arch && (
                        <div>
                          <Badge variant="secondary" className="text-[10px] font-mono" title="Architecture">
                            {artifact.arch}
                          </Badge>
                        </div>
                      )}
                    </div>
                  )}
                </TableCell>
                <TableCell>
                  <div className="flex flex-col gap-1">
                    <StatusBadge status={artifact.phase} />
                    {artifact.phase === "Error" && artifact.message && (
                      <span className="text-xs text-red-600 truncate max-w-xs">
                        {artifact.message}
                      </span>
                    )}
                  </div>
                </TableCell>
                <TableCell className="text-xs">
                  {artifact.createdAt
                    ? new Date(artifact.createdAt).toLocaleDateString()
                    : "-"}
                </TableCell>
                <TableCell className="text-xs">
                  {(artifact.artifacts || []).length > 0
                    ? `${artifact.artifacts.length} file(s)`
                    : "-"}
                </TableCell>
                <TableCell>
                  <div className="flex gap-1">
                    {/* Clone is kairos-only: it pre-fills the Kairos wizard
                        from an artifact record. Hadron rows carry a different
                        spec shape; forcing them through this button would
                        produce a mostly-empty wizard, so we hide it. */}
                    {artifact.phase === "Ready" && artifact.kind !== "hadron" && (
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-7 w-7"
                        onClick={(e) => {
                          e.stopPropagation();
                          navigate(`/artifacts/new?clone=${artifact.id}`);
                        }}
                      >
                        <Copy className="h-4 w-4" />
                      </Button>
                    )}
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-7 w-7"
                      onClick={(e) => {
                        e.stopPropagation();
                        setConfirmState({
                          open: true,
                          title: "Delete Artifact",
                          description: "Are you sure you want to delete this artifact? This cannot be undone.",
                          action: async () => {
                            await deleteArtifact(artifact.id);
                            fetchArtifacts();
                          },
                        });
                      }}
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            ))
          )}
        </TableBody>
      </Table>

      <ConfirmDialog
        open={confirmState.open}
        onOpenChange={(open) => setConfirmState(prev => ({ ...prev, open }))}
        title={confirmState.title}
        description={confirmState.description}
        confirmLabel="Delete"
        destructive
        onConfirm={confirmState.action}
      />
    </div>
  );
}

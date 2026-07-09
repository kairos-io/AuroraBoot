import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { listExtensions, type Extension } from "@/api/extensions";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/PageHeader";
import { ExtensionTypeChip } from "@/components/ExtensionTypeChip";

const TEMPLATES = ["Tailscale", "Fluent-bit", "Nvidia container toolkit"];

export function Extensions() {
  const navigate = useNavigate();
  const [rows, setRows] = useState<Extension[] | null>(null);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    listExtensions()
      .then(setRows)
      .catch((e) => setErr(String(e)));
  }, []);

  if (err) {
    return <div className="text-red-600 p-4">Failed to load: {err}</div>;
  }
  if (rows === null) {
    return <div className="p-4 text-muted-foreground text-sm">Loading…</div>;
  }
  if (rows.length === 0) {
    return (
      <div>
        <PageHeader
          title="Extensions"
          description="System and config extensions installed on nodes alongside the OS image."
        />
        <div className="text-center py-16">
          <p className="text-xl font-semibold mb-1.5">No extensions yet</p>
          <p className="text-sm text-muted-foreground max-w-md mx-auto mb-5">
            System and config extensions extend a running Kairos node without
            re-imaging — ship a binary, a config drop-in, or a whole agent on
            top of an OS artifact.
          </p>
          <Button onClick={() => navigate("/extensions/new")}>
            Build extension
          </Button>
          <div className="flex gap-2 justify-center mt-4 text-xs text-muted-foreground items-center flex-wrap">
            <span>Or start from a template:</span>
            {TEMPLATES.map((t) => (
              <Link
                key={t}
                to={`/extensions/new?template=${encodeURIComponent(t)}`}
                className="px-2.5 py-0.5 rounded-full border border-dashed"
              >
                {t}
              </Link>
            ))}
          </div>
        </div>
      </div>
    );
  }

  return (
    <div>
      <PageHeader
        title="Extensions"
        description="System and config extensions installed on nodes alongside the OS image."
      >
        <Button onClick={() => navigate("/extensions/new")}>
          Build extension
        </Button>
      </PageHeader>

      <div className="rounded-md border overflow-hidden text-sm">
        <div className="grid grid-cols-[1.6fr_.7fr_.7fr_.7fr_.6fr_1.4fr_.9fr_.7fr] px-3 py-2 bg-muted/50 text-[11px] uppercase tracking-wider text-muted-foreground">
          <div>Name</div>
          <div>Type</div>
          <div>Arch</div>
          <div>Version</div>
          <div>Signed</div>
          <div>Phase</div>
          <div>Updated</div>
          <div className="text-right">Actions</div>
        </div>
        {rows.map((r) => (
          <ExtensionRow key={r.id} ext={r} />
        ))}
      </div>
    </div>
  );
}

function ExtensionRow({ ext }: { ext: Extension }) {
  const navigate = useNavigate();
  const progress =
    ext.phase === "Building" ? parseInt(ext.message, 10) || 50 : null;
  return (
    <div
      className="grid grid-cols-[1.6fr_.7fr_.7fr_.7fr_.6fr_1.4fr_.9fr_.7fr] px-3 py-3 items-center border-t hover:bg-muted/30 cursor-pointer"
      onClick={() => navigate(`/extensions/${ext.id}`)}
    >
      <div>
        <div className="font-medium">{ext.name}</div>
        <div className="text-[11px] text-muted-foreground">
          {sourceLabel(ext)}
        </div>
      </div>
      <div>
        <ExtensionTypeChip type={ext.type} />
      </div>
      <div>
        <code className="text-[11px]">{ext.arch}</code>
      </div>
      <div>
        <code className="text-[11px]">{ext.version}</code>
      </div>
      <div>
        {ext.signingKeySetId ? (
          <span className="text-emerald-600">✓</span>
        ) : (
          <span className="opacity-40">—</span>
        )}
      </div>
      <div>
        {ext.phase === "Building" && progress !== null ? (
          <div className="flex items-center gap-2">
            <span className="text-[11px] px-2 py-0.5 rounded-full bg-amber-500/15 text-amber-700">
              Building
            </span>
            <div
              role="progressbar"
              aria-valuenow={progress}
              className="h-1 flex-1 bg-muted rounded"
            >
              <div
                className="h-1 bg-amber-500 rounded"
                style={{ width: `${progress}%` }}
              />
            </div>
            <span className="text-[11px] text-muted-foreground">
              {progress}%
            </span>
          </div>
        ) : ext.phase === "Error" ? (
          <span>
            <span className="text-[11px] px-2 py-0.5 rounded-full bg-red-500/15 text-red-700 mr-2">
              Error
            </span>
            <span className="text-[11px] text-red-700/80">
              {ext.message?.split("\n")[0]}
            </span>
          </span>
        ) : (
          <PhasePill phase={ext.phase} />
        )}
      </div>
      <div className="text-[11px] text-muted-foreground">
        {relTime(ext.updatedAt)}
      </div>
      <div className="text-right text-[11px] text-muted-foreground">
        View · ⋯
      </div>
    </div>
  );
}

function PhasePill({ phase }: { phase: string }) {
  const cls =
    phase === "Ready"
      ? "bg-emerald-500/15 text-emerald-700"
      : "bg-muted text-foreground/70";
  return (
    <span className={`text-[11px] px-2 py-0.5 rounded-full ${cls}`}>
      {phase}
    </span>
  );
}

function sourceLabel(ext: Extension): string {
  switch (ext.sourceMode) {
    case "artifact":
      return `From artifact ${ext.sourceArtifactId ?? ""}${ext.extraSteps ? " + steps" : ""}`;
    case "dockerfile":
      return "From Dockerfile";
    case "image":
      return `From ${ext.sourceImage}`;
    default:
      return "";
  }
}

function relTime(iso?: string): string {
  if (!iso) return "";
  const ms = Date.now() - new Date(iso).getTime();
  if (ms < 60_000) return "just now";
  if (ms < 3_600_000) return `${Math.floor(ms / 60_000)} min ago`;
  if (ms < 86_400_000) return `${Math.floor(ms / 3_600_000)} h ago`;
  return new Date(iso).toLocaleDateString();
}

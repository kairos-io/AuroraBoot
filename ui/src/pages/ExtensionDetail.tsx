import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import {
  getExtension,
  getExtensionLogs,
  deleteExtension,
  cancelExtension,
  extensionDownloadUrl,
  type Extension,
} from "@/api/extensions";
import { getArtifact } from "@/api/artifacts";
import { ApiError } from "@/api/client";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { PageHeader } from "@/components/PageHeader";
import { ExtensionTypeChip } from "@/components/ExtensionTypeChip";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { InstallExtensionDialog } from "@/components/InstallExtensionDialog";

export function ExtensionDetail() {
  const { id = "" } = useParams();
  const navigate = useNavigate();
  const [ext, setExt] = useState<Extension | null>(null);
  const [logs, setLogs] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [installOpen, setInstallOpen] = useState(false);
  // When delete is blocked by a 409, we look up the names of the referencing
  // artifacts and surface a proper banner instead of the raw error string.
  const [deleteBlocker, setDeleteBlocker] = useState<
    { artifacts: { id: string; name: string }[] } | null
  >(null);

  useEffect(() => {
    if (!id) return;
    let cancelled = false;
    async function load() {
      try {
        const [e, l] = await Promise.all([
          getExtension(id),
          getExtensionLogs(id).catch(() => ""),
        ]);
        if (!cancelled) {
          setExt(e);
          setLogs(l);
        }
      } catch (e) {
        if (!cancelled) setErr(String(e));
      }
    }
    load();
    // Poll while not in a terminal phase.
    const t = setInterval(() => {
      if (cancelled) return;
      if (ext && (ext.phase === "Ready" || ext.phase === "Error")) return;
      load();
    }, 3000);
    return () => {
      cancelled = true;
      clearInterval(t);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

  async function onDelete() {
    try {
      await deleteExtension(id);
      navigate("/extensions");
    } catch (e) {
      // Server returns 409 with {error, artifacts:[artifactId,...]} when the
      // extension is still bundled. Look the names up so we can surface a
      // friendly message + links, instead of dumping the raw JSON on the user.
      if (
        e instanceof ApiError &&
        e.status === 409 &&
        e.body &&
        typeof e.body === "object" &&
        Array.isArray((e.body as any).artifacts)
      ) {
        const ids = (e.body as any).artifacts as string[];
        const named = await Promise.all(
          ids.map(async (aid) => {
            try {
              const a = await getArtifact(aid);
              return { id: aid, name: a.name || aid };
            } catch {
              return { id: aid, name: aid };
            }
          }),
        );
        setDeleteBlocker({ artifacts: named });
        return;
      }
      setErr(String(e));
    }
  }

  async function onCancel() {
    try {
      await cancelExtension(id);
    } catch (e) {
      setErr(String(e));
    }
  }

  if (err) {
    return <div className="text-red-600 p-4">Failed: {err}</div>;
  }
  if (!ext) {
    return <div className="p-4 text-muted-foreground text-sm">Loading…</div>;
  }

  return (
    <div>
      <PageHeader title={ext.name} description={ext.message || `Extension ${ext.id}`}>
        <div className="flex gap-2 items-center">
          <ExtensionTypeChip type={ext.type} />
          <PhasePill phase={ext.phase} />
          {ext.phase === "Building" && (
            <Button variant="outline" size="sm" onClick={onCancel}>
              Cancel
            </Button>
          )}
          {ext.phase === "Ready" && ext.rawFilename && (
            <a
              href={extensionDownloadUrl(ext.id, ext.rawFilename)}
              className="text-sm underline"
              download
            >
              Download .raw
            </a>
          )}
          <Button
            disabled={ext.phase !== "Ready"}
            onClick={() => setInstallOpen(true)}
          >
            Install on group…
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setConfirmDelete(true)}
          >
            Delete
          </Button>
        </div>
      </PageHeader>

      <div className="grid md:grid-cols-2 gap-4 mb-6">
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Details</CardTitle>
          </CardHeader>
          <CardContent className="grid gap-1.5 text-sm">
            <KV k="Type" v={ext.type} />
            <KV k="Arch" v={ext.arch} />
            <KV k="Version" v={ext.version} />
            <KV k="Source mode" v={ext.sourceMode} />
            {ext.sourceImage && <KV k="Source image" v={ext.sourceImage} />}
            {ext.sourceArtifactId && (
              <KV k="Source artifact" v={ext.sourceArtifactId} />
            )}
            {ext.containerImage && (
              <KV k="Container image" v={ext.containerImage} />
            )}
            {ext.rawFilename && <KV k="Raw filename" v={ext.rawFilename} />}
            {ext.hierarchies && ext.hierarchies.length > 0 && (
              <KV k="Hierarchies" v={ext.hierarchies.join(", ")} />
            )}
            {ext.serviceReload && <KV k="Service reload" v="yes" />}
            {ext.signingKeySetId && (
              <KV k="Signing key set" v={ext.signingKeySetId} />
            )}
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-sm">Build logs</CardTitle>
        </CardHeader>
        <CardContent>
          <pre className="text-xs font-mono bg-muted/40 rounded p-3 max-h-[40vh] overflow-auto whitespace-pre-wrap">
            {logs || "(no logs yet)"}
          </pre>
        </CardContent>
      </Card>

      <ConfirmDialog
        open={confirmDelete}
        onOpenChange={setConfirmDelete}
        title="Delete extension"
        description={`Delete ${ext.name}? Artifacts that bundle this extension by name will block the deletion.`}
        onConfirm={onDelete}
      />

      <DeleteBlockedDialog
        open={deleteBlocker !== null}
        onOpenChange={(o) => !o && setDeleteBlocker(null)}
        extensionName={ext.name}
        artifacts={deleteBlocker?.artifacts ?? []}
      />

      <InstallExtensionDialog
        open={installOpen}
        onOpenChange={setInstallOpen}
        extension={ext}
      />
    </div>
  );
}

function DeleteBlockedDialog({
  open,
  onOpenChange,
  extensionName,
  artifacts,
}: {
  open: boolean;
  onOpenChange: (next: boolean) => void;
  extensionName: string;
  artifacts: { id: string; name: string }[];
}) {
  if (!open) return null;
  return (
    <div
      role="dialog"
      aria-modal="true"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
      onClick={() => onOpenChange(false)}
    >
      <div
        className="bg-background rounded-lg shadow-xl border max-w-[480px] w-[92%] p-5"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="text-base font-semibold flex items-center gap-2">
          <span className="inline-block h-2 w-2 rounded-full bg-amber-500" />
          Can&apos;t delete <code className="text-sm font-mono">{extensionName}</code> yet
        </h2>
        <p className="text-sm text-muted-foreground mt-2">
          It&apos;s currently bundled into
          {artifacts.length === 1 ? " this OS artifact" : ` these ${artifacts.length} OS artifacts`}.
          Detach it from each, then try deleting again.
        </p>
        <ul className="mt-3 border rounded-md divide-y max-h-56 overflow-y-auto">
          {artifacts.map((a) => (
            <li key={a.id} className="px-3 py-2 text-sm flex items-center justify-between gap-3">
              <span className="truncate font-medium">{a.name}</span>
              <Link
                to={`/artifacts/${a.id}`}
                className="text-xs text-[#EE5007] hover:underline shrink-0"
              >
                Open →
              </Link>
            </li>
          ))}
        </ul>
        <div className="mt-4 flex justify-end">
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Got it
          </Button>
        </div>
      </div>
    </div>
  );
}

function PhasePill({ phase }: { phase: string }) {
  const cls =
    phase === "Ready"
      ? "bg-emerald-500/15 text-emerald-700"
      : phase === "Building"
      ? "bg-amber-500/15 text-amber-700"
      : phase === "Error"
      ? "bg-red-500/15 text-red-700"
      : "bg-muted text-foreground/70";
  return (
    <span className={`text-[11px] px-2 py-0.5 rounded-full ${cls}`}>
      {phase}
    </span>
  );
}

function KV({ k, v }: { k: string; v: string }) {
  return (
    <div className="grid grid-cols-[140px_1fr] gap-2 text-sm">
      <span className="text-muted-foreground">{k}</span>
      <span className="font-mono break-all">{v}</span>
    </div>
  );
}

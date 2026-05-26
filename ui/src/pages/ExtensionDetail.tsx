import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import {
  getExtension,
  getExtensionLogs,
  deleteExtension,
  cancelExtension,
  extensionDownloadUrl,
  type Extension,
} from "@/api/extensions";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { PageHeader } from "@/components/PageHeader";
import { ExtensionTypeChip } from "@/components/ExtensionTypeChip";
import { ConfirmDialog } from "@/components/ConfirmDialog";

export function ExtensionDetail() {
  const { id = "" } = useParams();
  const navigate = useNavigate();
  const [ext, setExt] = useState<Extension | null>(null);
  const [logs, setLogs] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const [confirmDelete, setConfirmDelete] = useState(false);

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

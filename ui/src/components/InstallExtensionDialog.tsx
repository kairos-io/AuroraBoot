import { useEffect, useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { listGroups, type Group } from "@/api/groups";
import { sendBulkCommand } from "@/api/nodes";
import { extensionDownloadUrl, type Extension } from "@/api/extensions";
import { ExtensionTypeChip } from "@/components/ExtensionTypeChip";

type Action = "install" | "enable" | "disable" | "remove";
type Scope = "active" | "passive" | "recovery" | "common";

interface Props {
  open: boolean;
  onOpenChange: (next: boolean) => void;
  extension: Extension;
}

export function InstallExtensionDialog({
  open,
  onOpenChange,
  extension,
}: Props) {
  const [groups, setGroups] = useState<Group[]>([]);
  const [groupID, setGroupID] = useState<string>("");
  const [action, setAction] = useState<Action>("install");
  const [bootState, setBootState] = useState<Scope>("common");
  const [now, setNow] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    listGroups()
      .then((gs) => {
        setGroups(gs);
        if (gs.length === 1) setGroupID(gs[0].id);
      })
      .catch(() => {});
  }, [open]);

  const args: Record<string, string> = {
    type: extension.type,
    action,
    name: extension.name,
    bootState,
    now: now ? "true" : "false",
  };
  if (action === "install" && extension.rawFilename) {
    args.source =
      window.location.origin +
      extensionDownloadUrl(extension.id, extension.rawFilename);
  }

  const canSend = groupID !== "" && !submitting;

  async function handleSend() {
    setSubmitting(true);
    setErr(null);
    try {
      await sendBulkCommand({ groupID }, "extension", args);
      onOpenChange(false);
    } catch (e) {
      setErr(String(e));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-[720px]">
        <DialogHeader>
          <DialogTitle>
            <span className="inline-flex items-center gap-2.5">
              <ExtensionTypeChip type={extension.type} />
              <span className="font-semibold">{extension.name}</span>
              <code className="text-xs opacity-60">
                {extension.version} · {extension.arch}
              </code>
            </span>
          </DialogTitle>
        </DialogHeader>

        <p className="text-sm text-muted-foreground">
          Re-running over the same name = upgrade.
        </p>

        <Section label="Target">
          <Tab active label="Group" />
          <select
            className="border rounded-md px-3 py-2 text-sm bg-background w-full mt-2"
            value={groupID}
            onChange={(e) => setGroupID(e.target.value)}
          >
            <option value="" disabled>
              Select a group…
            </option>
            {groups.map((g) => (
              <option key={g.id} value={g.id}>
                {g.name}
              </option>
            ))}
          </select>
        </Section>

        <Section label="Action">
          <div className="grid grid-cols-4 gap-1.5">
            {(["install", "enable", "disable", "remove"] as const).map((a) => (
              <button
                key={a}
                type="button"
                aria-pressed={action === a}
                onClick={() => setAction(a)}
                className={`px-2.5 py-2 border rounded-md text-xs ${
                  action === a
                    ? "border-[#EE5007] bg-[#EE5007]/5 ring-1 ring-[#EE5007]"
                    : "hover:bg-muted/30"
                }`}
              >
                {a[0].toUpperCase() + a.slice(1)}
              </button>
            ))}
          </div>
        </Section>

        <div className="grid grid-cols-2 gap-4">
          <Section label="Boot scope">
            <div className="flex gap-1.5 flex-wrap">
              {(["active", "passive", "recovery", "common"] as const).map(
                (s) => (
                  <button
                    key={s}
                    type="button"
                    aria-pressed={bootState === s}
                    onClick={() => setBootState(s)}
                    className={`px-2.5 py-1 text-xs rounded-md border ${
                      bootState === s
                        ? "bg-[#EE5007] text-white border-[#EE5007]"
                        : "hover:bg-muted/30"
                    }`}
                  >
                    {s[0].toUpperCase() + s.slice(1)}
                  </button>
                ),
              )}
            </div>
            {bootState === "active" && (
              <p
                role="alert"
                className="text-xs mt-2 px-2.5 py-1.5 rounded-md border border-amber-500/40 bg-amber-500/10 text-amber-800 dark:text-amber-200"
              >
                This extension is only enabled when the node is booted in the
                active partition. If the node is rolled back to passive, it
                won&apos;t be loaded.
              </p>
            )}
          </Section>

          <Section label="When to apply">
            <label className="flex items-start gap-2 text-sm">
              <input
                type="checkbox"
                className="mt-0.5"
                checked={now}
                onChange={(e) => setNow(e.target.checked)}
              />
              <span>
                <span className="font-medium">Activate immediately</span>
                <span className="block text-xs text-muted-foreground">
                  Otherwise applies on next reboot.
                </span>
              </span>
            </label>
          </Section>
        </div>

        <details className="mt-2">
          <summary className="text-xs text-muted-foreground cursor-pointer">
            Show payload
          </summary>
          <pre className="text-[11px] mt-1.5 p-2 rounded-md bg-muted/40 whitespace-pre-wrap">
            {JSON.stringify({ command: "extension", args }, null, 2)}
          </pre>
        </details>

        {err && (
          <p role="alert" className="text-sm text-red-600 mt-2">
            {err}
          </p>
        )}

        <div className="flex justify-end gap-2 mt-4">
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button disabled={!canSend} onClick={handleSend}>
            {submitting ? "Sending…" : "Send to group"}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}

function Section({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="mb-3">
      <div className="text-xs text-muted-foreground mb-1.5">{label}</div>
      {children}
    </div>
  );
}

function Tab({ active, label }: { active: boolean; label: string }) {
  return (
    <button
      type="button"
      aria-pressed={active}
      className={`px-2.5 py-1 text-xs rounded-md ${
        active ? "bg-[#EE5007] text-white" : "border hover:bg-muted/30"
      }`}
      disabled={!active}
    >
      {label}
    </button>
  );
}

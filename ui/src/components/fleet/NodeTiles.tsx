import { useNavigate } from "react-router";
import type { Node } from "@/api/nodes";
import { timeAgo } from "@/lib/time";
import { isOnline, phaseTone } from "@/lib/phase";
import { cpuCount, hasBootIssue, imageVersion, memGiB, nodeAddress } from "@/lib/nodeInfo";
import { phaseCounts } from "@/lib/nodeFilter";
import { cn } from "@/lib/utils";
import { StackBar } from "./StackBar";
import { toneBg } from "./tones";

export interface NodeTileGroup {
  key: string;
  label: string;
  nodes: Node[];
}

interface NodeTilesProps {
  // Ungrouped views pass a single group; its header is then not shown.
  groups: NodeTileGroup[];
  colorBy?: "status";
}

function hardwareLine(node: Node): string {
  const cpu = cpuCount(node);
  const mem = memGiB(node);
  const parts = [cpu !== null ? `${cpu} vCPU` : null, mem].filter(Boolean);
  return parts.join(" · ");
}

function Tile({ node }: { node: Node }) {
  const navigate = useNavigate();
  const online = isOnline(node.phase);
  const image = imageVersion(node);
  const recovery = hasBootIssue(node) ? (node.bootState ?? "").toLowerCase() : "";
  const hw = hardwareLine(node);
  const heartbeat = node.lastHeartbeat ? timeAgo(node.lastHeartbeat) : "";
  return (
    <button
      type="button"
      data-testid="node-tile"
      onClick={() => navigate(`/nodes/${node.id}`)}
      className={cn(
        "relative flex min-w-0 flex-col gap-1 overflow-hidden rounded-md border border-border bg-card p-3 pt-4 text-left text-card-foreground transition-colors hover:border-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
        !online && "opacity-60",
      )}
    >
      <span aria-hidden="true" className={cn("absolute inset-x-0 top-0 h-1", toneBg[phaseTone(node.phase)])} />
      <span className="truncate font-medium">{node.hostname || node.id}</span>
      <span className="truncate font-mono text-xs text-muted-foreground">{nodeAddress(node) || "no address yet"}</span>
      <span className="flex flex-wrap items-center gap-1">
        {image && <span className="truncate rounded bg-muted px-1.5 py-0.5 font-mono text-xs">{image}</span>}
        {recovery && (
          <span className="rounded border border-warning/25 bg-warning/15 px-1.5 py-0.5 text-xs text-warning-foreground">
            {recovery}
          </span>
        )}
      </span>
      <span className="mt-auto flex justify-between gap-2 pt-1 text-xs text-muted-foreground">
        <span className="truncate">{online ? hw || "—" : node.phase || "Unknown"}</span>
        <span className="shrink-0">{heartbeat}</span>
      </span>
    </button>
  );
}

// NodeTiles shows nodes as cards, grouped like the table. Each tile has a
// stripe in the status tone; nodes that are not online are dimmed.
export function NodeTiles({ groups }: NodeTilesProps) {
  const showHeaders = groups.length > 1 || (groups.length === 1 && groups[0].key !== "");
  return (
    <div className="flex flex-col gap-5">
      {groups.map((g) => {
        const online = g.nodes.filter((n) => isOnline(n.phase)).length;
        return (
          <section key={g.key} aria-label={showHeaders ? g.label : "Nodes"}>
            {showHeaders && (
              <div className="mb-2 flex flex-wrap items-center gap-3">
                <h3 className="text-sm font-semibold">{g.label}</h3>
                <span className="text-xs text-muted-foreground">
                  {online}/{g.nodes.length} online
                </span>
                <StackBar counts={phaseCounts(g.nodes)} className="w-20" />
              </div>
            )}
            <div className="grid grid-cols-[repeat(auto-fill,minmax(11rem,1fr))] gap-2">
              {g.nodes.map((n) => (
                <Tile key={n.id} node={n} />
              ))}
            </div>
          </section>
        );
      })}
    </div>
  );
}

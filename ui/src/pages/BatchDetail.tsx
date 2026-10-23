import { useCallback, useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import { AlertTriangle } from "lucide-react";
import { getBatch, stopBatch, type Batch, type BatchNodeState } from "@/api/batches";
import { getNode, sendBulkCommand } from "@/api/nodes";
import { CommandDialog, type CommandKey } from "@/components/CommandDialog";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { PageHeader } from "@/components/PageHeader";
import { StatusBadge } from "@/components/StatusBadge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  commandLabel,
  commandReboots,
  isBatchActive,
  nodeDeletedMessage,
  nonZeroCounts,
  phaseStatus,
  remainingNodes,
  stateLabels,
} from "@/lib/batch";
import { toast } from "@/hooks/useToast";

const POLL_MS = 5000;

function formatTime(t: string | null | undefined): string {
  return t ? new Date(t).toLocaleString() : "-";
}

// stateStatus maps a node state to a phase string the shared badge colors.
function stateStatus(state: BatchNodeState): string {
  switch (state) {
    case "done":
      return "completed";
    case "failed":
    case "timed_out":
      return "failed";
    case "running":
    case "rebooting":
      return "running";
    case "waiting":
      return "pending";
    default:
      return "neutral";
  }
}

function settingsText(batch: Batch): string {
  const parts = [
    batch.concurrency > 0 ? `${batch.concurrency} at a time` : "All at a time",
    batch.stopOnFailure ? "stop on failure" : "keep going on failure",
  ];
  if (commandReboots(batch.command)) {
    parts.push(`wait ${Math.ceil(batch.rebootTimeoutSeconds / 60)} min`);
  }
  return parts.join(", ");
}

export function BatchDetail() {
  const { id } = useParams();
  const navigate = useNavigate();
  const [batch, setBatch] = useState<Batch | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [cameBack, setCameBack] = useState<string | null>(null);
  const [confirmStop, setConfirmStop] = useState(false);
  const [newBatchOpen, setNewBatchOpen] = useState(false);

  const load = useCallback(() => {
    if (!id) return;
    getBatch(id)
      .then(setBatch)
      .catch((err) => setError((err as Error).message));
  }, [id]);

  useEffect(() => {
    load();
  }, [load]);

  const active = batch ? isBatchActive(batch) : false;
  useEffect(() => {
    if (!active) return;
    const t = setInterval(load, POLL_MS);
    return () => clearInterval(t);
  }, [active, load]);

  // The banner mentions a node that timed out and later sent a heartbeat. The
  // node is looked up once the batch has stopped.
  const stoppedBy =
    batch?.phase === "stopped" && batch.stoppedByNodeID
      ? (batch.nodes ?? []).find(
          (n) => n.nodeID === batch.stoppedByNodeID && n.state === "timed_out",
        )
      : undefined;
  const stoppedByID = stoppedBy?.nodeID;
  const stoppedByDeadline = stoppedBy?.deadline;
  useEffect(() => {
    if (!stoppedByID || !stoppedByDeadline) return;
    let live = true;
    getNode(stoppedByID)
      .then((n) => {
        if (!live || !n.lastHeartbeat) return;
        if (new Date(n.lastHeartbeat) > new Date(stoppedByDeadline)) {
          setCameBack(n.lastHeartbeat);
        }
      })
      .catch(() => {});
    return () => {
      live = false;
    };
  }, [stoppedByID, stoppedByDeadline]);

  if (!batch) {
    return (
      <div className="text-muted-foreground">
        {error ? `Failed to load batch: ${error}` : "Loading..."}
      </div>
    );
  }

  const rows = batch.nodes ?? [];
  const remaining = remainingNodes(batch);
  const finished = batch.phase !== "running";

  async function handleStop() {
    try {
      setBatch(await stopBatch(batch!.id));
    } catch (err) {
      toast(`Failed to stop batch: ${(err as Error).message}`, "error");
    }
  }

  async function handleNewBatch(
    command: string,
    args: Record<string, unknown>,
    rollout?: Parameters<typeof sendBulkCommand>[3],
    nodeIDs?: string[],
  ) {
    try {
      const created = await sendBulkCommand({ nodeIDs }, command, args, rollout);
      navigate(`/batches/${created.id}`);
    } catch (err) {
      toast(`Failed to start batch: ${(err as Error).message}`, "error");
    }
  }

  return (
    <div>
      <PageHeader
        title={`${commandLabel(batch.command)} · ${batch.selection}`}
        breadcrumb={[{ label: "Batches", to: "/batches" }, { label: commandLabel(batch.command) }]}
        status={<StatusBadge status={phaseStatus(batch.phase)} label={batch.phase} />}
        meta={<span>{settingsText(batch)}</span>}
      >
        {batch.phase === "running" && (
          <Button variant="destructive" onClick={() => setConfirmStop(true)}>
            Stop batch
          </Button>
        )}
        {finished && remaining.length > 0 && (
          <Button onClick={() => setNewBatchOpen(true)}>
            {`Start a new batch for the ${remaining.length} remaining nodes`}
          </Button>
        )}
      </PageHeader>

      {batch.phase === "stopped" && (
        <div
          role="alert"
          className="mb-6 flex gap-3 rounded-md border border-danger/40 bg-danger/10 px-4 py-3 text-sm"
        >
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-danger-foreground" aria-hidden="true" />
          <div>
            <p className="font-medium">
              Stopped at {formatTime(batch.stoppedAt)}: {batch.stopReason}
            </p>
            {cameBack && stoppedBy && (
              <p className="mt-1">
                {stoppedBy.hostname} came back at {formatTime(cameBack)}. The batch stays stopped.
              </p>
            )}
          </div>
        </div>
      )}

      <div className="mb-4 flex flex-wrap gap-x-6 gap-y-1 text-sm text-muted-foreground">
        {nonZeroCounts(batch).map(([label, n]) => (
          <span key={label}>
            <span className="font-semibold text-foreground">{n}</span> {label}
          </span>
        ))}
      </div>

      <Card>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Node</TableHead>
                <TableHead>State</TableHead>
                <TableHead>Message</TableHead>
                <TableHead>Started</TableHead>
                <TableHead>Finished</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((n) => (
                <TableRow key={n.nodeID}>
                  <TableCell className="font-medium">
                    {n.message === nodeDeletedMessage ? (
                      n.hostname
                    ) : (
                      <Link to={`/nodes/${n.nodeID}`} className="hover:underline">
                        {n.hostname}
                      </Link>
                    )}
                  </TableCell>
                  <TableCell>
                    <StatusBadge
                      status={stateStatus(n.state)}
                      label={stateLabels[n.state]}
                      className="whitespace-nowrap"
                    />
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">{n.message}</TableCell>
                  <TableCell className="text-xs text-muted-foreground">{formatTime(n.startedAt)}</TableCell>
                  <TableCell className="text-xs text-muted-foreground">{formatTime(n.finishedAt)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      <ConfirmDialog
        open={confirmStop}
        onOpenChange={setConfirmStop}
        title="Stop batch"
        description="Nodes that have not started are canceled. Nodes already running finish their command."
        confirmLabel="Stop batch"
        destructive
        onConfirm={handleStop}
      />

      <CommandDialog
        open={newBatchOpen}
        onOpenChange={setNewBatchOpen}
        onSubmit={handleNewBatch}
        title="Start a new batch"
        defaultCommand={batch.command as CommandKey}
        defaultArgs={batch.args}
        defaultRollout={{
          concurrency: batch.concurrency > 0 ? batch.concurrency : undefined,
          stopOnFailure: batch.stopOnFailure,
          rebootTimeoutSeconds: batch.rebootTimeoutSeconds,
        }}
        nodeChoices={remaining.map((n) => ({ id: n.nodeID, label: n.hostname }))}
      />
    </div>
  );
}

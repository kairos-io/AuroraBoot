import { useEffect, useState } from "react";
import { X } from "lucide-react";
import { getBatchStatus, type BatchOutcome, type Command } from "@/api/commands";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { StatusBadge } from "@/components/StatusBadge";
import { abortCause, batchLabel, finished, isBatchTerminal, progressPercent } from "@/lib/batch";
import { phaseTone } from "@/lib/phase";
import { cn } from "@/lib/utils";
import { toneBg, toneText } from "./tones";

// How often an unfinished rollout is re-read.
//
// Polling rather than the UI WebSocket is deliberate. `command_update` frames
// carry only {id, phase, result} with no batch, and the cancellations a
// fail-fast batch fans out are store writes that broadcast nothing at all, so
// a socket-driven panel would miss exactly the event this panel exists to
// show. Re-reading the batch endpoint gets the whole rollout, cancellations
// included, from the one place that derives it.
const POLL_MS = 3000;

interface BatchProgressProps {
  batchID: string;
  onDismiss?(): void;
  // Injectable so tests do not have to drive a timer.
  pollMs?: number;
}

const countOrder: Array<{ key: keyof BatchOutcome; label: string; phase: string }> = [
  { key: "completed", label: "Completed", phase: "Completed" },
  { key: "running", label: "Running", phase: "Running" },
  { key: "pending", label: "Pending", phase: "Pending" },
  { key: "queued", label: "Queued", phase: "Queued" },
  { key: "failed", label: "Failed", phase: "Failed" },
  { key: "canceled", label: "Canceled", phase: "Canceled" },
  { key: "expired", label: "Expired", phase: "Expired" },
];

function nodeOf(cmd: Command): string {
  return cmd.managedNodeID || cmd.id;
}

// BatchProgress follows one fan-out until it is over: how far it has got, the
// settings it ran under, which nodes are where, and — for a fail-fast batch
// that was cut short — the node and the error that stopped it.
export function BatchProgress({ batchID, onDismiss, pollMs = POLL_MS }: BatchProgressProps) {
  // Holds whatever the last read returned, including which batch it was for:
  // pointing the panel at a second rollout must not show the first one's
  // numbers under the new heading while the first read is in flight.
  const [loaded, setLoaded] = useState<BatchOutcome | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    let cancelled = false;
    // Whether the batch has reached a phase it can no longer leave. Local to
    // the effect rather than state, so recording it cannot itself cause a
    // render and restart the interval.
    let done = false;

    async function read() {
      try {
        const next = await getBatchStatus(batchID);
        if (cancelled) return;
        setLoaded(next);
        setError("");
        done = isBatchTerminal(next);
      } catch (err) {
        if (!cancelled) setError((err as Error).message);
      }
    }

    void read();
    const timer = setInterval(() => {
      // A finished batch never changes again, so stop reading it. The interval
      // is cleared rather than left running on a no-op tick.
      if (done) {
        clearInterval(timer);
        return;
      }
      void read();
    }, pollMs);

    return () => {
      cancelled = true;
      clearInterval(timer);
    };
  }, [batchID, pollMs]);

  // A payload for a different batch is the previous rollout still arriving.
  const batch = loaded && loaded.batchID === batchID ? loaded : null;

  if (error && !batch) {
    return (
      <Card data-testid="batch-progress">
        <CardContent className="py-4 text-sm text-danger">Could not read rollout: {error}</CardContent>
      </Card>
    );
  }

  if (!batch) {
    return (
      <Card data-testid="batch-progress">
        <CardContent className="py-4 text-sm text-muted-foreground">Loading rollout...</CardContent>
      </Card>
    );
  }

  const percent = progressPercent(batch);
  const cause = abortCause(batch);
  const tone = phaseTone(batch.phase);

  return (
    <Card data-testid="batch-progress">
      <CardHeader className="flex flex-row items-start justify-between gap-2 space-y-0">
        <div>
          <CardTitle className="text-base">
            Rollout: <span className="font-mono">{batch.command || "command"}</span>
          </CardTitle>
          <p className="mt-1 text-xs text-muted-foreground">
            {finished(batch)} of {batch.total} node{batch.total !== 1 ? "s" : ""} done
            {batch.failFast ? " · fail-fast" : ""}
            {batch.maxConcurrency ? ` · at most ${batch.maxConcurrency} at a time` : ""}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <span data-testid="batch-phase">
            <StatusBadge status={batchLabel(batch)} />
          </span>
          {onDismiss && (
            <Button variant="ghost" size="icon" aria-label="Dismiss rollout" onClick={onDismiss}>
              <X aria-hidden="true" />
            </Button>
          )}
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="h-1.5 w-full overflow-hidden rounded-full bg-muted" aria-hidden="true">
          <div className={cn("h-full rounded-full", toneBg[tone])} style={{ width: `${percent}%` }} />
        </div>

        <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs">
          {countOrder.map(({ key, label, phase }) => {
            const n = batch[key] as number | undefined;
            if (!n) return null;
            return (
              <span key={label} data-testid={`batch-count-${label.toLowerCase()}`}>
                <span className={cn("font-semibold tabular-nums", toneText[phaseTone(phase)])}>{n}</span>{" "}
                <span className="text-muted-foreground">{label}</span>
              </span>
            );
          })}
        </div>

        {cause && (
          <div data-testid="batch-abort-cause" className="rounded border border-danger/25 bg-danger/10 p-3 text-xs">
            <p className="font-semibold text-danger-foreground">
              Stopped after node {nodeOf(cause)} failed
            </p>
            {cause.result && (
              <pre className="mt-1 max-h-24 overflow-y-auto whitespace-pre-wrap font-mono">{cause.result}</pre>
            )}
          </div>
        )}

        {batch.commands?.length > 0 && (
          <ul className="divide-y text-xs">
            {batch.commands.map((cmd) => (
              <li key={cmd.id} className="flex items-center justify-between gap-2 py-1.5">
                <span className="truncate font-mono">{nodeOf(cmd)}</span>
                <StatusBadge status={cmd.phase} />
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}

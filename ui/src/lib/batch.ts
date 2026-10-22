// Reading one fan-out of commands as a single rollout.
//
// The server already folds the rows into a BatchOutcome, so nothing here
// recounts them. What it adds is the two questions the counts alone do not
// answer: is this rollout over, and if it ended badly, which node ended it.

import type { BatchOutcome, Command } from "@/api/commands";

// A batch is over when the server stops moving it. SummarizeBatch reports
// Pending while any node still has work ahead of it, so anything else is
// terminal and there is nothing left to wait for.
export function isBatchTerminal(batch: Pick<BatchOutcome, "phase">): boolean {
  return (batch.phase ?? "").toLowerCase() !== "pending";
}

// finished is every row that will not change again.
export function finished(batch: BatchOutcome): number {
  return batch.completed + batch.failed + batch.canceled + batch.expired;
}

// progressPercent is how much of the batch has reached a terminal phase. An
// empty batch reads as complete rather than as a division by zero.
export function progressPercent(batch: BatchOutcome): number {
  if (batch.total <= 0) return 100;
  return Math.round((finished(batch) / batch.total) * 100);
}

// abortCause names the node whose failure stopped a fail-fast batch, and the
// result it reported.
//
// It answers only for a batch that actually was cut short: fail-fast has to
// have been asked for, a node has to have failed, and the rule has to have
// reached siblings, which is what the canceled rows are. A fail-fast batch in
// which the failure landed last cancels nothing and is a plain failure, not an
// abort, so there is no cause to show.
//
// The earliest failure is the one that triggered the cancellation. Rows the
// server already reported in order are kept in that order; `completedAt` is
// what orders them when it is set, because a node that failed without ever
// completing has none.
export function abortCause(batch: BatchOutcome): Command | null {
  if (!batch.failFast || batch.failed === 0 || batch.canceled === 0) return null;
  const failures = (batch.commands ?? []).filter((c) => (c.phase ?? "").toLowerCase() === "failed");
  if (failures.length === 0) return null;
  return failures.reduce((earliest, cmd) => (before(cmd, earliest) ? cmd : earliest));
}

function before(a: Command, b: Command): boolean {
  const ta = stamp(a);
  const tb = stamp(b);
  if (ta === null) return false;
  if (tb === null) return true;
  return ta < tb;
}

function stamp(cmd: Command): number | null {
  if (!cmd.completedAt) return null;
  const t = new Date(cmd.completedAt).getTime();
  return Number.isNaN(t) ? null : t;
}

// batchLabel is how the rollout's overall phase should read to an operator.
// "Pending" is what the store calls a batch that is under way, which to
// somebody watching a rollout means the opposite of pending, and "Canceled"
// hides that the rollout was stopped by a failure rather than by a person.
export function batchLabel(batch: BatchOutcome): string {
  const phase = (batch.phase ?? "").toLowerCase();
  if (phase === "pending") return "In progress";
  if (phase === "completed") return "Succeeded";
  if (phase === "failed") return abortCause(batch) ? "Aborted" : "Failed";
  if (phase === "canceled") return "Canceled";
  return batch.phase || "unknown";
}

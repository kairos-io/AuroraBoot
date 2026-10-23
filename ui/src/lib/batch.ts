import type { Batch, BatchNode, BatchNodeState } from "@/api/batches";

const commandLabels: Record<string, string> = {
  upgrade: "Upgrade",
  "upgrade-recovery": "Upgrade recovery",
  reset: "Reset",
  "apply-config": "Apply config",
  reboot: "Reboot",
  exec: "Run command",
};

// A stopped batch needs attention, so its badge reads as a failure.
export function phaseStatus(phase: string): string {
  return phase === "stopped" ? "failed" : phase;
}

export function commandLabel(command: string): string {
  return commandLabels[command] ?? command;
}

// The states a node can end in without having finished its command.
const remainingStates: BatchNodeState[] = ["failed", "timed_out", "skipped", "canceled"];

const stateOrder: BatchNodeState[] = [
  "waiting",
  "running",
  "rebooting",
  "done",
  "failed",
  "timed_out",
  "skipped",
  "canceled",
];

export const stateLabels: Record<BatchNodeState, string> = {
  waiting: "waiting",
  running: "running",
  rebooting: "rebooting",
  done: "done",
  failed: "failed",
  timed_out: "timed out",
  skipped: "skipped",
  canceled: "canceled",
};

export const nodeDeletedMessage = "node deleted";

export function remainingNodes(batch: Batch): BatchNode[] {
  return (batch.nodes ?? []).filter(
    (n) => remainingStates.includes(n.state) && n.message !== nodeDeletedMessage,
  );
}

// nonZeroCounts lists the states that have nodes, in lifecycle order, as
// [state label, count] pairs.
export function nonZeroCounts(batch: Batch): [string, number][] {
  return stateOrder
    .filter((s) => (batch.counts[s] ?? 0) > 0)
    .map((s) => [stateLabels[s], batch.counts[s] as number]);
}

export function countsText(batch: Batch): string {
  return nonZeroCounts(batch)
    .map(([label, n]) => `${n} ${label}`)
    .join(", ");
}

export function commandReboots(command: string): boolean {
  return command === "upgrade" || command === "reset" || command === "reboot";
}

// A batch is live while it is running or any node is still in flight.
export function isBatchActive(batch: Batch): boolean {
  return (
    batch.phase === "running" ||
    (batch.nodes ?? []).some((n) => n.state === "running" || n.state === "rebooting")
  );
}

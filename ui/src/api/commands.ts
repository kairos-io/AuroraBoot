import { apiFetch } from "./client";

export interface Command {
  id: string;
  managedNodeID: string;
  command: string;
  args: Record<string, string>;
  phase: string;
  result: string;
  expiresAt: string | null;
  deliveredAt: string | null;
  completedAt: string | null;
  createdAt: string;
  // batchID names the fan-out this command was created by. The server omits
  // it on a command addressed to a single node, so it is optional here.
  batchID?: string;
  // failFast is carried on every row of a batch rather than on a batch
  // record, and the server omits it when false.
  failFast?: boolean;
}

// BatchOutcome is the server's view of one fan-out, derived from the commands
// it created. The counts and the phase come from store.SummarizeBatch, so they
// cannot drift from the rows in `commands`.
export interface BatchOutcome {
  batchID: string;
  command: string;
  failFast: boolean;
  // phase is Pending while any node still has work ahead of it, then Failed,
  // Canceled or Completed once every node is terminal.
  phase: string;
  total: number;
  pending: number;
  running: number;
  completed: number;
  failed: number;
  canceled: number;
  expired: number;
  // queued and maxConcurrency are sent by servers that pace a batch. An older
  // server omits both, which reads as "no limit", so they stay optional.
  queued?: number;
  maxConcurrency?: number;
  commands: Command[];
}

export function listNodeCommands(nodeID: string): Promise<Command[]> {
  return apiFetch<Command[]>(`/api/v1/nodes/${nodeID}/commands`);
}

export function getBatchStatus(batchID: string): Promise<BatchOutcome> {
  return apiFetch<BatchOutcome>(`/api/v1/commands/batches/${batchID}`);
}

export function deleteCommand(nodeID: string, commandID: string): Promise<void> {
  return apiFetch(`/api/v1/nodes/${nodeID}/commands/${commandID}`, { method: "DELETE" });
}

export function clearCommandHistory(nodeID: string): Promise<void> {
  return apiFetch(`/api/v1/nodes/${nodeID}/commands`, { method: "DELETE" });
}

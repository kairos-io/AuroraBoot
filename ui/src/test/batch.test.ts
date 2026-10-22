import { describe, it, expect } from "vitest";
import type { BatchOutcome, Command } from "@/api/commands";
import {
  abortCause,
  batchLabel,
  finished,
  isBatchTerminal,
  progressPercent,
} from "@/lib/batch";

function cmd(id: string, phase: string, extra: Partial<Command> = {}): Command {
  return {
    id,
    managedNodeID: `node-${id}`,
    command: "upgrade",
    args: {},
    phase,
    result: "",
    expiresAt: null,
    deliveredAt: null,
    completedAt: null,
    createdAt: "2026-10-22T10:00:00Z",
    ...extra,
  };
}

function batch(over: Partial<BatchOutcome> = {}): BatchOutcome {
  return {
    batchID: "b-1",
    command: "upgrade",
    failFast: false,
    phase: "Pending",
    total: 0,
    pending: 0,
    running: 0,
    completed: 0,
    failed: 0,
    canceled: 0,
    expired: 0,
    commands: [],
    ...over,
  };
}

describe("isBatchTerminal", () => {
  it("treats Pending as still running", () => {
    expect(isBatchTerminal({ phase: "Pending" })).toBe(false);
  });

  it("treats every other phase as over", () => {
    for (const phase of ["Completed", "Failed", "Canceled"]) {
      expect(isBatchTerminal({ phase })).toBe(true);
    }
  });
});

describe("counts", () => {
  it("counts canceled and expired rows as finished, not as pending work", () => {
    expect(finished(batch({ completed: 1, failed: 1, canceled: 2, expired: 1 }))).toBe(5);
  });

  it("reports progress over the whole batch", () => {
    expect(progressPercent(batch({ total: 4, completed: 1 }))).toBe(25);
  });

  it("does not divide by zero on an empty batch", () => {
    expect(progressPercent(batch({ total: 0 }))).toBe(100);
  });
});

describe("abortCause", () => {
  const aborted = batch({
    failFast: true,
    phase: "Failed",
    total: 3,
    failed: 1,
    canceled: 2,
    commands: [
      cmd("a", "Canceled", { result: "canceled: batch b-1 stopped after node node-b failed" }),
      cmd("b", "Failed", { result: "exit status 1", completedAt: "2026-10-22T10:05:00Z" }),
      cmd("c", "Canceled"),
    ],
  });

  it("names the node that stopped the batch", () => {
    expect(abortCause(aborted)?.managedNodeID).toBe("node-b");
  });

  it("carries the error the node reported", () => {
    expect(abortCause(aborted)?.result).toBe("exit status 1");
  });

  it("picks the earliest failure when more than one node failed", () => {
    const two = batch({
      failFast: true,
      phase: "Failed",
      failed: 2,
      canceled: 1,
      commands: [
        cmd("late", "Failed", { completedAt: "2026-10-22T10:09:00Z" }),
        cmd("first", "Failed", { completedAt: "2026-10-22T10:02:00Z" }),
        cmd("c", "Canceled"),
      ],
    });
    expect(abortCause(two)?.id).toBe("first");
  });

  it("is silent for a batch that did not ask for fail-fast", () => {
    expect(abortCause({ ...aborted, failFast: false })).toBeNull();
  });

  it("is silent when the failure cancelled nothing", () => {
    // Fail-fast only cuts a rollout short if siblings were still pending. A
    // last-node failure is a plain failure, not an abort.
    const last = batch({
      failFast: true,
      phase: "Failed",
      failed: 1,
      canceled: 0,
      commands: [cmd("a", "Completed"), cmd("b", "Failed")],
    });
    expect(abortCause(last)).toBeNull();
  });

  it("is silent while the batch is still going", () => {
    expect(abortCause(batch({ failFast: true, phase: "Pending", pending: 2 }))).toBeNull();
  });
});

describe("batchLabel", () => {
  it("calls a running batch in progress, not pending", () => {
    expect(batchLabel(batch({ phase: "Pending", pending: 3 }))).toBe("In progress");
  });

  it("calls a clean batch succeeded", () => {
    expect(batchLabel(batch({ phase: "Completed", completed: 3 }))).toBe("Succeeded");
  });

  it("separates an aborted rollout from a plain failure", () => {
    const aborted = batch({
      failFast: true,
      phase: "Failed",
      failed: 1,
      canceled: 1,
      commands: [cmd("a", "Failed"), cmd("b", "Canceled")],
    });
    expect(batchLabel(aborted)).toBe("Aborted");
    expect(batchLabel({ ...aborted, failFast: false })).toBe("Failed");
  });

  it("keeps a batch cancelled without a failure distinct", () => {
    expect(batchLabel(batch({ phase: "Canceled", canceled: 2 }))).toBe("Canceled");
  });
});

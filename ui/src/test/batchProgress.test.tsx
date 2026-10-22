import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";

import { BatchProgress } from "@/components/fleet/BatchProgress";
import type { BatchOutcome, Command } from "@/api/commands";

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
    batchID: "b-1",
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

// Serves one batch body per state, holding each until the test advances, so a
// rollout can be stepped through without racing the poll interval.
function mockFetch(bodies: BatchOutcome[], opts: { status?: number } = {}) {
  let i = 0;
  const fn = vi.fn(async (input: RequestInfo | URL) => {
    const url = typeof input === "string" ? input : input.toString();
    if (!url.startsWith("/api/v1/commands/batches/")) {
      return new Response("[]", { status: 200, headers: { "Content-Type": "application/json" } });
    }
    if (opts.status && opts.status >= 400) {
      return new Response(JSON.stringify({ error: "no such batch" }), {
        status: opts.status,
        headers: { "Content-Type": "application/json" },
      });
    }
    return new Response(JSON.stringify(bodies[i]), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
  });
  vi.stubGlobal("fetch", fn);
  return { fn, advance: () => { i = Math.min(i + 1, bodies.length - 1); } };
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("BatchProgress", () => {
  it("reads the batch the rollout created", async () => {
    const { fn: fetchMock } = mockFetch([
      batch({ total: 3, pending: 2, running: 1, commands: [cmd("a", "Running"), cmd("b", "Pending")] }),
    ]);
    render(<BatchProgress batchID="b-1" />);

    await waitFor(() => expect(screen.getByTestId("batch-phase")).toHaveTextContent("In progress"));
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/commands/batches/b-1");
    expect(screen.getByText(/0 of 3 nodes done/)).toBeInTheDocument();
  });

  it("shows each node and the phase it is in", async () => {
    mockFetch([
      batch({
        total: 2,
        running: 1,
        completed: 1,
        commands: [cmd("a", "Completed"), cmd("b", "Running")],
      }),
    ]);
    render(<BatchProgress batchID="b-1" />);

    await waitFor(() => expect(screen.getByText("node-a")).toBeInTheDocument());
    expect(screen.getByText("node-b")).toBeInTheDocument();
    expect(screen.getByTestId("batch-count-completed")).toHaveTextContent("1 Completed");
    expect(screen.getByTestId("batch-count-running")).toHaveTextContent("1 Running");
  });

  it("surfaces the fail-fast and concurrency settings the batch ran under", async () => {
    mockFetch([batch({ total: 10, pending: 6, queued: 3, running: 1, failFast: true, maxConcurrency: 2 })]);
    render(<BatchProgress batchID="b-1" />);

    await waitFor(() => expect(screen.getByText(/fail-fast/)).toBeInTheDocument());
    expect(screen.getByText(/at most 2 at a time/)).toBeInTheDocument();
    expect(screen.getByTestId("batch-count-queued")).toHaveTextContent("3 Queued");
  });

  it("says nothing about a limit a server did not report", async () => {
    mockFetch([batch({ total: 2, pending: 2 })]);
    render(<BatchProgress batchID="b-1" />);

    await waitFor(() => expect(screen.getByTestId("batch-progress")).toBeInTheDocument());
    expect(screen.queryByText(/at a time/)).not.toBeInTheDocument();
    expect(screen.queryByTestId("batch-count-queued")).not.toBeInTheDocument();
  });

  it("names the node and the error that aborted a fail-fast rollout", async () => {
    mockFetch([
      batch({
        failFast: true,
        phase: "Failed",
        total: 3,
        failed: 1,
        canceled: 2,
        commands: [
          cmd("a", "Failed", { result: "upgrade: exit status 1", completedAt: "2026-10-22T10:05:00Z" }),
          cmd("b", "Canceled", { result: "canceled: batch b-1 stopped after node node-a failed" }),
          cmd("c", "Canceled"),
        ],
      }),
    ]);
    render(<BatchProgress batchID="b-1" />);

    await waitFor(() => expect(screen.getByTestId("batch-abort-cause")).toBeInTheDocument());
    const cause = screen.getByTestId("batch-abort-cause");
    expect(cause).toHaveTextContent("Stopped after node node-a failed");
    expect(cause).toHaveTextContent("upgrade: exit status 1");
    expect(screen.getByTestId("batch-phase")).toHaveTextContent("Aborted");
  });

  it("does not claim an abort when no sibling was stopped", async () => {
    mockFetch([
      batch({
        failFast: true,
        phase: "Failed",
        total: 2,
        completed: 1,
        failed: 1,
        commands: [cmd("a", "Completed"), cmd("b", "Failed", { result: "boom" })],
      }),
    ]);
    render(<BatchProgress batchID="b-1" />);

    await waitFor(() => expect(screen.getByTestId("batch-phase")).toHaveTextContent("Failed"));
    expect(screen.queryByTestId("batch-abort-cause")).not.toBeInTheDocument();
  });

  it("follows the rollout to its outcome", async () => {
    const { advance } = mockFetch([
      batch({ total: 2, pending: 2, commands: [cmd("a", "Pending"), cmd("b", "Pending")] }),
      batch({ total: 2, running: 1, completed: 1, commands: [cmd("a", "Completed"), cmd("b", "Running")] }),
      batch({
        phase: "Completed",
        total: 2,
        completed: 2,
        commands: [cmd("a", "Completed"), cmd("b", "Completed")],
      }),
    ]);
    render(<BatchProgress batchID="b-1" pollMs={10} />);

    await waitFor(() => expect(screen.getByTestId("batch-phase")).toHaveTextContent("In progress"));
    expect(screen.getByText(/0 of 2 nodes done/)).toBeInTheDocument();

    advance();
    await waitFor(() => expect(screen.getByText(/1 of 2 nodes done/)).toBeInTheDocument());
    expect(screen.getByTestId("batch-phase")).toHaveTextContent("In progress");

    advance();
    await waitFor(() => expect(screen.getByTestId("batch-phase")).toHaveTextContent("Succeeded"));
    expect(screen.getByText(/2 of 2 nodes done/)).toBeInTheDocument();
  });

  it("stops reading a batch that is over", async () => {
    const { fn: fetchMock } = mockFetch([batch({ phase: "Completed", total: 1, completed: 1 })]);
    render(<BatchProgress batchID="b-1" pollMs={10} />);

    await waitFor(() => expect(screen.getByTestId("batch-phase")).toHaveTextContent("Succeeded"));
    const afterFirst = fetchMock.mock.calls.length;
    // Long enough for many more polls had the interval been left running.
    await new Promise((r) => setTimeout(r, 150));
    expect(fetchMock.mock.calls.length).toBe(afterFirst);
  });

  it("does not show one rollout's numbers under another rollout's name", async () => {
    // Pointing the panel at a second batch must not leave the first one's
    // counts on screen while the new read is still in flight.
    let release: (() => void) | null = null;
    const gate = new Promise<void>((r) => { release = r; });
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.endsWith("/b-1")) {
          return new Response(JSON.stringify(batch({ batchID: "b-1", total: 9, completed: 9, phase: "Completed" })), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          });
        }
        await gate;
        return new Response(JSON.stringify(batch({ batchID: "b-2", total: 1, pending: 1 })), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );

    const view = render(<BatchProgress batchID="b-1" pollMs={10} />);
    await waitFor(() => expect(screen.getByText(/9 of 9 nodes done/)).toBeInTheDocument());

    view.rerender(<BatchProgress batchID="b-2" pollMs={10} />);
    await waitFor(() => expect(screen.getByText("Loading rollout...")).toBeInTheDocument());
    expect(screen.queryByText(/9 of 9 nodes done/)).not.toBeInTheDocument();

    release!();
    await waitFor(() => expect(screen.getByText(/0 of 1 node done/)).toBeInTheDocument());
  });

  it("reports a batch it cannot read instead of rendering an empty rollout", async () => {
    mockFetch([], { status: 404 });
    render(<BatchProgress batchID="b-1" />);

    await waitFor(() => expect(screen.getByText(/Could not read rollout/)).toBeInTheDocument());
    expect(screen.getByText(/no such batch/)).toBeInTheDocument();
  });
});

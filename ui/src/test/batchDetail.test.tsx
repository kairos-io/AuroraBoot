import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { act, render, screen, waitFor, fireEvent, within } from "@testing-library/react";
import { Link, MemoryRouter, Route, Routes } from "react-router";

import { BatchDetail } from "@/pages/BatchDetail";
import type { Batch, BatchNode } from "@/api/batches";

const getBatch = vi.fn();
const stopBatch = vi.fn();
const getNode = vi.fn();
const sendBulkCommand = vi.fn();

vi.mock("@/api/batches", () => ({
  getBatch: (...a: unknown[]) => getBatch(...a),
  stopBatch: (...a: unknown[]) => stopBatch(...a),
  listBatches: vi.fn(),
}));
vi.mock("@/api/nodes", () => ({
  getNode: (...a: unknown[]) => getNode(...a),
  sendBulkCommand: (...a: unknown[]) => sendBulkCommand(...a),
}));
const resolveBundle = vi.fn();
vi.mock("@/api/artifacts", () => ({
  listArtifacts: vi.fn().mockResolvedValue([]),
  resolveBundle: (...a: unknown[]) => resolveBundle(...a),
}));

function row(id: string, state: BatchNode["state"], extra: Partial<BatchNode> = {}): BatchNode {
  return {
    batchID: "b1",
    nodeID: id,
    position: 0,
    hostname: `host-${id}`,
    commandID: null,
    state,
    message: "",
    startedAt: null,
    deadline: null,
    finishedAt: null,
    ...extra,
  };
}

function makeBatch(extra: Partial<Batch> = {}): Batch {
  return {
    id: "b1",
    command: "upgrade",
    args: { source: "oci:quay.io/kairos/kairos:v4" },
    selection: "group edge",
    groupID: "g1",
    concurrency: 2,
    stopOnFailure: true,
    rebootTimeoutSeconds: 600,
    phase: "running",
    createdAt: "2026-10-23T10:00:00Z",
    finishedAt: null,
    stoppedAt: null,
    stopReason: "",
    stoppedByNodeID: null,
    counts: {},
    nodes: [],
    ...extra,
  };
}

function renderPage() {
  return render(
    <MemoryRouter initialEntries={["/batches/b1"]}>
      <Link to="/batches/b2">to b2</Link>
      <Routes>
        <Route path="/batches/:id" element={<BatchDetail />} />
        <Route path="/batches/:id/other" element={<div />} />
      </Routes>
    </MemoryRouter>,
  );
}

// advanceTimersByTime fires the poll; the getBatch promise settles on the
// microtask queue, so act waits for it before the assertions run.
async function tick(ms: number) {
  await act(async () => {
    vi.advanceTimersByTime(ms);
    await Promise.resolve();
  });
}

beforeEach(() => {
  vi.clearAllMocks();
  resolveBundle.mockResolvedValue([]);
});

afterEach(() => {
  vi.useRealTimers();
});

describe("BatchDetail", () => {
  it("explains why a stopped batch stopped", async () => {
    getBatch.mockResolvedValue(
      makeBatch({
        phase: "stopped",
        stoppedAt: "2026-10-23T10:30:00Z",
        stopReason: "host-n1 failed: disk full",
        stoppedByNodeID: "n1",
        counts: { failed: 1, canceled: 1 },
        nodes: [row("n1", "failed", { message: "disk full" }), row("n2", "canceled")],
      }),
    );
    renderPage();
    const banner = await screen.findByRole("alert");
    expect(banner).toHaveTextContent(/Stopped at .*: host-n1 failed: disk full/);
    expect(screen.queryByText(/came back at/)).not.toBeInTheDocument();
    expect(screen.getByText("2 at a time, stop on failure, wait 10 min")).toBeInTheDocument();
  });

  it("says when the node that timed out came back", async () => {
    getBatch.mockResolvedValue(
      makeBatch({
        phase: "stopped",
        stoppedAt: "2026-10-23T10:30:00Z",
        stopReason: "host-n1 did not come back in time",
        stoppedByNodeID: "n1",
        counts: { timed_out: 1 },
        nodes: [row("n1", "timed_out", { deadline: "2026-10-23T10:20:00Z" })],
      }),
    );
    getNode.mockResolvedValue({ id: "n1", hostname: "host-n1", lastHeartbeat: "2026-10-23T10:45:00Z" });
    renderPage();
    const banner = await screen.findByRole("alert");
    await waitFor(() => expect(banner).toHaveTextContent(/host-n1 came back at .*\. The batch stays stopped\./));
    expect(getNode).toHaveBeenCalledWith("n1");
  });

  it("offers a new batch for the nodes that did not finish, without deleted nodes", async () => {
    getBatch.mockResolvedValue(
      makeBatch({
        phase: "stopped",
        stoppedAt: "2026-10-23T10:30:00Z",
        stopReason: "x",
        counts: { failed: 1, canceled: 2, done: 1 },
        nodes: [
          row("n1", "failed"),
          row("n2", "canceled"),
          row("n3", "canceled", { message: "node deleted" }),
          row("n4", "done"),
        ],
      }),
    );
    sendBulkCommand.mockResolvedValue(makeBatch({ id: "b2" }));
    renderPage();
    fireEvent.click(await screen.findByRole("button", { name: "Start a new batch for the 2 remaining nodes" }));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByLabelText("host-n1")).toBeChecked();
    expect(within(dialog).getByLabelText("host-n2")).toBeChecked();
    expect(within(dialog).queryByLabelText("host-n3")).not.toBeInTheDocument();
    fireEvent.click(within(dialog).getByLabelText("host-n1"));
    fireEvent.click(within(dialog).getByRole("button", { name: "Send upgrade" }));
    await waitFor(() => expect(sendBulkCommand).toHaveBeenCalled());
    expect(sendBulkCommand).toHaveBeenCalledWith(
      { nodeIDs: ["n2"] },
      "upgrade",
      { source: "oci:quay.io/kairos/kairos:v4" },
      { stopOnFailure: true, rebootTimeoutSeconds: 600 },
    );
  });

  it("carries the batch's reboot timeout over to the one remaining node", async () => {
    getBatch.mockResolvedValue(
      makeBatch({
        phase: "stopped",
        stoppedAt: "2026-10-23T10:30:00Z",
        stopReason: "x",
        rebootTimeoutSeconds: 1200,
        counts: { failed: 1, done: 1 },
        nodes: [row("n1", "failed"), row("n2", "done")],
      }),
    );
    sendBulkCommand.mockResolvedValue(makeBatch({ id: "b2" }));
    renderPage();
    fireEvent.click(await screen.findByRole("button", { name: "Start a new batch for the 1 remaining node" }));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByLabelText("Wait for a node to come back")).toHaveValue(20);
    fireEvent.click(within(dialog).getByRole("button", { name: "Send upgrade" }));
    await waitFor(() => expect(sendBulkCommand).toHaveBeenCalled());
    expect(sendBulkCommand.mock.calls[0][3]).toEqual({ stopOnFailure: true, rebootTimeoutSeconds: 1200 });
  });

  it("keeps an unticked extension when the batch refreshes with the dialog open", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    resolveBundle.mockResolvedValue([
      { name: "a", type: "sysext", version: "1", source: "s" },
      { name: "b", type: "sysext", version: "1", source: "s" },
    ]);
    // A stopped batch with a node still rebooting keeps polling, and every
    // poll returns a new args object.
    getBatch.mockImplementation(async () =>
      makeBatch({
        args: { source: "artifact:art1" },
        phase: "stopped",
        stoppedAt: "2026-10-23T10:30:00Z",
        stopReason: "x",
        counts: { failed: 2, rebooting: 1 },
        nodes: [row("n1", "failed"), row("n2", "failed"), row("n3", "rebooting")],
      }),
    );
    renderPage();
    fireEvent.click(await screen.findByRole("button", { name: "Start a new batch for the 2 remaining nodes" }));
    const dialog = await screen.findByRole("dialog");
    await waitFor(() => expect(within(dialog).queryByLabelText("include a")).not.toBeNull());
    fireEvent.click(within(dialog).getByLabelText("include a"));
    expect(within(dialog).getByLabelText("include a")).not.toBeChecked();

    const polls = getBatch.mock.calls.length;
    await tick(5000);
    await waitFor(() => expect(getBatch.mock.calls.length).toBeGreaterThan(polls));
    await tick(0);

    expect(within(dialog).getByLabelText("include a")).not.toBeChecked();
    expect(within(dialog).getByLabelText("include b")).toBeChecked();
  });

  it("says when a refresh failed while showing the last batch it loaded", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    getBatch
      .mockResolvedValueOnce(makeBatch({ counts: { running: 1 }, nodes: [row("n1", "running")] }))
      .mockRejectedValue(new Error("connection refused"));
    renderPage();
    await screen.findByRole("button", { name: "Stop batch" });
    expect(screen.queryByText(/Last refresh failed/)).not.toBeInTheDocument();

    await tick(5000);

    expect(await screen.findByText("Last refresh failed: connection refused")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Stop batch" })).toBeInTheDocument();
  });

  it("drops the previous batch's state when it shows another batch", async () => {
    const stoppedBy = (id: string, nodeID: string) =>
      makeBatch({
        id,
        phase: "stopped",
        stoppedAt: "2026-10-23T10:30:00Z",
        stopReason: `${id} stopped`,
        stoppedByNodeID: nodeID,
        counts: { timed_out: 1 },
        nodes: [row(nodeID, "timed_out", { deadline: "2026-10-23T10:20:00Z" })],
      });
    getBatch.mockImplementation(async (id: string) => stoppedBy(id, id === "b1" ? "n1" : "n9"));
    getNode.mockImplementation(async (id: string) => ({
      id,
      hostname: `host-${id}`,
      // n1 came back after its deadline; n9 has not been heard from since.
      lastHeartbeat: id === "n1" ? "2026-10-23T10:45:00Z" : "2026-10-23T10:00:00Z",
    }));
    renderPage();
    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent(/host-n1 came back at/));

    fireEvent.click(screen.getByRole("link", { name: "to b2" }));

    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent("b2 stopped"));
    await waitFor(() => expect(getNode).toHaveBeenCalledWith("n9"));
    expect(screen.getByRole("alert")).not.toHaveTextContent(/came back at/);
  });

  it("stops a running batch after confirmation", async () => {
    getBatch.mockResolvedValue(
      makeBatch({ counts: { running: 1 }, nodes: [row("n1", "running")] }),
    );
    stopBatch.mockResolvedValue(makeBatch({ phase: "stopped" }));
    renderPage();
    fireEvent.click(await screen.findByRole("button", { name: "Stop batch" }));
    expect(stopBatch).not.toHaveBeenCalled();
    const dialog = await screen.findByRole("dialog");
    fireEvent.click(within(dialog).getByRole("button", { name: "Stop batch" }));
    await waitFor(() => expect(stopBatch).toHaveBeenCalledWith("b1"));
  });

  it("carries the batch's rollout settings over when several nodes stay checked", async () => {
    getBatch.mockResolvedValue(
      makeBatch({
        phase: "stopped",
        stoppedAt: "2026-10-23T10:30:00Z",
        stopReason: "x",
        counts: { failed: 2 },
        nodes: [row("n1", "failed"), row("n2", "failed")],
      }),
    );
    sendBulkCommand.mockResolvedValue(makeBatch({ id: "b2" }));
    renderPage();
    fireEvent.click(await screen.findByRole("button", { name: "Start a new batch for the 2 remaining nodes" }));
    const dialog = await screen.findByRole("dialog");
    fireEvent.click(within(dialog).getByRole("button", { name: "Upgrade 2 nodes" }));
    await waitFor(() => expect(sendBulkCommand).toHaveBeenCalled());
    expect(sendBulkCommand).toHaveBeenCalledWith(
      { nodeIDs: ["n1", "n2"] },
      "upgrade",
      { source: "oci:quay.io/kairos/kairos:v4" },
      { concurrency: 2, stopOnFailure: true, rebootTimeoutSeconds: 600 },
    );
  });

  it("sends the original args of an artifact upgrade with the original extensions", async () => {
    const extensions = JSON.stringify([
      { name: "a", type: "sysext", version: "1", source: "s" },
    ]);
    const args = { source: "artifact:art1", extensions };
    resolveBundle.mockResolvedValue([
      { name: "a", type: "sysext", version: "1", source: "s" },
      { name: "b", type: "sysext", version: "1", source: "s" },
    ]);
    getBatch.mockResolvedValue(
      makeBatch({
        args,
        phase: "stopped",
        stoppedAt: "2026-10-23T10:30:00Z",
        stopReason: "x",
        counts: { failed: 2 },
        nodes: [row("n1", "failed"), row("n2", "failed")],
      }),
    );
    sendBulkCommand.mockResolvedValue(makeBatch({ id: "b2" }));
    renderPage();
    fireEvent.click(await screen.findByRole("button", { name: "Start a new batch for the 2 remaining nodes" }));
    const dialog = await screen.findByRole("dialog");
    await waitFor(() => expect(resolveBundle).toHaveBeenCalledWith("art1"));
    await waitFor(() => expect(within(dialog).queryByLabelText("include b")).not.toBeNull());
    expect(within(dialog).getByLabelText("include a")).toBeChecked();
    expect(within(dialog).getByLabelText("include b")).not.toBeChecked();
    fireEvent.click(within(dialog).getByRole("button", { name: "Upgrade 2 nodes" }));
    await waitFor(() => expect(sendBulkCommand).toHaveBeenCalled());
    expect(sendBulkCommand.mock.calls[0][2]).toEqual(args);
  });

  it("shows a stopped batch with a danger badge", async () => {
    getBatch.mockResolvedValue(
      makeBatch({ phase: "stopped", stoppedAt: "2026-10-23T10:30:00Z", stopReason: "x", nodes: [] }),
    );
    renderPage();
    const badge = await screen.findByText("stopped", { selector: "[data-slot=badge], div, span" });
    expect(badge.className).toContain("danger");
  });
});

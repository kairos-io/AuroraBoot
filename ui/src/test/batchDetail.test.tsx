import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";

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
vi.mock("@/api/artifacts", () => ({
  listArtifacts: vi.fn().mockResolvedValue([]),
  resolveBundle: vi.fn().mockResolvedValue([]),
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
      <Routes>
        <Route path="/batches/:id" element={<BatchDetail />} />
        <Route path="/batches/:id/other" element={<div />} />
      </Routes>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
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
      undefined,
    );
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
});

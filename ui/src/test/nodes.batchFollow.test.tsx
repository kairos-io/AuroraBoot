import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, waitFor, fireEvent, act, within } from "@testing-library/react";
import { MemoryRouter } from "react-router";

import { Nodes } from "@/pages/Nodes";
import { Toaster } from "@/components/ui/toaster";

const BATCH_ID = "batch-7";

function node(id: string, hostname: string) {
  return {
    id,
    hostname,
    machineID: `m-${id}`,
    groupID: null,
    labels: {},
    phase: "Online",
    osRelease: null,
    agentVersion: "v2.31.2",
    lastHeartbeat: "2026-10-22T10:00:00Z",
    createdAt: "2026-10-20T10:00:00Z",
    updatedAt: "2026-10-22T10:00:00Z",
  };
}

const nodes = [node("id-milan", "pos-milan-01"), node("id-rome", "pos-rome-01")];

// The rows a fan-out creates, as the server returns them: the stored command
// itself, batch and all.
function created(batchID: string) {
  return nodes.map((n) => ({
    id: `cmd-${n.id}`,
    managedNodeID: n.id,
    command: "reboot",
    args: {},
    phase: "Pending",
    result: "",
    expiresAt: null,
    deliveredAt: null,
    completedAt: null,
    createdAt: "2026-10-22T10:00:00Z",
    batchID,
    failFast: true,
  }));
}

function mockFetch(opts: { batchID?: string } = {}) {
  const batchID = opts.batchID;
  const json = (body: unknown, status = 200) =>
    new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
  const fn = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = typeof input === "string" ? input : input.toString();
    const method = (init?.method ?? "GET").toUpperCase();
    if (url.startsWith("/api/v1/commands/batches/")) {
      return json({
        batchID,
        command: "reboot",
        failFast: true,
        phase: "Pending",
        total: 2,
        pending: 1,
        running: 1,
        completed: 0,
        failed: 0,
        canceled: 0,
        expired: 0,
        commands: created(batchID ?? ""),
      });
    }
    if (url.startsWith("/api/v1/nodes/commands") && method === "POST") {
      return json(batchID ? created(batchID) : []);
    }
    if (url.startsWith("/api/v1/nodes")) return json(nodes);
    return json([]);
  });
  vi.stubGlobal("fetch", fn);
  return fn;
}

async function sendRebootToAll() {
  fireEvent.click(await screen.findByRole("button", { name: "Send Command to 2 nodes" }));
  const dialog = await screen.findByRole("dialog");
  fireEvent.click(within(dialog).getByRole("button", { name: /Reboot/ }));
  await act(async () => {
    fireEvent.click(within(dialog).getByRole("button", { name: "Send reboot" }));
  });
  // No filter and no selection reaches the whole fleet, so it asks first.
  await act(async () => {
    fireEvent.click(await screen.findByRole("button", { name: "Send to all" }));
  });
}

function renderNodes() {
  return render(
    <MemoryRouter>
      <Nodes />
      <Toaster />
    </MemoryRouter>,
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("Nodes follows the rollout it starts", () => {
  it("reads back the batch the bulk command created", async () => {
    const fetchMock = mockFetch({ batchID: BATCH_ID });
    renderNodes();
    expect(await screen.findByText("pos-milan-01")).toBeInTheDocument();

    expect(screen.queryByTestId("batch-progress")).not.toBeInTheDocument();
    await sendRebootToAll();

    await waitFor(() => expect(screen.getByTestId("batch-progress")).toBeInTheDocument());
    expect(
      fetchMock.mock.calls.some((c) => String(c[0]) === `/api/v1/commands/batches/${BATCH_ID}`),
    ).toBe(true);
    await waitFor(() => expect(screen.getByTestId("batch-phase")).toHaveTextContent("In progress"));
  });

  it("can be dismissed once the operator is done with it", async () => {
    mockFetch({ batchID: BATCH_ID });
    renderNodes();
    expect(await screen.findByText("pos-milan-01")).toBeInTheDocument();
    await sendRebootToAll();
    await waitFor(() => expect(screen.getByTestId("batch-progress")).toBeInTheDocument());

    fireEvent.click(screen.getByRole("button", { name: "Dismiss rollout" }));
    await waitFor(() => expect(screen.queryByTestId("batch-progress")).not.toBeInTheDocument());
  });

  it("shows nothing when the response carries no batch", async () => {
    // A server that does not batch, or a fan-out that matched no node, comes
    // back without one. There is no rollout to follow and no empty panel.
    mockFetch();
    renderNodes();
    expect(await screen.findByText("pos-milan-01")).toBeInTheDocument();
    await sendRebootToAll();

    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(screen.queryByTestId("batch-progress")).not.toBeInTheDocument();
  });
});

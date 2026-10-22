import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, waitFor, fireEvent, act, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";

import { GroupDetail } from "@/pages/GroupDetail";
import { Toaster } from "@/components/ui/toaster";

const BATCH_ID = "batch-group-3";

const group = { id: "g1", name: "edge-retail", description: "Shops", node_count: 2, createdAt: "", updatedAt: "" };

function makeNode(id: string, hostname: string) {
  return {
    id,
    hostname,
    machineID: `m-${id}`,
    groupID: "g1",
    group: { id: "g1", name: "edge-retail" },
    labels: {},
    phase: "Online",
    osRelease: { KAIROS_VERSION: "v3.5.0", CPU_COUNT: "4", MEM_TOTAL: "8388608 kB" },
    agentVersion: "v2.31.2",
    lastHeartbeat: "2026-10-22T10:00:00Z",
    createdAt: "2026-10-20T10:00:00Z",
    updatedAt: "2026-10-22T10:00:00Z",
  };
}

const nodes = [makeNode("n1", "pos-milan-01"), makeNode("n2", "pos-rome-01")];

function created() {
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
    batchID: BATCH_ID,
  }));
}

function mockFetch() {
  const json = (body: unknown, status = 200) =>
    new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
  const fn = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = typeof input === "string" ? input : input.toString();
    const method = (init?.method ?? "GET").toUpperCase();
    if (url.startsWith("/api/v1/commands/batches/")) {
      return json({
        batchID: BATCH_ID,
        command: "reboot",
        failFast: false,
        phase: "Pending",
        total: 2,
        pending: 2,
        running: 0,
        completed: 0,
        failed: 0,
        canceled: 0,
        expired: 0,
        commands: created(),
      });
    }
    // The group fan-out endpoint answers with the rows it created.
    if (url === "/api/v1/groups/g1/commands" && method === "POST") return json(created());
    if (method !== "GET") return json([]);
    if (url.startsWith("/api/v1/groups/g1")) return json(group);
    if (url.startsWith("/api/v1/nodes")) return json(nodes);
    return json([]);
  });
  vi.stubGlobal("fetch", fn);
  return fn;
}

function renderPage() {
  return render(
    <MemoryRouter initialEntries={["/groups/g1"]}>
      <Routes>
        <Route path="/groups/:id" element={<GroupDetail />} />
      </Routes>
      <Toaster />
    </MemoryRouter>,
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("GroupDetail follows the rollout it starts", () => {
  it("reads back the batch a group command created", async () => {
    const fetchMock = mockFetch();
    renderPage();
    expect(await screen.findByText("pos-milan-01")).toBeInTheDocument();
    expect(screen.queryByTestId("batch-progress")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /Send command/i }));
    const dialog = await screen.findByRole("dialog");
    fireEvent.click(within(dialog).getByRole("button", { name: /Reboot/ }));
    await act(async () => {
      fireEvent.click(within(dialog).getByRole("button", { name: "Send reboot" }));
    });

    await waitFor(() => expect(screen.getByTestId("batch-progress")).toBeInTheDocument());
    expect(
      fetchMock.mock.calls.some((c) => String(c[0]) === `/api/v1/commands/batches/${BATCH_ID}`),
    ).toBe(true);
    await waitFor(() => expect(screen.getByTestId("batch-phase")).toHaveTextContent("In progress"));
    // The panel lists the batch's own rows, keyed by the node each one targets.
    expect(screen.getByText("n1")).toBeInTheDocument();
    expect(screen.getByText("n2")).toBeInTheDocument();
  });
});

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor, fireEvent, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";

import { GroupDetail } from "@/pages/GroupDetail";
import { Toaster } from "@/components/ui/toaster";
import { setToken } from "@/api/client";

function makeNode(id: string, hostname: string, extra: Record<string, unknown> = {}) {
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
    lastHeartbeat: "2026-09-29T10:00:00Z",
    createdAt: "2026-09-20T10:00:00Z",
    updatedAt: "2026-09-29T10:00:00Z",
    ...extra,
  };
}

const group = { id: "g1", name: "edge-retail", description: "Shops", node_count: 4, createdAt: "", updatedAt: "" };

const nodes = [
  makeNode("n1", "pos-milan-01"),
  makeNode("n2", "pos-milan-02", { bootState: "recovery" }),
  makeNode("n3", "pos-rome-01", { osRelease: { KAIROS_VERSION: "v3.4.0", CPU_COUNT: "2", MEM_TOTAL: "4194304 kB" } }),
  makeNode("n4", "pos-rome-02", { phase: "Offline" }),
];

type Call = { url: string; method: string; body: unknown };

function mockFetch() {
  const calls: Call[] = [];
  const json = (body: unknown, status = 200) =>
    new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
  const fn = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = typeof input === "string" ? input : input.toString();
    const method = (init?.method ?? "GET").toUpperCase();
    if (method !== "GET") {
      const body = init?.body ? JSON.parse(String(init.body)) : undefined;
      calls.push({ url, method, body });
      if (url === "/api/v1/groups/g1" && method === "PUT") return json({ ...group, ...body });
      return json([]);
    }
    if (url.startsWith("/api/v1/groups/g1")) return json(group);
    if (url.startsWith("/api/v1/nodes")) return json(nodes);
    return json([]);
  });
  return { fn, calls };
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

beforeEach(() => {
  setToken("t");
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("Group detail", () => {
  it("shows the summary band", async () => {
    vi.stubGlobal("fetch", mockFetch().fn);
    renderPage();
    const summary = await screen.findByRole("region", { name: "Group summary" });
    await waitFor(() => expect(within(summary).getByText("3 of 4 online")).toBeInTheDocument());
    expect(within(summary).getByTestId("stat-versions")).toHaveTextContent("2");
    expect(within(summary).getByTestId("stat-boot")).toHaveTextContent("1");
    expect(within(summary).getByTestId("stat-capacity")).toHaveTextContent("14 vCPU");
  });

  it("shows an alert row for a node in recovery", async () => {
    vi.stubGlobal("fetch", mockFetch().fn);
    renderPage();
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("pos-milan-02");
    expect(alert).toHaveTextContent(/recovery/i);
    expect(screen.getAllByRole("alert")).toHaveLength(1);
  });

  it("saves name and description through the Edit dialog", async () => {
    const f = mockFetch();
    vi.stubGlobal("fetch", f.fn);
    renderPage();
    await screen.findByRole("heading", { name: "edge-retail" });
    const more = screen.getByRole("button", { name: "More actions" });
    fireEvent.keyDown(more, { key: "Enter" });
    fireEvent.click(await screen.findByRole("menuitem", { name: /Edit name and description/ }));
    const name = await screen.findByLabelText("Name");
    fireEvent.change(name, { target: { value: "edge-shops" } });
    fireEvent.change(screen.getByLabelText("Description"), { target: { value: "All shops" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(f.calls).toContainEqual({
        url: "/api/v1/groups/g1",
        method: "PUT",
        body: { name: "edge-shops", description: "All shops" },
      }),
    );
    expect(await screen.findByRole("heading", { name: "edge-shops" })).toBeInTheDocument();
  });

  it("shows the node table without a Group column and with selection", async () => {
    vi.stubGlobal("fetch", mockFetch().fn);
    renderPage();
    await screen.findByText("pos-milan-01");
    const headers = screen.getAllByRole("columnheader").map((h) => h.textContent?.trim());
    expect(headers).not.toContain("Group");
    expect(screen.getAllByRole("checkbox").length).toBeGreaterThan(1);
  });
});

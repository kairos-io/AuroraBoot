import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router";

import { Dashboard } from "@/pages/Dashboard";
import { setToken } from "@/api/client";

const baseNode = {
  groupID: "",
  labels: {},
  osRelease: null,
  agentVersion: "v2.31.2",
  lastHeartbeat: "2026-09-28T10:00:00Z",
  createdAt: "2026-09-20T10:00:00Z",
  updatedAt: "2026-09-28T10:00:00Z",
};

const nodes = [
  { ...baseNode, id: "n1", hostname: "node-online", machineID: "m1", phase: "Online" },
  { ...baseNode, id: "n2", hostname: "node-offline", machineID: "m2", phase: "Offline" },
  { ...baseNode, id: "n3", hostname: "node-pending", machineID: "m3", phase: "Pending" },
  { ...baseNode, id: "n4", hostname: "node-registered", machineID: "m4", phase: "Registered" },
];

const artifacts = [
  {
    id: "a1",
    name: "build-in-progress",
    phase: "Building",
    createdAt: "2026-09-28T09:00:00Z",
    updatedAt: "2026-09-28T09:00:00Z",
  },
];

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

beforeEach(() => {
  setToken("t");
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = typeof input === "string" ? input : input.toString();
      const path = url.split("?")[0];
      if (path === "/api/v1/nodes") return json(nodes);
      if (path === "/api/v1/artifacts") return json(artifacts);
      if (path === "/api/v1/groups") return json([]);
      if (path === "/api/v1/deployments") return json([]);
      return json([]);
    }),
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function cardOf(title: HTMLElement): HTMLElement {
  return title.parentElement!.parentElement!;
}

describe("Dashboard counts", () => {
  it("shows the running build and lists only the offline node as offline", async () => {
    render(
      <MemoryRouter>
        <Dashboard />
      </MemoryRouter>,
    );

    const buildsTitle = await screen.findByText("Active Builds");
    // Title -> CardHeader -> Card. The activity feed also names the build and
    // the nodes, so each assertion is scoped to its card.
    const buildsCard = cardOf(buildsTitle);
    expect(within(buildsCard).getByText("build-in-progress")).toBeInTheDocument();
    expect(screen.queryByText("No active builds")).not.toBeInTheDocument();

    const offlineTitle = screen.getByText("Offline Nodes");
    const offline = within(cardOf(offlineTitle));
    expect(offline.getByText("node-offline")).toBeInTheDocument();
    expect(offline.queryByText("node-online")).not.toBeInTheDocument();
    expect(offline.queryByText("node-pending")).not.toBeInTheDocument();
    expect(offline.queryByText("node-registered")).not.toBeInTheDocument();

    expect(screen.getByText("waiting")).toBeInTheDocument();
  });
});

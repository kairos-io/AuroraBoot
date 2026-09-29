import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router";

import { Groups } from "@/pages/Groups";
import { setToken } from "@/api/client";

const groups = [
  {
    id: "g1",
    name: "prod",
    description: "",
    node_count: 2,
    createdAt: "2026-09-01T10:00:00Z",
    updatedAt: "2026-09-01T10:00:00Z",
  },
  {
    // An older server sends no node_count; the page shows 0, not a blank.
    id: "g2",
    name: "staging",
    description: "",
    createdAt: "2026-09-01T10:00:00Z",
    updatedAt: "2026-09-01T10:00:00Z",
  },
];

beforeEach(() => {
  setToken("t");
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = typeof input === "string" ? input : input.toString();
      const body = url === "/api/v1/groups" ? groups : [];
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function nodesCell(groupName: string) {
  const row = screen.getByText(groupName).closest("tr");
  expect(row).not.toBeNull();
  const headers = screen.getAllByRole("columnheader").map((h) => h.textContent);
  const idx = headers.indexOf("Nodes");
  expect(idx).toBeGreaterThanOrEqual(0);
  return within(row as HTMLElement).getAllByRole("cell")[idx];
}

describe("Groups node count", () => {
  it("renders node_count in the Nodes column", async () => {
    render(
      <MemoryRouter>
        <Groups />
      </MemoryRouter>,
    );
    await screen.findByText("prod");
    expect(nodesCell("prod")).toHaveTextContent(/^2$/);
    expect(nodesCell("staging")).toHaveTextContent(/^0$/);
  });
});

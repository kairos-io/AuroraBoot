import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";

import { Batches } from "@/pages/Batches";

const listBatches = vi.fn();
vi.mock("@/api/batches", () => ({
  listBatches: (...a: unknown[]) => listBatches(...a),
}));

describe("Batches", () => {
  it("lists batches with their non-zero counts and links to them", async () => {
    listBatches.mockResolvedValue({
      total: 1,
      batches: [
        {
          id: "b1",
          command: "upgrade",
          args: {},
          selection: "group edge",
          groupID: "g1",
          concurrency: 2,
          stopOnFailure: true,
          rebootTimeoutSeconds: 600,
          phase: "stopped",
          createdAt: "2026-10-23T10:00:00Z",
          finishedAt: null,
          stoppedAt: null,
          stopReason: "",
          stoppedByNodeID: null,
          counts: { done: 3, failed: 1, waiting: 0 },
        },
      ],
    });
    render(
      <MemoryRouter>
        <Batches />
      </MemoryRouter>,
    );
    expect(await screen.findByText("3 done, 1 failed")).toBeInTheDocument();
    expect(screen.getByText("group edge")).toBeInTheDocument();
    expect(screen.getByText("stopped")).toBeInTheDocument();
    const link = screen.getByRole("link", { name: /Upgrade/ });
    expect(link).toHaveAttribute("href", "/batches/b1");
    expect(listBatches).toHaveBeenCalledWith({ limit: 50, offset: 0 });
  });
});

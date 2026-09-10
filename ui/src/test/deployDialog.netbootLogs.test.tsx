import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router";

vi.mock("@/api/deployments", async () => {
  const actual = await vi.importActual<typeof import("@/api/deployments")>(
    "@/api/deployments",
  );
  return {
    ...actual,
    getNetbootStatus: vi.fn(async () => ({
      running: true,
      artifactId: "artifact-1",
      address: "0.0.0.0",
      port: "8090",
    })),
    getNetbootLogs: vi.fn(async () => "dhcp: server started\n"),
    startNetboot: vi.fn(async () => ({})),
    stopNetboot: vi.fn(async () => ({})),
    listBMCTargets: vi.fn(async () => []),
  };
});

vi.mock("@/api/redfish", () => ({
  listQuirkProfiles: vi.fn(async () => []),
}));

// Captures the onMessage callback so the test can simulate a live WS chunk
// without standing up a real WebSocket (jsdom has none by default).
let wsOnMessage: ((msg: { type: string; data: unknown }) => void) | null = null;
vi.mock("@/hooks/useUIWebSocket", () => ({
  useUIWebSocket: (onMessage: (msg: { type: string; data: unknown }) => void) => {
    wsOnMessage = onMessage;
    return { connected: true };
  },
}));

import { DeployDialog } from "@/components/DeployDialog";

beforeEach(() => {
  wsOnMessage = null;
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  });
});

describe("DeployDialog: netboot log pane", () => {
  it("shows the snapshot log and appends live WS chunks", async () => {
    render(
      <MemoryRouter>
        <DeployDialog
          artifactId="artifact-1"
          artifactFiles={["kairos.squashfs"]}
          hasNetboot={true}
          onClose={() => {}}
        />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByText(/dhcp: server started/)).toBeInTheDocument();
    });

    expect(wsOnMessage).not.toBeNull();
    wsOnMessage!({ type: "netboot-log", data: { chunk: "tftp: sent kairos-kernel\n" } });

    await waitFor(() => {
      expect(screen.getByText(/tftp: sent kairos-kernel/)).toBeInTheDocument();
    });
    // The snapshot line is still there — live chunks append, they don't replace.
    expect(screen.getByText(/dhcp: server started/)).toBeInTheDocument();
  });

  it("ignores WS messages of a different type", async () => {
    render(
      <MemoryRouter>
        <DeployDialog
          artifactId="artifact-1"
          artifactFiles={["kairos.squashfs"]}
          hasNetboot={true}
          onClose={() => {}}
        />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByText(/dhcp: server started/)).toBeInTheDocument();
    });

    wsOnMessage!({ type: "build-log", data: { id: "x", chunk: "unrelated\n" } });
    expect(screen.queryByText(/unrelated/)).not.toBeInTheDocument();
  });
});

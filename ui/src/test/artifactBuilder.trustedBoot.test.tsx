import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router";

vi.mock("@/api/artifacts", async () => {
  const actual = await vi.importActual<typeof import("@/api/artifacts")>(
    "@/api/artifacts",
  );
  return {
    ...actual,
    listSecureBootKeySets: vi.fn(async () => [
      {
        id: "ks-1",
        name: "prod-keys",
        hasTpmPcrKey: true,
        secureBootEnroll: "if-safe",
        createdAt: "2026-01-01T00:00:00Z",
      },
    ]),
    getArtifact: vi.fn(),
    listBundleExtensions: vi.fn(async () => []),
    createArtifact: vi.fn(async () => ({
      id: "build-1",
      phase: "Pending",
      message: "",
      artifacts: [],
    })),
  };
});

vi.mock("@/api/groups", () => ({
  listGroups: vi.fn(async () => []),
}));

import { ArtifactBuilder } from "@/pages/ArtifactBuilder";
import { Toaster } from "@/components/ui/toaster";
import { createArtifact, getArtifact } from "@/api/artifacts";

beforeEach(() => {
  vi.mocked(createArtifact).mockClear();
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
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response("[]", { status: 200 })),
  );
});

function renderBuilder(initialEntry: string) {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <Toaster />
      <Routes>
        <Route path="/artifacts/new" element={<ArtifactBuilder />} />
      </Routes>
    </MemoryRouter>,
  );
}

function source(overrides: Record<string, unknown> = {}) {
  return {
    id: "src-artifact-id",
    name: "edge-gateway",
    phase: "Ready",
    baseImage: "ghcr.io/kairos-io/ubuntu:24.04",
    kairosVersion: "v3.5.0",
    model: "generic",
    arch: "amd64",
    variant: "core",
    iso: true,
    cloudImage: false,
    netboot: false,
    uki: false,
    fips: false,
    trustedBoot: false,
    artifacts: [],
    ...overrides,
  };
}

async function openClone(overrides: Record<string, unknown> = {}) {
  vi.mocked(getArtifact).mockResolvedValue(source(overrides) as never);
  renderBuilder("/artifacts/new?clone=src-artifact-id");
  await waitFor(() => {
    expect(screen.getByText(/Clone: edge-gateway/)).toBeTruthy();
  });
}

function outputCard(label: string): HTMLButtonElement {
  return screen.getByRole("button", { name: new RegExp(`^${label}\\b`) }) as HTMLButtonElement;
}

describe("ArtifactBuilder: Trusted Boot outputs", () => {
  it("unticks and disables ISO and Netboot when Trusted Boot is turned on", async () => {
    await openClone({ netboot: true });
    fireEvent.click(screen.getByRole("button", { name: /^Access/ }));
    fireEvent.click(await screen.findByRole("checkbox", { name: /Trusted Boot/ }));

    fireEvent.click(screen.getByRole("button", { name: /^Output/ }));
    await screen.findByText(/ISO and Netboot are not available/);

    expect(outputCard("ISO").disabled).toBe(true);
    expect(outputCard("Netboot").disabled).toBe(true);
    expect(outputCard("UKI").disabled).toBe(false);
  });

  it("keeps ISO available without Trusted Boot, UKI included", async () => {
    await openClone({ uki: true });
    fireEvent.click(screen.getByRole("button", { name: /^Output/ }));
    await screen.findByText(/UKI requires secure boot signing keys/);

    expect(outputCard("ISO").disabled).toBe(false);
    expect(outputCard("Netboot").disabled).toBe(false);
    expect(screen.queryByText(/ISO and Netboot are not available/)).toBeNull();
  });

  it("refuses to start a cloned Trusted Boot build that still asks for an ISO", async () => {
    await openClone({ trustedBoot: true });
    fireEvent.click(screen.getByRole("button", { name: /^Review/ }));
    fireEvent.click(await screen.findByRole("button", { name: /Start Build/i }));

    await screen.findAllByText(/ISO and Netboot are not available with Trusted Boot/);
    expect(createArtifact).not.toHaveBeenCalled();
  });
});

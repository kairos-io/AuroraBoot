import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router";

vi.mock("@/api/artifacts", async () => {
  const actual = await vi.importActual<typeof import("@/api/artifacts")>(
    "@/api/artifacts",
  );
  return {
    ...actual,
    listSecureBootKeySets: vi.fn(async () => []),
    getArtifact: vi.fn(),
    listBundleExtensions: vi.fn(async () => []),
    createArtifact: vi.fn(),
    uploadOverlayFiles: vi.fn(),
  };
});

vi.mock("@/api/groups", () => ({
  listGroups: vi.fn(async () => []),
}));

import { ArtifactBuilder } from "@/pages/ArtifactBuilder";
import { Toaster } from "@/components/ui/toaster";
import { ApiError } from "@/api/client";
import { createArtifact, getArtifact } from "@/api/artifacts";

const REJECTION = 'bundled extension "tools" is not Ready (phase Building)';

beforeEach(() => {
  vi.mocked(createArtifact).mockReset();
  vi.mocked(getArtifact).mockResolvedValue({
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
  } as never);
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

// A clone has every step unlocked, so the stepper can jump to Review.
async function openReview() {
  render(
    <MemoryRouter initialEntries={["/artifacts/new?clone=src-artifact-id"]}>
      <Toaster />
      <Routes>
        <Route path="/artifacts/new" element={<ArtifactBuilder />} />
        <Route path="/artifacts/:id" element={<p>artifact detail page</p>} />
      </Routes>
    </MemoryRouter>,
  );
  await screen.findByText(/Clone: edge-gateway/);
  fireEvent.click(screen.getByRole("button", { name: /^Review/ }));
  return screen.findByRole("button", { name: /Start Build/i });
}

describe("ArtifactBuilder: Start build", () => {
  it("shows the server's reason when the build is rejected", async () => {
    vi.mocked(createArtifact).mockRejectedValue(
      new ApiError(400, { error: REJECTION }, JSON.stringify({ error: REJECTION })),
    );
    const startBuild = await openReview();

    fireEvent.click(startBuild);

    expect(await screen.findByText(REJECTION)).toBeTruthy();
    expect(
      await screen.findByText(`Could not start the build: ${REJECTION}`),
    ).toBeTruthy();
    expect(createArtifact).toHaveBeenCalledTimes(1);
    expect(screen.queryByText("artifact detail page")).toBeNull();
    const retry = screen.getByRole("button", { name: /Start Build/i }) as HTMLButtonElement;
    expect(retry.disabled).toBe(false);
  });

  it("clears the reason when the operator tries again", async () => {
    vi.mocked(createArtifact)
      .mockRejectedValueOnce(new ApiError(400, { error: REJECTION }, ""))
      .mockReturnValueOnce(new Promise(() => {}));
    const startBuild = await openReview();

    fireEvent.click(startBuild);
    await screen.findByText(REJECTION);

    fireEvent.click(screen.getByRole("button", { name: /Start Build/i }));

    await waitFor(() => expect(screen.queryByText(REJECTION)).toBeNull());
    expect(createArtifact).toHaveBeenCalledTimes(2);
  });

  it("goes to the new build when the server accepts it", async () => {
    vi.mocked(createArtifact).mockResolvedValue({
      id: "build-1",
      phase: "Pending",
      message: "",
      artifacts: [],
    } as never);
    const startBuild = await openReview();

    fireEvent.click(startBuild);

    expect(await screen.findByText("artifact detail page")).toBeTruthy();
    await waitFor(() => expect(createArtifact).toHaveBeenCalledTimes(1));
  });
});

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
    createArtifact: vi.fn(async () => ({ id: "build-1", phase: "Pending", message: "", artifacts: [] })),
    uploadOverlayFiles: vi.fn(),
  };
});

vi.mock("@/api/groups", () => ({
  listGroups: vi.fn(async () => []),
}));

import { ArtifactBuilder } from "@/pages/ArtifactBuilder";
import {
  catalogExtensionsForArch,
  serializeExtensionSelection,
  parseExtensionSelection,
} from "@/lib/catalogExtensions";
import { createArtifact } from "@/api/artifacts";

const CATALOG_URL = "https://kairos-io.github.io/hadron-layers/releases.json";

// Shaped like the published hadron-layers index: a per-version `sysext` map
// keyed by architecture is what decides whether an extension can be
// materialized for the build at all.
const CATALOG = {
  repo: "ghcr.io/kairos-io/hadron-layers",
  layers: [
    {
      name: "nvidia",
      description: "NVIDIA drivers",
      latest: "v2.0.0",
      tags: [
        { tag: "v1.0.0", sysext: { amd64: { oci: "ghcr.io/x/nvidia@sha256:aa" } } },
        { tag: "v2.0.0", sysext: { amd64: { oci: "ghcr.io/x/nvidia@sha256:bb" } } },
      ],
    },
    {
      name: "rpi-firmware",
      description: "Raspberry Pi firmware",
      latest: "v1.0.0",
      tags: [{ tag: "v1.0.0", sysext: { arm64: { oci: "ghcr.io/x/rpi@sha256:cc" } } }],
    },
  ],
};

beforeEach(() => {
  vi.clearAllMocks();
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
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes("releases.json")) {
        return new Response(JSON.stringify(CATALOG), { status: 200 });
      }
      return new Response("[]", { status: 200 });
    }),
  );
});

function renderBuilder() {
  return render(
    <MemoryRouter initialEntries={["/artifacts/new"]}>
      <Routes>
        <Route path="/artifacts/new" element={<ArtifactBuilder />} />
      </Routes>
    </MemoryRouter>,
  );
}

async function gotoOutputStep() {
  fireEvent.click(await screen.findByText(/^Hadron v/));
  fireEvent.click(screen.getByRole("button", { name: /Output/i }));
}

describe("catalog extension helpers", () => {
  it("keeps only the versions published for the target architecture", () => {
    const items = [
      {
        name: "nvidia",
        versions: [
          { version: "v1", archs: ["amd64"] },
          { version: "v2", archs: ["amd64", "arm64"] },
        ],
      },
      { name: "rpi", versions: [{ version: "v1", archs: ["arm64"] }] },
    ];

    const amd64 = catalogExtensionsForArch(items, "amd64");
    expect(amd64.map((i) => i.name)).toEqual(["nvidia"]);
    expect(amd64[0].versions.map((v) => v.version)).toEqual(["v1", "v2"]);

    const arm64 = catalogExtensionsForArch(items, "arm64");
    expect(arm64.map((i) => i.name)).toEqual(["nvidia", "rpi"]);
    expect(arm64[0].versions.map((v) => v.version)).toEqual(["v2"]);

    expect(catalogExtensionsForArch(items, "riscv64")).toEqual([]);
  });

  it("serializes a pinned version and leaves a latest pick as the bare name", () => {
    const selection = parseExtensionSelection(["nvidia", "tailscale@v1.2.3"]);
    expect(serializeExtensionSelection(selection, ["nvidia", "tailscale"])).toEqual([
      "nvidia",
      "tailscale@v1.2.3",
    ]);
  });

  it("round-trips through parse and serialize", () => {
    const values = ["nvidia", "tailscale@v1.2.3"];
    const selection = parseExtensionSelection(values);
    expect(serializeExtensionSelection(selection, Object.keys(selection).sort())).toEqual(values);
  });
});

describe("ArtifactBuilder: catalog extension picker", () => {
  it("reads the default catalog and offers only what it publishes for the selected arch", async () => {
    renderBuilder();
    await gotoOutputStep();

    expect(await screen.findByText("nvidia")).toBeInTheDocument();
    // The Hadron template builds amd64, and rpi-firmware publishes arm64
    // only, so offering it would produce a build that cannot resolve it.
    expect(screen.queryByText("rpi-firmware")).not.toBeInTheDocument();

    const calls = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls.map((c) =>
      String(c[0]),
    );
    expect(calls).toContain(CATALOG_URL);
  });

  it("sends the selection with no catalog override when the default catalog is used", async () => {
    renderBuilder();
    await gotoOutputStep();

    fireEvent.click(await screen.findByLabelText("nvidia"));
    fireEvent.click(screen.getByRole("button", { name: /Review/i }));
    fireEvent.click(await screen.findByRole("button", { name: /Start Build/i }));

    await waitFor(() => expect(createArtifact).toHaveBeenCalled());
    const input = (createArtifact as unknown as ReturnType<typeof vi.fn>).mock.calls[0][0];
    expect(input.extensions).toEqual(["nvidia"]);
    // The server owns the default catalog URL, so a build that did not
    // override it must not pin today's value.
    expect(input.extensionsCatalogs).toBeUndefined();
  });

  it("sends the catalog when the operator points the build at another one", async () => {
    renderBuilder();
    await gotoOutputStep();

    fireEvent.click(await screen.findByLabelText("nvidia"));
    fireEvent.change(screen.getByLabelText(/Extension catalog URL/i), {
      target: { value: "https://example.test/mine/releases.json" },
    });
    fireEvent.click(screen.getByRole("button", { name: /Review/i }));
    fireEvent.click(await screen.findByRole("button", { name: /Start Build/i }));

    await waitFor(() => expect(createArtifact).toHaveBeenCalled());
    const input = (createArtifact as unknown as ReturnType<typeof vi.fn>).mock.calls[0][0];
    expect(input.extensions).toEqual(["nvidia"]);
    expect(input.extensionsCatalogs).toEqual(["https://example.test/mine/releases.json"]);
  });

  it("sends nothing when no extension is selected", async () => {
    renderBuilder();
    await gotoOutputStep();
    await screen.findByText("nvidia");

    fireEvent.click(screen.getByRole("button", { name: /Review/i }));
    fireEvent.click(await screen.findByRole("button", { name: /Start Build/i }));

    await waitFor(() => expect(createArtifact).toHaveBeenCalled());
    const input = (createArtifact as unknown as ReturnType<typeof vi.fn>).mock.calls[0][0];
    expect(input.extensions).toBeUndefined();
    expect(input.extensionsCatalogs).toBeUndefined();
  });

  it("reports a catalog it cannot read instead of showing an empty picker", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        if (String(input).includes("releases.json")) {
          return new Response("nope", { status: 404 });
        }
        return new Response("[]", { status: 200 });
      }),
    );

    renderBuilder();
    await gotoOutputStep();

    expect(await screen.findByText(/Could not read that catalog/i)).toBeInTheDocument();
  });
});

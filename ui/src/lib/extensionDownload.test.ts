import { describe, it, expect, afterEach } from "vitest";

import { extensionDownloadUrl } from "@/api/extensions";
import {
  PHONEHOME_ALL_COMMANDS,
  PHONEHOME_DESTRUCTIVE_COMMANDS,
  PHONEHOME_SAFE_DEFAULTS,
} from "./buildConfig";

const ADMIN = "s3cr3t-admin-pass";

afterEach(() => {
  localStorage.removeItem("auroraboot_token");
});

describe("extensionDownloadUrl", () => {
  it("appends the admin token as a query param", () => {
    localStorage.setItem("auroraboot_token", ADMIN);
    expect(extensionDownloadUrl("e-1", "x.raw")).toBe(
      `/api/v1/extensions/e-1/download/x.raw?token=${encodeURIComponent(ADMIN)}`,
    );
  });

  it("encodes the id and filename so neither can inject a path or query separator", () => {
    expect(extensionDownloadUrl("../e-1", "a b?x=1.raw")).toBe(
      "/api/v1/extensions/..%2Fe-1/download/a%20b%3Fx%3D1.raw?token=",
    );
  });
});

describe("phonehome command catalogue", () => {
  // The Install Extension dialog sends the `extension` command; the agent
  // refuses any command absent from the baked phonehome.allowed_commands, and
  // an operator can only tick what this catalogue renders. Without the entry
  // the whole feature is unreachable on a provisioned node.
  it("offers `extension`, in the destructive set", () => {
    expect(PHONEHOME_ALL_COMMANDS).toContain("extension");
    expect(PHONEHOME_DESTRUCTIVE_COMMANDS).toContain("extension");
    expect(PHONEHOME_SAFE_DEFAULTS).not.toContain("extension");
  });
});

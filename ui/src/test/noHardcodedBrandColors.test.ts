import { describe, it, expect } from "vitest";
import fs from "node:fs";
import path from "node:path";

// Files that must use design tokens instead of hard-coded brand colors.
const CHECKED = [
  "src/pages/ArtifactBuilder.tsx",
  "src/pages/ArtifactDetail.tsx",
  "src/pages/Artifacts.tsx",
  "src/pages/ExtensionBuilder.tsx",
  "src/pages/ExtensionDetail.tsx",
  "src/components/InstallExtensionDialog.tsx",
  "src/components/DeployDialog.tsx",
];

const BRAND_HEX = /#(EE5007|FF7442|C73F00|03153A|FFB380|051d52)\b/i;

describe("no hard-coded brand colors", () => {
  it("uses design tokens instead of brand hex literals", () => {
    const root = path.resolve(__dirname, "..", "..");
    const hits: string[] = [];
    for (const file of CHECKED) {
      const lines = fs.readFileSync(path.join(root, file), "utf8").split("\n");
      lines.forEach((line, i) => {
        if (BRAND_HEX.test(line)) hits.push(`${file}:${i + 1}`);
      });
    }
    expect(hits, `hard-coded brand colors found:\n${hits.join("\n")}`).toEqual([]);
  });
});

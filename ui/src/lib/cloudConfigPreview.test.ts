import { describe, it, expect } from "vitest";
import { parse } from "yaml";

import { buildCloudConfigPreview } from "./cloudConfigPreview";
import { PHONEHOME_SAFE_DEFAULTS } from "./buildConfig";

const base: Parameters<typeof buildCloudConfigPreview>[0] = {
  autoInstall: true,
  registerAuroraBoot: true,
  groupName: "production",
  allowedCommands: [...PHONEHOME_SAFE_DEFAULTS],
  variant: "standard",
  kubernetesDistro: "k3s",
  kubernetesEnabled: true,
  userMode: "default",
  username: "",
  password: "",
  sshKeys: "",
  extraYAML: "",
};

function docBody(yaml: string): Record<string, unknown> {
  const body = yaml.replace(/^#cloud-config\n?/, "");
  const parsed = parse(body);
  expect(parsed).toBeTypeOf("object");
  return parsed as Record<string, unknown>;
}

describe("buildCloudConfigPreview", () => {
  it("emits k3s.enabled for standard variant", () => {
    const doc = docBody(buildCloudConfigPreview(base));
    expect(doc.k3s).toEqual({ enabled: true });
  });

  it("merges extra k3s config instead of duplicating the top-level key", () => {
    const yaml = buildCloudConfigPreview({
      ...base,
      extraYAML: "k3s:\n  enabled: true\n  cluster-cidr: 10.42.0.0/16",
    });
    expect(yaml.match(/^k3s:/gm)?.length ?? 0).toBeLessThanOrEqual(1);

    const doc = docBody(yaml);
    expect(doc.k3s).toEqual({
      enabled: true,
      "cluster-cidr": "10.42.0.0/16",
    });
  });

  it("lets extra YAML override kubernetes enabled when present", () => {
    const doc = docBody(
      buildCloudConfigPreview({
        ...base,
        kubernetesEnabled: true,
        extraYAML: "k3s:\n  enabled: false",
      }),
    );
    expect(doc.k3s).toEqual({ enabled: false });
  });

  it("omits k3s for core variant", () => {
    const doc = docBody(
      buildCloudConfigPreview({
        ...base,
        variant: "core",
        kubernetesDistro: "k3s",
      }),
    );
    expect(doc.k3s).toBeUndefined();
  });

  it("merges extra stages under the canonical stages key", () => {
    const doc = docBody(
      buildCloudConfigPreview({
        ...base,
        extraYAML: "stages:\n  boot:\n    - commands:\n        - echo hi",
      }),
    );
    const stages = doc.stages as Record<string, unknown>;
    expect(stages.initramfs).toBeDefined();
    expect(stages.boot).toBeDefined();
  });
});

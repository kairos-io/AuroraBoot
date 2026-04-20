import { describe, it, expect } from "vitest";

import {
  BUILD_CONFIG_KIND,
  BUILD_CONFIG_VERSION,
  PHONEHOME_SAFE_DEFAULTS,
  payloadFromArtifact,
  payloadFromBuilder,
} from "./buildConfig";
import type { Artifact, CreateArtifactInput, SecureBootKeySet } from "@/api/artifacts";
import type { Group } from "@/api/groups";

// Helper that builds a valid CreateArtifactInput with sane defaults so each
// test only has to override the fields it actually cares about.
function makeForm(overrides: Partial<CreateArtifactInput> = {}): CreateArtifactInput {
  return {
    name: "My Build",
    baseImage: "ubuntu:24.04",
    kairosVersion: "v4.0.3",
    arch: "amd64",
    model: "generic",
    variant: "core",
    kubernetesDistro: "",
    kubernetesVersion: "",
    dockerfile: "",
    overlayRootfs: "",
    kairosInitImage: "",
    outputs: {
      iso: true,
      cloudImage: false,
      netboot: false,
      rawDisk: false,
      tar: false,
      gce: false,
      vhd: false,
      uki: false,
      fips: false,
      trustedBoot: false,
    },
    signing: {
      ukiKeySetId: "",
      ukiSecureBootKey: "",
      ukiSecureBootCert: "",
      ukiTpmPcrKey: "",
      ukiPublicKeysDir: "",
      ukiSecureBootEnroll: "if-safe",
    },
    provisioning: {
      autoInstall: true,
      registerAuroraBoot: true,
      targetGroupId: "",
      allowedCommands: [...PHONEHOME_SAFE_DEFAULTS],
    },
    cloudConfig: "",
    ...overrides,
  };
}

function makeArtifact(overrides: Partial<Artifact> = {}): Artifact {
  return {
    id: "art-1",
    name: "Built Artifact",
    phase: "Ready",
    message: "",
    baseImage: "ubuntu:24.04",
    kairosVersion: "v4.0.3",
    model: "generic",
    arch: "amd64",
    variant: "core",
    iso: true,
    cloudImage: false,
    netboot: false,
    rawDisk: false,
    tar: false,
    gce: false,
    vhd: false,
    uki: false,
    fips: false,
    trustedBoot: false,
    autoInstall: true,
    registerAuroraBoot: true,
    artifacts: [],
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

const groups: Group[] = [
  { id: "grp-1", name: "production", description: "", node_count: 0 },
  { id: "grp-2", name: "staging", description: "", node_count: 0 },
];

const keySets: SecureBootKeySet[] = [
  {
    id: "ks-1",
    name: "prod-keys",
    keysDir: "/data/keys/prod-keys",
    tpmPcrKeyPath: "/data/keys/prod-keys/tpm.pem",
    secureBootEnroll: "if-safe",
    createdAt: "2026-01-01T00:00:00Z",
  },
];

describe("payloadFromBuilder", () => {
  it("produces a versioned envelope with kind/version/timestamp", () => {
    const p = payloadFromBuilder({
      form: makeForm(),
      buildMode: "image",
      groups,
      keySets,
      userMode: "default",
      username: "",
      sshKeys: "",
      advancedConfig: "",
    });

    expect(p.kind).toBe(BUILD_CONFIG_KIND);
    expect(p.version).toBe(BUILD_CONFIG_VERSION);
    expect(p.exportedAt).toMatch(/^\d{4}-\d{2}-\d{2}T/);
  });

  it("omits Kubernetes fields on the core variant", () => {
    const p = payloadFromBuilder({
      form: makeForm({
        variant: "core",
        kubernetesDistro: "k3s",
        kubernetesVersion: "v1.28.0",
      }),
      buildMode: "image",
      groups,
      keySets,
      userMode: "default",
      username: "",
      sshKeys: "",
      advancedConfig: "",
    });

    expect(p.source.kubernetesDistro).toBeUndefined();
    expect(p.source.kubernetesVersion).toBeUndefined();
  });

  it("preserves Kubernetes fields on the standard variant", () => {
    const p = payloadFromBuilder({
      form: makeForm({
        variant: "standard",
        kubernetesDistro: "k3s",
        kubernetesVersion: "v1.28.0",
      }),
      buildMode: "image",
      groups,
      keySets,
      userMode: "default",
      username: "",
      sshKeys: "",
      advancedConfig: "",
    });

    expect(p.source.kubernetesDistro).toBe("k3s");
    expect(p.source.kubernetesVersion).toBe("v1.28.0");
  });

  it("resolves group + key set to names for portability", () => {
    const p = payloadFromBuilder({
      form: makeForm({
        provisioning: {
          autoInstall: true,
          registerAuroraBoot: true,
          targetGroupId: "grp-2",
        },
        signing: {
          ukiKeySetId: "ks-1",
          ukiSecureBootKey: "",
          ukiSecureBootCert: "",
          ukiTpmPcrKey: "",
          ukiPublicKeysDir: "",
          ukiSecureBootEnroll: "if-safe",
        },
      }),
      buildMode: "image",
      groups,
      keySets,
      userMode: "default",
      username: "",
      sshKeys: "",
      advancedConfig: "",
    });

    expect(p.provisioning.targetGroupName).toBe("staging");
    expect(p.signing.ukiKeySetName).toBe("prod-keys");
  });

  it("never serializes manual UKI key paths or passwords", () => {
    const p = payloadFromBuilder({
      form: makeForm({
        signing: {
          ukiKeySetId: "",
          ukiSecureBootKey: "/instance/local/db.key",
          ukiSecureBootCert: "/instance/local/db.pem",
          ukiTpmPcrKey: "/instance/local/tpm.pem",
          ukiPublicKeysDir: "/instance/local/keys",
          ukiSecureBootEnroll: "if-safe",
        },
      }),
      buildMode: "image",
      groups,
      keySets,
      userMode: "custom",
      username: "alice",
      sshKeys: "",
      advancedConfig: "",
    });

    // A JSON round-trip is the surest way to prove nothing leaked.
    const serialized = JSON.stringify(p);
    expect(serialized).not.toContain("/instance/local");
    expect(p.signing).toEqual({
      ukiSecureBootEnroll: "if-safe",
      // no ukiKeySetName (none selected), no raw paths
    });
    // No "password" key should appear at all.
    expect(serialized).not.toMatch(/"password"/);
  });

  it("includes username only when userMode=custom", () => {
    const base = {
      form: makeForm(),
      buildMode: "image" as const,
      groups,
      keySets,
      username: "alice",
      sshKeys: "",
      advancedConfig: "",
    };

    expect(payloadFromBuilder({ ...base, userMode: "default" }).provisioning.username).toBeUndefined();
    expect(payloadFromBuilder({ ...base, userMode: "none" }).provisioning.username).toBeUndefined();
    expect(payloadFromBuilder({ ...base, userMode: "custom" }).provisioning.username).toBe("alice");
  });

  it("only includes ssh keys when a user will actually be created", () => {
    const base = {
      form: makeForm(),
      buildMode: "image" as const,
      groups,
      keySets,
      username: "",
      sshKeys: "ssh-rsa AAA user@host",
      advancedConfig: "",
    };

    expect(payloadFromBuilder({ ...base, userMode: "default" }).provisioning.sshKeys).toBe("ssh-rsa AAA user@host");
    expect(payloadFromBuilder({ ...base, userMode: "none" }).provisioning.sshKeys).toBeUndefined();
  });

  // phonehome.allowed_commands is the opt-in gate on destructive remote
  // commands. The UI is authoritative: whatever is in form state must ride
  // through to the payload verbatim (no "omit-when-default" compression).
  it("exports form.provisioning.allowedCommands verbatim", () => {
    const p = payloadFromBuilder({
      form: makeForm({
        provisioning: {
          autoInstall: true,
          registerAuroraBoot: true,
          targetGroupId: "",
          allowedCommands: ["exec", "reboot"],
        },
      }),
      buildMode: "image",
      groups,
      keySets,
      userMode: "default",
      username: "",
      sshKeys: "",
      advancedConfig: "",
    });
    expect(p.provisioning.allowedCommands).toEqual(["exec", "reboot"]);
  });

  it("exports the safe defaults when the form hasn't customized them", () => {
    const p = payloadFromBuilder({
      form: makeForm({
        provisioning: {
          autoInstall: true,
          registerAuroraBoot: true,
          targetGroupId: "",
          allowedCommands: [...PHONEHOME_SAFE_DEFAULTS],
        },
      }),
      buildMode: "image",
      groups,
      keySets,
      userMode: "default",
      username: "",
      sshKeys: "",
      advancedConfig: "",
    });
    expect(p.provisioning.allowedCommands).toEqual([...PHONEHOME_SAFE_DEFAULTS]);
  });

  it("preserves an empty allowedCommands list (observe-only)", () => {
    const p = payloadFromBuilder({
      form: makeForm({
        provisioning: {
          autoInstall: true,
          registerAuroraBoot: true,
          targetGroupId: "",
          allowedCommands: [],
        },
      }),
      buildMode: "image",
      groups,
      keySets,
      userMode: "default",
      username: "",
      sshKeys: "",
      advancedConfig: "",
    });
    expect(p.provisioning.allowedCommands).toEqual([]);
  });

  it("falls back to safe defaults when form state omits allowedCommands", () => {
    // Older in-memory form state (e.g. hydrated from a legacy import path)
    // may not carry the field — the export helper should substitute the
    // safe defaults rather than emitting undefined.
    const form = makeForm();
    // Intentionally strip the field the makeForm default set.
    delete (form.provisioning as { allowedCommands?: string[] }).allowedCommands;
    const p = payloadFromBuilder({
      form,
      buildMode: "image",
      groups,
      keySets,
      userMode: "default",
      username: "",
      sshKeys: "",
      advancedConfig: "",
    });
    expect(p.provisioning.allowedCommands).toEqual([...PHONEHOME_SAFE_DEFAULTS]);
  });

  it("exposes the dockerfile payload only in dockerfile build mode", () => {
    const withDocker = payloadFromBuilder({
      form: makeForm({ dockerfile: "FROM scratch" }),
      buildMode: "dockerfile",
      groups,
      keySets,
      userMode: "default",
      username: "",
      sshKeys: "",
      advancedConfig: "",
    });
    expect(withDocker.dockerfile).toBe("FROM scratch");

    const withoutDocker = payloadFromBuilder({
      form: makeForm({ dockerfile: "FROM scratch" }),
      buildMode: "image",
      groups,
      keySets,
      userMode: "default",
      username: "",
      sshKeys: "",
      advancedConfig: "",
    });
    expect(withoutDocker.dockerfile).toBeUndefined();
  });
});

describe("payloadFromArtifact", () => {
  it("maps a built artifact back to the same envelope shape", () => {
    const p = payloadFromArtifact(
      makeArtifact({
        name: "hadron-prod",
        baseImage: "quay.io/kairos/hadron:v0.0.4",
        kairosVersion: "v4.0.3",
        arch: "arm64",
        model: "rpi4",
        variant: "standard",
        kubernetesDistro: "k3s",
        kubernetesVersion: "v1.28.0",
        targetGroupId: "grp-1",
        uki: true,
        iso: true,
      }),
      groups,
    );

    expect(p.kind).toBe(BUILD_CONFIG_KIND);
    expect(p.version).toBe(BUILD_CONFIG_VERSION);
    expect(p.name).toBe("hadron-prod");
    expect(p.source.arch).toBe("arm64");
    expect(p.source.model).toBe("rpi4");
    expect(p.source.variant).toBe("standard");
    expect(p.source.kubernetesDistro).toBe("k3s");
    expect(p.outputs.uki).toBe(true);
    expect(p.outputs.iso).toBe(true);
    expect(p.provisioning.targetGroupName).toBe("production");
    // Signing is intentionally empty — the artifact record doesn't carry it.
    expect(p.signing).toEqual({});
  });

  it("defaults arch/model/variant when the record has them missing", () => {
    const p = payloadFromArtifact(
      makeArtifact({ arch: undefined, model: "", variant: undefined }),
      groups,
    );
    expect(p.source.arch).toBe("amd64");
    expect(p.source.model).toBe("generic");
    expect(p.source.variant).toBe("core");
  });

  it("switches to dockerfile build mode when a dockerfile is present", () => {
    const p = payloadFromArtifact(
      makeArtifact({ dockerfile: "FROM quay.io/kairos/hadron:latest" }),
      groups,
    );
    expect(p.buildMode).toBe("dockerfile");
    expect(p.dockerfile).toBe("FROM quay.io/kairos/hadron:latest");
  });

  it("leaves targetGroupName empty when the artifact's group id is unknown", () => {
    const p = payloadFromArtifact(
      makeArtifact({ targetGroupId: "deleted-group" }),
      groups,
    );
    expect(p.provisioning.targetGroupName).toBeUndefined();
  });

  it("round-trips through JSON without throwing", () => {
    const p = payloadFromArtifact(makeArtifact(), groups);
    const serialized = JSON.stringify(p);
    const reparsed = JSON.parse(serialized);
    expect(reparsed.kind).toBe(BUILD_CONFIG_KIND);
    expect(reparsed.version).toBe(BUILD_CONFIG_VERSION);
  });
});

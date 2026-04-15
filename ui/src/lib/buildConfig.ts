// Shared serialization for the "auroraboot build config" portable format
// used by the Artifact Builder (export current form state) and the Artifact
// Detail page (export an already-built artifact). A single kind/version pair
// keeps exports from both paths importable back into the Builder.

import type { Artifact, CreateArtifactInput, SecureBootKeySet } from "@/api/artifacts";
import type { Group } from "@/api/groups";

export const BUILD_CONFIG_KIND = "auroraboot.build-config";
export const BUILD_CONFIG_VERSION = 1;

export type UserMode = "default" | "custom" | "none";

export interface BuildConfigPayload {
  version: typeof BUILD_CONFIG_VERSION;
  kind: typeof BUILD_CONFIG_KIND;
  exportedAt: string;
  name?: string;
  buildMode: "image" | "dockerfile";
  source: {
    baseImage: string;
    kairosVersion: string;
    arch: string;
    model: string;
    variant: string;
    kubernetesDistro?: string;
    kubernetesVersion?: string;
    kairosInitImage?: string;
  };
  dockerfile?: string;
  overlayRootfs?: string;
  outputs: CreateArtifactInput["outputs"];
  signing: {
    ukiKeySetName?: string;
    ukiSecureBootEnroll?: string;
  };
  provisioning: {
    autoInstall: boolean;
    registerAuroraBoot: boolean;
    targetGroupName?: string;
    userMode: UserMode;
    username?: string;
    sshKeys?: string;
  };
  advancedCloudConfig?: string;
}

// Builds a payload from ArtifactBuilder's current live form state.
export function payloadFromBuilder(args: {
  form: CreateArtifactInput;
  buildMode: "image" | "dockerfile";
  groups: Group[];
  keySets: SecureBootKeySet[];
  userMode: UserMode;
  username: string;
  sshKeys: string;
  advancedConfig: string;
}): BuildConfigPayload {
  const { form, buildMode, groups, keySets, userMode, username, sshKeys, advancedConfig } = args;
  const groupName = groups.find((g) => g.id === form.provisioning.targetGroupId)?.name;
  const keySetName = keySets.find((k) => k.id === form.signing.ukiKeySetId)?.name;
  return {
    version: BUILD_CONFIG_VERSION,
    kind: BUILD_CONFIG_KIND,
    exportedAt: new Date().toISOString(),
    name: form.name || undefined,
    buildMode,
    source: {
      baseImage: form.baseImage,
      kairosVersion: form.kairosVersion,
      arch: form.arch,
      model: form.model,
      variant: form.variant,
      kubernetesDistro: form.variant === "standard" ? form.kubernetesDistro || undefined : undefined,
      kubernetesVersion: form.variant === "standard" ? form.kubernetesVersion || undefined : undefined,
      kairosInitImage: form.kairosInitImage || undefined,
    },
    dockerfile: buildMode === "dockerfile" ? form.dockerfile : undefined,
    overlayRootfs: form.overlayRootfs || undefined,
    outputs: { ...form.outputs },
    signing: {
      ukiKeySetName: keySetName,
      ukiSecureBootEnroll: form.signing.ukiSecureBootEnroll || undefined,
    },
    provisioning: {
      autoInstall: form.provisioning.autoInstall,
      registerAuroraBoot: form.provisioning.registerAuroraBoot,
      targetGroupName: groupName,
      userMode,
      username: userMode === "custom" ? username : undefined,
      sshKeys: userMode !== "none" && sshKeys.trim() ? sshKeys : undefined,
    },
    advancedCloudConfig: advancedConfig || undefined,
  };
}

// Builds a payload from an already-built Artifact record. Signing is not
// stored on the artifact (resolved at build time), so the export carries no
// key reference — on re-import the user picks one again.
export function payloadFromArtifact(artifact: Artifact, groups: Group[]): BuildConfigPayload {
  const groupName = artifact.targetGroupId
    ? groups.find((g) => g.id === artifact.targetGroupId)?.name
    : undefined;
  const buildMode: "image" | "dockerfile" = artifact.dockerfile ? "dockerfile" : "image";
  return {
    version: BUILD_CONFIG_VERSION,
    kind: BUILD_CONFIG_KIND,
    exportedAt: new Date().toISOString(),
    name: artifact.name,
    buildMode,
    source: {
      baseImage: artifact.baseImage,
      kairosVersion: artifact.kairosVersion,
      arch: artifact.arch || "amd64",
      model: artifact.model || "generic",
      variant: artifact.variant || "core",
      kubernetesDistro: artifact.kubernetesDistro || undefined,
      kubernetesVersion: artifact.kubernetesVersion || undefined,
      kairosInitImage: artifact.kairosInitImage || undefined,
    },
    dockerfile: buildMode === "dockerfile" ? artifact.dockerfile : undefined,
    outputs: {
      iso: artifact.iso,
      cloudImage: artifact.cloudImage,
      netboot: artifact.netboot,
      rawDisk: artifact.rawDisk ?? false,
      tar: artifact.tar ?? false,
      gce: artifact.gce ?? false,
      vhd: artifact.vhd ?? false,
      uki: artifact.uki ?? false,
      fips: artifact.fips,
      trustedBoot: artifact.trustedBoot,
    },
    signing: {},
    provisioning: {
      autoInstall: artifact.autoInstall ?? true,
      registerAuroraBoot: artifact.registerAuroraBoot ?? true,
      targetGroupName: groupName,
      userMode: "default",
    },
    advancedCloudConfig: artifact.cloudConfig || undefined,
  };
}

// Triggers a browser download of the given payload as a JSON file.
// Returns the chosen filename so callers can surface it in a toast.
export function downloadBuildConfig(payload: BuildConfigPayload): string {
  const blob = new Blob([JSON.stringify(payload, null, 2)], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  const slug = (payload.name || "build-config").trim().toLowerCase().replace(/[^a-z0-9-]+/g, "-");
  const filename = `${slug || "build-config"}.auroraboot.json`;
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
  return filename;
}

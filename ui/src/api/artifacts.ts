import { apiFetch, apiFetchText } from "./client";

export interface Artifact {
  id: string;
  name?: string;
  saved?: boolean;
  phase: string;
  message: string;
  baseImage: string;
  kairosVersion: string;
  model: string;
  arch?: string;
  variant?: string;
  iso: boolean;
  cloudImage: boolean;
  netboot: boolean;
  rawDisk: boolean;
  tar: boolean;
  gce: boolean;
  vhd: boolean;
  uki: boolean;
  fips: boolean;
  trustedBoot: boolean;
  kairosInitImage?: string;
  autoInstall: boolean;
  registerAuroraBoot: boolean;
  dockerfile?: string;
  cloudConfig?: string;
  kubernetesDistro?: string;
  kubernetesVersion?: string;
  targetGroupId?: string;
  containerImage?: string;
  artifacts: string[];
  createdAt: string;
  updatedAt: string;
}

export interface CreateArtifactOutputs {
  iso: boolean;
  cloudImage: boolean;
  netboot: boolean;
  rawDisk: boolean;
  tar: boolean;
  gce: boolean;
  vhd: boolean;
  uki: boolean;
  fips: boolean;
  trustedBoot: boolean;
}

export interface CreateArtifactSigning {
  ukiKeySetId: string;
  ukiSecureBootKey: string;
  ukiSecureBootCert: string;
  ukiTpmPcrKey: string;
  ukiPublicKeysDir: string;
  ukiSecureBootEnroll: string;
}

export interface CreateArtifactProvisioning {
  autoInstall: boolean;
  registerAuroraBoot: boolean;
  targetGroupId: string;
  userMode?: "default" | "custom" | "none";
  username?: string;
  password?: string;
  sshKeys?: string;
  /**
   * Explicit list baked into phonehome.allowed_commands on the node.
   * Always sent by the ArtifactBuilder UI; omit/null means "use AuroraBoot's
   * safe defaults". Empty array means deny-all (observe-only node).
   */
  allowedCommands?: string[];
}

export interface CreateArtifactInput {
  name?: string;
  baseImage: string;
  kairosVersion: string;
  model: string;
  arch: string;
  variant: string;
  kubernetesDistro?: string;
  kubernetesVersion?: string;
  dockerfile?: string;
  overlayRootfs?: string;
  kairosInitImage?: string;
  outputs: CreateArtifactOutputs;
  signing: CreateArtifactSigning;
  provisioning: CreateArtifactProvisioning;
  cloudConfig?: string;
}

export interface SecureBootKeySet {
  id: string;
  name: string;
  keysDir: string;
  tpmPcrKeyPath: string;
  secureBootEnroll: string;
  createdAt: string;
}

export function listSecureBootKeySets(): Promise<SecureBootKeySet[]> {
  return apiFetch("/api/v1/secureboot-keys");
}

export function generateSecureBootKeys(
  name: string
): Promise<SecureBootKeySet> {
  return apiFetch("/api/v1/secureboot-keys/generate", {
    method: "POST",
    body: JSON.stringify({ name }),
  });
}

export function deleteSecureBootKeySet(id: string): Promise<void> {
  return apiFetch(`/api/v1/secureboot-keys/${id}`, { method: "DELETE" });
}

// exportSecureBootKeySet fetches the tar.gz export for a key set as a Blob.
// Uses raw fetch so we can surface a binary body; apiFetch assumes JSON.
export async function exportSecureBootKeySet(id: string): Promise<{ blob: Blob; filename: string }> {
  const token = localStorage.getItem("auroraboot_token");
  const res = await fetch(`/api/v1/secureboot-keys/${id}/export`, {
    headers: token ? { Authorization: `Bearer ${token}` } : undefined,
  });
  if (!res.ok) {
    throw new Error(`API error ${res.status}: ${await res.text()}`);
  }
  const disp = res.headers.get("Content-Disposition") || "";
  const match = disp.match(/filename="?([^";]+)"?/);
  const filename = match?.[1] || `keyset-${id}.tar.gz`;
  return { blob: await res.blob(), filename };
}

// importSecureBootKeySet uploads a tar.gz produced by exportSecureBootKeySet
// and returns the new key set record. The optional `name` overrides the name
// from the archive's manifest (useful when importing into an instance that
// already has a key set with the same name).
export async function importSecureBootKeySet(file: File, name?: string): Promise<SecureBootKeySet> {
  const token = localStorage.getItem("auroraboot_token");
  const fd = new FormData();
  fd.append("file", file);
  const qs = name ? `?name=${encodeURIComponent(name)}` : "";
  const res = await fetch(`/api/v1/secureboot-keys/import${qs}`, {
    method: "POST",
    headers: token ? { Authorization: `Bearer ${token}` } : undefined,
    body: fd,
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || `API error ${res.status}`);
  }
  return res.json();
}

export function listArtifacts(): Promise<Artifact[]> {
  return apiFetch<Artifact[]>("/api/v1/artifacts");
}

export function getArtifact(id: string): Promise<Artifact> {
  return apiFetch<Artifact>(`/api/v1/artifacts/${id}`);
}

export function createArtifact(
  input: CreateArtifactInput
): Promise<Artifact> {
  return apiFetch<Artifact>("/api/v1/artifacts", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function getArtifactLogs(id: string): Promise<string> {
  return apiFetchText(`/api/v1/artifacts/${id}/logs`);
}

export function cancelArtifact(id: string): Promise<void> {
  return apiFetch(`/api/v1/artifacts/${id}/cancel`, { method: "POST" });
}

export function artifactDownloadUrl(id: string, filename: string): string {
  const token = localStorage.getItem("auroraboot_token") || "";
  return `/api/v1/artifacts/${id}/download/${filename}?token=${token}`;
}

export function updateArtifact(
  id: string,
  patch: { name?: string; saved?: boolean }
): Promise<Artifact> {
  return apiFetch<Artifact>(`/api/v1/artifacts/${id}`, {
    method: "PATCH",
    body: JSON.stringify(patch),
  });
}

export function deleteArtifact(id: string): Promise<void> {
  return apiFetch(`/api/v1/artifacts/${id}`, { method: "DELETE" });
}

export function clearFailedArtifacts(): Promise<void> {
  return apiFetch("/api/v1/artifacts/failed", { method: "DELETE" });
}

export async function uploadOverlayFiles(files: FileList | File[]): Promise<string> {
  const formData = new FormData();
  for (const file of Array.from(files)) {
    formData.append("files", file);
  }
  const token = localStorage.getItem("auroraboot_token") || "";
  const res = await fetch("/api/v1/artifacts/upload-overlay", {
    method: "POST",
    headers: { Authorization: `Bearer ${token}` },
    body: formData,
  });
  if (!res.ok) throw new Error("Upload failed");
  const data = await res.json();
  return data.path;
}

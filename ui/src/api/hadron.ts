import { apiFetch } from "./client";
import type { Artifact } from "./artifacts";

export interface HadronFirmwareItem {
  name: string;
  image: string;
  version: string;
  releaseTag: string;
}

export interface HadronLayerItem {
  name: string;
  title?: string;
  description?: string;
  image: string;
  latest?: string;
  tags?: string[];
}

export interface RegistryCredential {
  registry: string;
  username: string;
  /** Passwords are never returned by the API — only the presence flag. */
  hasPassword?: boolean;
  /** Populated when submitting a new/rotated password. */
  password?: string;
  /** When true on submit, preserve the currently-stored password verbatim. */
  keepPassword?: boolean;
}

export interface CreateHadronArtifactInput {
  baseImage: string;
  firmware: string[];
  layers: string[];
  extraDockerfile?: string;
  platforms: string[];
  outputRef: string;
  push?: boolean;
  produceTarball?: boolean;
  noCache?: boolean;
}

export function listHadronBaseVersions(): Promise<string[]> {
  return apiFetch<string[]>("/api/v1/hadron/base-versions");
}

export function listHadronFirmware(): Promise<HadronFirmwareItem[]> {
  return apiFetch<HadronFirmwareItem[]>("/api/v1/hadron/firmware");
}

export function listHadronLayers(): Promise<HadronLayerItem[]> {
  return apiFetch<HadronLayerItem[]>("/api/v1/hadron/layers");
}

export function listRegistryCredentials(): Promise<RegistryCredential[]> {
  return apiFetch<RegistryCredential[]>("/api/v1/hadron/registry-credentials");
}

export function putRegistryCredentials(
  creds: RegistryCredential[]
): Promise<RegistryCredential[]> {
  return apiFetch<RegistryCredential[]>("/api/v1/hadron/registry-credentials", {
    method: "PUT",
    body: JSON.stringify(creds),
  });
}

export function createHadronArtifact(
  name: string,
  spec: CreateHadronArtifactInput
): Promise<Artifact> {
  return apiFetch<Artifact>("/api/v1/artifacts", {
    method: "POST",
    body: JSON.stringify({ kind: "hadron", name, hadron: spec }),
  });
}

// retryHadronArtifact spawns a new build from a failed hadron artifact's
// persisted spec. Returns the fresh Artifact (Pending phase) — the caller
// typically navigates to its detail page to watch the retry progress.
export function retryHadronArtifact(id: string): Promise<Artifact> {
  return apiFetch<Artifact>(`/api/v1/artifacts/${id}/retry`, {
    method: "POST",
  });
}

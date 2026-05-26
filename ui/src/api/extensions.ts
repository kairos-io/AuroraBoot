import { apiFetch, apiFetchText } from "./client";

export type ExtensionType = "sysext" | "confext";

export interface ExtensionSource {
  mode: "artifact" | "image" | "dockerfile";
  artifactId?: string;
  baseImage?: string;
  dockerfile?: string;
  extraSteps?: string;
  buildContextDir?: string;
}

export interface Extension {
  id: string;
  name: string;
  type: ExtensionType;
  phase: string; // Pending | Building | Ready | Error
  message: string;
  arch: string;
  version: string;
  sourceMode: ExtensionSource["mode"];
  sourceArtifactId?: string;
  sourceImage?: string;
  dockerfile?: string;
  extraSteps?: string;
  signingKeySetId?: string;
  hierarchies?: string[];
  serviceReload?: boolean;
  containerImage?: string;
  rawFilename?: string;
  createdAt: string;
  updatedAt: string;
}

export interface CreateExtensionInput {
  name: string;
  type: ExtensionType;
  arch: string;
  version: string;
  source: ExtensionSource;
  signingKeySetId?: string;
  hierarchies?: string[];
  serviceReload?: boolean;
}

export interface ExtensionBuildStatus {
  id: string;
  phase: string;
  message: string;
  rawFile?: string;
  containerImage?: string;
}

export function listExtensions(): Promise<Extension[]> {
  return apiFetch<Extension[]>("/api/v1/extensions");
}

export function getExtension(id: string): Promise<Extension> {
  return apiFetch<Extension>(`/api/v1/extensions/${id}`);
}

export function createExtension(input: CreateExtensionInput): Promise<ExtensionBuildStatus> {
  return apiFetch<ExtensionBuildStatus>("/api/v1/extensions", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function updateExtension(id: string, patch: { name?: string }): Promise<Extension> {
  return apiFetch<Extension>(`/api/v1/extensions/${id}`, {
    method: "PATCH",
    body: JSON.stringify(patch),
  });
}

export function deleteExtension(id: string): Promise<void> {
  return apiFetch<void>(`/api/v1/extensions/${id}`, { method: "DELETE" });
}

export function cancelExtension(id: string): Promise<void> {
  return apiFetch<void>(`/api/v1/extensions/${id}/cancel`, { method: "POST" });
}

export function getExtensionLogs(id: string): Promise<string> {
  return apiFetchText(`/api/v1/extensions/${id}/logs`);
}

export function extensionDownloadUrl(id: string, filename: string): string {
  const token = localStorage.getItem("auroraboot_token") ?? "";
  return `/api/v1/extensions/${id}/download/${filename}?token=${token}`;
}

// NodeExtensionRow mirrors store.NodeExtensionRow on the server. Tracks
// which extensions are installed on which node, per boot scope.
export interface NodeExtensionRow {
  nodeId: string;
  name: string;
  type: ExtensionType;
  bootState: "active" | "passive" | "recovery" | "common";
  extensionId?: string;
  version: string;
  installedAt: string;
  updatedAt: string;
}

export function listExtensionsForNode(nodeId: string): Promise<NodeExtensionRow[]> {
  return apiFetch<NodeExtensionRow[]>(`/api/v1/nodes/${nodeId}/extensions`);
}

export function listNodesForExtension(extensionId: string): Promise<NodeExtensionRow[]> {
  return apiFetch<NodeExtensionRow[]>(`/api/v1/extensions/${extensionId}/nodes`);
}

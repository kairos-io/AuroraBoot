import { apiFetch } from "./client";

export interface Deployment {
  id: string;
  artifactId: string;
  method: string;
  status: string;
  message: string;
  bmcTargetId: string;
  progress: number;
  startedAt: string;
  completedAt?: string;
}

export interface BMCTarget {
  id: string;
  name: string;
  endpoint: string;
  vendor: string;
  username: string;
  verifySSL: boolean;
  // systemId optionally pins the target ComputerSystem by its Redfish Id. Leave
  // blank for single-system BMCs; required when the BMC exposes more than one,
  // mirroring the CLI's --system-id.
  systemId?: string;
  // imageUrl optionally overrides the global default image URL for this BMC (the
  // HTTP(S) URL the BMC pulls the ISO from). Blank to use the global default.
  imageUrl?: string;
  nodeId?: string;
  createdAt: string;
}

// InspectResult mirrors the server's inspectResponse (POST
// /api/v1/bmc-targets/:id/inspect): the hardware facts AuroraBoot read off a
// saved BMC target, used to drive the deploy pre-flight gate.
export interface InspectResult {
  memoryGiB: number;
  processorCount: number;
  model: string;
  manufacturer: string;
  serialNumber: string;
  supportedFeatures: string[];
}

// DeployProgress is the live deploy-step event broadcast over the UI WebSocket
// ({"type":"deploy-progress",...}). It carries the same step/percent the server
// records on the Deployment row so the Deployments page can update in real time.
export interface DeployProgress {
  deploymentId: string;
  status: string;
  progress: number;
  step: string;
  message: string;
}

export interface NetbootStatus {
  running: boolean;
  artifactId: string;
  address: string;
  port: string;
}

export const listDeployments = () =>
  apiFetch<Deployment[]>("/api/v1/deployments");

export const getDeployment = (id: string) =>
  apiFetch<Deployment>(`/api/v1/deployments/${id}`);

export const listBMCTargets = () =>
  apiFetch<BMCTarget[]>("/api/v1/bmc-targets");

export const createBMCTarget = (t: {
  name: string;
  endpoint: string;
  vendor: string;
  username: string;
  password: string;
  verifySSL: boolean;
  systemId?: string;
  imageUrl?: string;
}) => apiFetch<BMCTarget>("/api/v1/bmc-targets", { method: "POST", body: JSON.stringify(t) });

// updateBMCTarget mirrors createBMCTarget but PUTs to an existing target. Leave
// `password` unset (or empty) to keep the stored credential — the backend treats
// a blank password as "no change".
export const updateBMCTarget = (
  id: string,
  t: {
    name: string;
    endpoint: string;
    vendor: string;
    username: string;
    password?: string;
    verifySSL: boolean;
    systemId?: string;
    imageUrl?: string;
  }
) =>
  apiFetch<BMCTarget>(`/api/v1/bmc-targets/${id}`, {
    method: "PUT",
    body: JSON.stringify(t),
  });

export const deleteBMCTarget = (id: string) =>
  apiFetch(`/api/v1/bmc-targets/${id}`, { method: "DELETE" });

export const inspectHardware = (id: string) =>
  apiFetch<InspectResult>(`/api/v1/bmc-targets/${id}/inspect`, { method: "POST" });

export const deployRedfish = (
  artifactId: string,
  body: {
    bmcTargetId?: string;
    endpoint?: string;
    username?: string;
    password?: string;
    vendor?: string;
    verifySSL?: boolean;
    systemId?: string;
  }
) =>
  apiFetch<Deployment>(`/api/v1/artifacts/${artifactId}/deploy/redfish`, {
    method: "POST",
    body: JSON.stringify(body),
  });

export const getNetbootStatus = () =>
  apiFetch<NetbootStatus>("/api/v1/netboot/status");

export const startNetboot = (artifactId: string) =>
  apiFetch("/api/v1/netboot/start", {
    method: "POST",
    body: JSON.stringify({ artifactId }),
  });

export const stopNetboot = () =>
  apiFetch("/api/v1/netboot/stop", { method: "POST" });

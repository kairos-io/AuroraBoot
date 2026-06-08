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
}) => apiFetch<BMCTarget>("/api/v1/bmc-targets", { method: "POST", body: JSON.stringify(t) });

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

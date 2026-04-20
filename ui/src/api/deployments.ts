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
  apiFetch(`/api/v1/bmc-targets/${id}/inspect`, { method: "POST" });

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

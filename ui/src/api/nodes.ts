import { apiFetch } from "./client";

export interface Node {
  id: string;
  hostname: string;
  machineID: string;
  groupID: string;
  group?: { id: string; name: string };
  labels: Record<string, string>;
  phase: string;
  osRelease: Record<string, string> | null;
  agentVersion: string;
  lastHeartbeat: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface NodeListParams {
  group_id?: string;  // query param name
  label?: string;
  phase?: string;
}

export interface CommandResult {
  id: string;
  node_id: string;
  command: string;
  args: Record<string, unknown>;
  status: string;
  result?: string;
  created_at: string;
  updated_at: string;
}

export function listNodes(params?: NodeListParams): Promise<Node[]> {
  const query = new URLSearchParams();
  if (params?.group_id) query.set("group", params.group_id);
  if (params?.label) query.set("label", params.label.replace("=", ":"));
  if (params?.phase) query.set("phase", params.phase);
  const qs = query.toString();
  return apiFetch<Node[]>(`/api/v1/nodes${qs ? `?${qs}` : ""}`);
}

export function getNode(id: string): Promise<Node> {
  return apiFetch<Node>(`/api/v1/nodes/${id}`);
}

export function deleteNode(id: string): Promise<void> {
  return apiFetch<void>(`/api/v1/nodes/${id}`, { method: "DELETE" });
}

export function setLabels(
  id: string,
  labels: Record<string, string>
): Promise<Node> {
  return apiFetch<Node>(`/api/v1/nodes/${id}/labels`, {
    method: "PUT",
    body: JSON.stringify({ labels }),
  });
}

export function setGroup(id: string, groupID: string): Promise<Node> {
  return apiFetch<Node>(`/api/v1/nodes/${id}/group`, {
    method: "PUT",
    body: JSON.stringify({ groupID }),
  });
}

export function sendCommand(
  nodeID: string,
  command: string,
  args?: Record<string, unknown>
): Promise<CommandResult> {
  return apiFetch<CommandResult>(`/api/v1/nodes/${nodeID}/commands`, {
    method: "POST",
    body: JSON.stringify({ command, args }),
  });
}

export function sendBulkCommand(
  selector: { groupID?: string; labels?: Record<string, string>; nodeIDs?: string[] },
  command: string,
  args?: Record<string, unknown>
): Promise<CommandResult[]> {
  return apiFetch<CommandResult[]>(`/api/v1/nodes/commands`, {
    method: "POST",
    body: JSON.stringify({ selector, command, args }),
  });
}

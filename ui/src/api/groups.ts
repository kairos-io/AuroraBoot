import { apiFetch } from "./client";
import type { CommandResult } from "./nodes";

export interface Group {
  id: string;
  name: string;
  description: string;
  node_count: number;
  created_at: string;
  updated_at: string;
}

export interface CreateGroupInput {
  name: string;
  description?: string;
}

export interface UpdateGroupInput {
  name?: string;
  description?: string;
}

export function listGroups(): Promise<Group[]> {
  return apiFetch<Group[]>("/api/v1/groups");
}

export function getGroup(id: string): Promise<Group> {
  return apiFetch<Group>(`/api/v1/groups/${id}`);
}

export function createGroup(input: CreateGroupInput): Promise<Group> {
  return apiFetch<Group>("/api/v1/groups", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function updateGroup(
  id: string,
  input: UpdateGroupInput
): Promise<Group> {
  return apiFetch<Group>(`/api/v1/groups/${id}`, {
    method: "PUT",
    body: JSON.stringify(input),
  });
}

export function deleteGroup(id: string): Promise<void> {
  return apiFetch<void>(`/api/v1/groups/${id}`, { method: "DELETE" });
}

export function sendGroupCommand(
  groupID: string,
  command: string,
  args?: Record<string, unknown>
): Promise<CommandResult[]> {
  return apiFetch<CommandResult[]>(`/api/v1/groups/${groupID}/commands`, {
    method: "POST",
    body: JSON.stringify({ command, args }),
  });
}

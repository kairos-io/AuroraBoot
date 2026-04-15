import { apiFetch } from "./client";

export interface RegistrationToken {
  token: string;
}

export function getRegistrationToken(): Promise<RegistrationToken> {
  return apiFetch<RegistrationToken>("/api/v1/settings/registration-token");
}

export function rotateRegistrationToken(): Promise<RegistrationToken> {
  return apiFetch<RegistrationToken>(
    "/api/v1/settings/registration-token/rotate",
    { method: "POST" }
  );
}

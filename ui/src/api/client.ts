const TOKEN_KEY = "auroraboot_token";

/**
 * ApiError carries the HTTP status plus the parsed response body so callers
 * can render structured errors (e.g. a 409 with the list of artifacts that
 * still reference an extension) instead of dumping raw JSON at the user.
 */
export class ApiError extends Error {
  status: number;
  body: unknown;
  raw: string;
  constructor(status: number, body: unknown, raw: string) {
    const detail =
      (body && typeof body === "object" && "error" in (body as any) && (body as any).error) ||
      raw ||
      `HTTP ${status}`;
    super(String(detail));
    this.name = "ApiError";
    this.status = status;
    this.body = body;
    this.raw = raw;
  }
}


export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}

export function setToken(token: string) {
  localStorage.setItem(TOKEN_KEY, token);
}

export function clearToken() {
  localStorage.removeItem(TOKEN_KEY);
}

export function login(password: string) {
  setToken(password);
}

export function logout() {
  clearToken();
}

/**
 * Validate the current token by making a test API call.
 * Returns true if the token is valid, false if 401.
 * Does NOT redirect on 401 (unlike apiFetch).
 */
export async function validateToken(): Promise<boolean> {
  const token = getToken();
  if (!token) return false;

  try {
    const res = await fetch("/api/v1/nodes", {
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
    });
    return res.ok;
  } catch {
    return false;
  }
}

export async function apiFetch<T>(
  path: string,
  options: RequestInit = {}
): Promise<T> {
  const token = getToken();
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...(options.headers as Record<string, string>),
  };
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }

  const res = await fetch(path, {
    ...options,
    headers,
  });

  if (res.status === 401) {
    clearToken();
    window.location.href = "/login";
    throw new Error("Unauthorized");
  }

  if (!res.ok) {
    const text = await res.text();
    let body: unknown = text;
    try {
      body = JSON.parse(text);
    } catch {
      /* keep body as raw text */
    }
    throw new ApiError(res.status, body, text);
  }

  if (res.status === 204) {
    return undefined as T;
  }

  return res.json();
}

export async function apiFetchText(
  path: string,
  options: RequestInit = {}
): Promise<string> {
  const token = getToken();
  const headers: Record<string, string> = {
    ...(options.headers as Record<string, string>),
  };
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }

  const res = await fetch(path, { ...options, headers });

  if (res.status === 401) {
    clearToken();
    window.location.href = "/login";
    throw new Error("Unauthorized");
  }

  if (!res.ok) {
    throw new Error(`API error ${res.status}`);
  }

  return res.text();
}

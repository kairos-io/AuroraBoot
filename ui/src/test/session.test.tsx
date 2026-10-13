import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";

import { login, logout, hasSession, apiFetch } from "@/api/client";
import { Login } from "@/pages/Login";
import App from "@/App";

describe("session", () => {
  const fetchMock = vi.fn();

  beforeEach(() => {
    fetchMock.mockReset();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it("logs in by posting the password to /api/v1/session and stores nothing", async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 204 }));
    expect(await login("pw")).toBe(true);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/session");
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body)).toEqual({ password: "pw" });
    expect(localStorage.length).toBe(0);
  });

  it("reports a wrong password", async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 401 }));
    expect(await login("bad")).toBe(false);
  });

  it("logs out with DELETE /api/v1/session", async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 204 }));
    await logout();
    expect(fetchMock).toHaveBeenCalledWith("/api/v1/session", expect.objectContaining({ method: "DELETE" }));
  });

  it("checks the session with GET /api/v1/session", async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 204 }));
    expect(await hasSession()).toBe(true);
    fetchMock.mockResolvedValue(new Response(null, { status: 401 }));
    expect(await hasSession()).toBe(false);
  });

  it("sends no Authorization header on API calls", async () => {
    fetchMock.mockResolvedValue(new Response("[]", { status: 200 }));
    await apiFetch("/api/v1/nodes");
    const init = fetchMock.mock.calls[0][1];
    expect(new Headers(init.headers).has("Authorization")).toBe(false);
  });

  it("removes a password left in localStorage by an earlier version", async () => {
    localStorage.setItem("auroraboot_token", "old-password");
    fetchMock.mockResolvedValue(new Response(null, { status: 401 }));
    render(
      <MemoryRouter initialEntries={["/login"]}>
        <App />
      </MemoryRouter>,
    );
    await waitFor(() => expect(localStorage.getItem("auroraboot_token")).toBeNull());
  });

  it("sends a signed-out visitor to the login page", async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 401 }));
    render(
      <MemoryRouter initialEntries={["/"]}>
        <App />
      </MemoryRouter>,
    );
    expect((await screen.findAllByText("Sign In")).length).toBeGreaterThan(0);
  });

  it("shows an error for a wrong password on the login page", async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 401 }));
    render(
      <MemoryRouter initialEntries={["/login"]}>
        <Routes>
          <Route path="/login" element={<Login />} />
        </Routes>
      </MemoryRouter>,
    );
    fireEvent.change(screen.getByLabelText("Password"), { target: { value: "bad" } });
    fireEvent.click(screen.getByRole("button", { name: /sign in/i }));
    expect(await screen.findByText("Invalid password")).toBeInTheDocument();
  });
});

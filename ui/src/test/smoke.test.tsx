import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";

import App from "@/App";

// jsdom doesn't ship matchMedia; shadcn's theme provider touches it on mount.
beforeEach(() => {
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  });
});

describe("UI smoke", () => {
  it("renders the AuroraBoot login screen for unauthenticated users", () => {
    // No token in storage → the auth guard sends `/` to `/login`.
    window.localStorage.clear();

    render(
      <MemoryRouter initialEntries={["/"]}>
        <App />
      </MemoryRouter>,
    );

    expect(screen.getByRole("heading", { name: /AuroraBoot/i })).toBeInTheDocument();
    expect(screen.getByLabelText(/password/i)).toBeInTheDocument();
    expect(screen.getByPlaceholderText(/admin password/i)).toBeInTheDocument();
  });
});

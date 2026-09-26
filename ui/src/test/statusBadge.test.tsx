import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";

import { StatusBadge } from "@/components/StatusBadge";

describe("StatusBadge", () => {
  it("renders a known status", () => {
    render(<StatusBadge status="Active" />);
    expect(screen.getByText("Active")).toBeInTheDocument();
  });

  // A node fetched from an endpoint that does not return a node has no phase.
  // One absent field must not be able to throw out of render and take the
  // whole page with it, whatever put it there.
  it("renders a placeholder instead of throwing when the status is missing", () => {
    const missing = undefined as unknown as string;
    expect(() => render(<StatusBadge status={missing} />)).not.toThrow();
    expect(screen.getByText("unknown")).toBeInTheDocument();
  });

  it("renders a placeholder for an empty status", () => {
    render(<StatusBadge status="" />);
    expect(screen.getByText("unknown")).toBeInTheDocument();
  });

  // Canceled is the phase a fail-fast batch leaves on the nodes it stopped.
  // It has to carry a style of its own, or a stopped rollout renders as the
  // same neutral fallback an unrecognised string gets.
  it("styles a canceled command distinctly from an unrecognised status", () => {
    const { container } = render(<StatusBadge status="Canceled" />);
    const canceled = container.firstElementChild?.className ?? "";
    const { container: other } = render(<StatusBadge status="Wat" />);
    const unknown = other.firstElementChild?.className ?? "";
    expect(canceled).not.toEqual(unknown);
    expect(screen.getByText("Canceled")).toBeInTheDocument();
  });
});

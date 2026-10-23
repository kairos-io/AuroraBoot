import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";

import { CommandDialog } from "@/components/CommandDialog";

vi.mock("@/api/artifacts", () => ({
  listArtifacts: vi.fn().mockResolvedValue([]),
  resolveBundle: vi.fn().mockResolvedValue([]),
}));

function openForm(
  onSubmit: ReturnType<typeof vi.fn>,
  action: string,
  targetCount?: number,
) {
  render(
    <CommandDialog
      open
      onOpenChange={() => {}}
      onSubmit={onSubmit}
      title="Send command"
      targetCount={targetCount}
    />,
  );
  fireEvent.click(screen.getByText(action));
}

describe("the Rollout section", () => {
  it("hides the Rollout section for a single node", () => {
    openForm(vi.fn(), "Reboot", 1);
    expect(screen.queryByText("Rollout")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Nodes at a time")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Send reboot" })).toBeInTheDocument();
  });

  it("sends the defaults when the user changes nothing", () => {
    const onSubmit = vi.fn();
    openForm(onSubmit, "Reboot", 3);
    expect(screen.getByText("3 nodes")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Reboot 3 nodes" }));
    expect(onSubmit).toHaveBeenCalledTimes(1);
    const rollout = onSubmit.mock.calls[0][2];
    expect(rollout).toEqual({ stopOnFailure: true, rebootTimeoutSeconds: 600 });
    expect(Object.keys(rollout)).not.toContain("concurrency");
  });

  it("sends the nodes at a time, the stop setting and the timeout in seconds", () => {
    const onSubmit = vi.fn();
    openForm(onSubmit, "Reboot", 3);
    fireEvent.change(screen.getByLabelText("Nodes at a time"), { target: { value: "2" } });
    fireEvent.click(screen.getByLabelText("Stop on the first failure"));
    fireEvent.change(screen.getByLabelText("Wait for a node to come back"), {
      target: { value: "15" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Reboot 3 nodes" }));
    expect(onSubmit.mock.calls[0][2]).toEqual({
      concurrency: 2,
      stopOnFailure: false,
      rebootTimeoutSeconds: 900,
    });
  });

  it("shows the timeout only for commands that reboot the node", () => {
    const { unmount } = render(
      <CommandDialog open onOpenChange={() => {}} onSubmit={vi.fn()} targetCount={3} />,
    );
    fireEvent.click(screen.getByText("Reboot"));
    expect(screen.getByLabelText("Wait for a node to come back")).toBeInTheDocument();
    unmount();

    render(
      <CommandDialog open onOpenChange={() => {}} onSubmit={vi.fn()} targetCount={3} />,
    );
    fireEvent.click(screen.getByText("Run command"));
    expect(screen.getByText("Rollout")).toBeInTheDocument();
    expect(screen.queryByLabelText("Wait for a node to come back")).not.toBeInTheDocument();
  });

  it("names the node count on the submit button", () => {
    openForm(vi.fn(), "Reboot", 3);
    expect(screen.getByRole("button", { name: "Reboot 3 nodes" })).toBeInTheDocument();
  });
});

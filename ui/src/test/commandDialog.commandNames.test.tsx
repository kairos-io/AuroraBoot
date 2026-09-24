import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

import { CommandDialog } from "@/components/CommandDialog";
import { PHONEHOME_ALL_COMMANDS } from "@/lib/buildConfig";

// The dialog's action cards are the only place a command name is minted for a
// node, and the node refuses any name that is not in its baked
// phonehome.allowed_commands -- which the Artifact Builder fills from
// PHONEHOME_ALL_COMMANDS. The two lists have to be the same vocabulary, so
// every card is driven to submit here and its name checked against that list.
// A name only this file knows about reaches the node as
// "command %q is not permitted by the phonehome policy" and can never be
// opted into, because the picker cannot render it.

vi.mock("@/api/artifacts", () => ({
  listArtifacts: vi.fn().mockResolvedValue([]),
  resolveBundle: vi.fn().mockResolvedValue([]),
}));

type Case = {
  card: string;
  // Fill whatever the Send button needs before it becomes enabled.
  prepare?: () => void;
};

const cases: Case[] = [
  {
    card: "Upgrade",
    prepare: () => {
      fireEvent.change(screen.getByPlaceholderText(/quay.io|ghcr.io|image/i), {
        target: { value: "quay.io/kairos/ubuntu:latest" },
      });
    },
  },
  {
    card: "Apply config",
    prepare: () => {
      fireEvent.change(screen.getByRole("textbox"), {
        target: { value: "#cloud-config\nstages: {}\n" },
      });
    },
  },
  { card: "Reboot" },
  { card: "Reset" },
];

describe("CommandDialog command names", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  for (const c of cases) {
    it(`submits a name the agent accepts for "${c.card}"`, async () => {
      const onSubmit = vi.fn();
      render(
        <CommandDialog open onOpenChange={() => {}} onSubmit={onSubmit} />
      );

      fireEvent.click(await screen.findByText(c.card));
      if (c.prepare) {
        await waitFor(() => c.prepare!());
      }

      const send = screen
        .getAllByRole("button")
        .find((b) => /^Send |^Run command$/.test(b.textContent ?? ""));
      expect(send, "send button").toBeTruthy();
      fireEvent.click(send!);

      await waitFor(() => expect(onSubmit).toHaveBeenCalled());
      const sent = onSubmit.mock.calls[0][0] as string;
      expect(PHONEHOME_ALL_COMMANDS).toContain(sent);
    });
  }
});

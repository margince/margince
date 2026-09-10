// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent, within } from "storybook/test";
import { teamWeek } from "./brief.fixtures";
import { AgendaPanel } from "./brief.teamweeklyagenda";
import { StoryProviders } from "./story-utils";

// The Monday agenda: one item per member, in the order the review already
// ranks — the person who asked for help first, the week that went well last.
//
// Every member gets an item, including the one with nothing to fix: a meeting
// that lists only the troubled people reads as a team where only those people
// exist. The frames are the ordinary panel, a team with nothing measured, and
// the one failure this surface has — a browser that will not hand over its
// clipboard, which the reader has to be told about because the alternative is
// pressing Copy and getting silence.
//
// Read every frame in BOTH themes with the toolbar's Theme control.

const meta: Meta<typeof AgendaPanel> = {
  title: "Shell/Brief team agenda",
  component: AgendaPanel,
};
export default meta;

type Story = StoryObj<typeof AgendaPanel>;

// Three reps, three different verdicts, and Copy in the header.
export const RankedAgenda: Story = {
  render: () => (
    <StoryProviders>
      <AgendaPanel review={teamWeek} />
    </StoryProviders>
  ),
};

// A week where nobody was counted. The panel says so in a sentence and drops
// Copy with it: there is nothing to put on a clipboard, and a verb that copied
// an empty agenda would be worse than no verb.
export const NothingToDiscuss: Story = {
  render: () => (
    <StoryProviders>
      <AgendaPanel review={{ ...teamWeek, reps: [], agenda: [] }} />
    </StoryProviders>
  ),
};

// The clipboard refused. `navigator.clipboard` is UNDEFINED outside a secure
// context, so the play below takes it away rather than making a write throw —
// that is the shape the real failure has, and the panel must name it instead of
// leaving the reader to wonder whether Copy did anything.
export const ClipboardRefused: Story = {
  render: () => (
    <StoryProviders>
      <AgendaPanel review={teamWeek} />
    </StoryProviders>
  ),
  play: async ({ canvasElement }) => {
    // Put back afterwards whatever this browser had: the catalog renders many
    // stories in one page, and a clipboard taken away for good would make the
    // next surface that copies fail for a reason nobody could find here.
    const had = Object.getOwnPropertyDescriptor(
      Navigator.prototype,
      "clipboard",
    );
    Object.defineProperty(navigator, "clipboard", {
      value: undefined,
      configurable: true,
    });
    try {
      await userEvent.click(
        within(canvasElement).getByRole("button", { name: "Copy agenda" }),
      );
      await screen.findByText("This browser refused the clipboard");
    } finally {
      if (had) Object.defineProperty(navigator, "clipboard", had);
    }
  },
};

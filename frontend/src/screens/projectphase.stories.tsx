// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { screen, within } from "storybook/test";
import { AdvanceProjectModal, PhaseStepper } from "./projectphase";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// Where a project is, and the one dialog that moves it.
//
// The ladder is a fact plus three moves: the phase the project stands on draws
// as a marker, every other rung as the move to it, and a move in flight
// refuses all of them. The dialog is the same control twice over — a close
// REQUIRES a reason and holds Confirm back until there is one, while every
// other move offers the field and goes without it.

const PROJECT = "22222222-2222-4222-8222-222222222222";

// The advance route is stubbed although no story presses Confirm: the stub's
// fallback answers a POST with an empty page and a 200, so a dialog that
// stopped refusing an empty reason would read as having moved the project.
function phase(children: ReactNode) {
  return () => {
    installFetchStub({
      [`POST /projects/${PROJECT}/advance`]: () =>
        jsonResponse({ title: "Unprocessable Entity", status: 422 }, 422),
    });
    return <StoryProviders>{children}</StoryProviders>;
  };
}

const meta: Meta<typeof PhaseStepper> = {
  title: "Records/Project/Phase",
  component: PhaseStepper,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof PhaseStepper>;

/** A project in delivery: two rungs behind it, one ahead, and the close. */
export const Ladder: Story = {
  render: phase(
    <PhaseStepper phase="delivering" pending={false} onMove={() => {}} />,
  ),
};

/** A project that ended, reopened by the same verb that closed it. */
export const Closed: Story = {
  render: phase(
    <PhaseStepper phase="closed" pending={false} onMove={() => {}} />,
  ),
};

/** A move already in flight: every rung refuses a second press. */
export const Moving: Story = {
  render: phase(
    <PhaseStepper phase="pursuing" pending={true} onMove={() => {}} />,
  ),
};

// The dialog portals to document.body, so what it drew is read off the
// document rather than the story canvas.
export const MovingToDelivery: Story = {
  render: phase(
    <AdvanceProjectModal
      projectId={PROJECT}
      version={7}
      to="delivering"
      onClose={() => {}}
    />,
  ),
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    await dialog.findByText("Reason");
  },
};

// Closing is the one move with a required answer, and the dialog says so on
// the field rather than letting the reader learn it from the server's refusal.
export const ClosingNeedsAReason: Story = {
  render: phase(
    <AdvanceProjectModal
      projectId={PROJECT}
      version={7}
      to="closed"
      onClose={() => {}}
    />,
  ),
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    await dialog.findByText("A closed project needs a reason.");
  },
};

// The ladder's trail is weight rather than a second fill, and the marker for
// where the project stands is the only filled thing on the row — which is
// what the dark ground is worth checking.
export const LadderDark: Story = {
  globals: { theme: "dark" },
  render: phase(
    <PhaseStepper phase="delivering" pending={false} onMove={() => {}} />,
  ),
};

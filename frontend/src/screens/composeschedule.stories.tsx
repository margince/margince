// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { screen, userEvent } from "storybook/test";
import { ScheduleDialog } from "./composeschedule";
import { StoryProviders } from "./story-utils";
// The dialog's rows and hour buttons are styled by the composer's stylesheet,
// which compose.tsx loads in the product; the story mounts the dialog alone.
import "./compose.css";

// Choosing when a message goes out: the three presets most sends take, then the
// calendar and the four business hours for the ones that do not. The hours are
// times, so they read in the body face with tabular figures and the four
// buttons keep one width per digit.

// A fixed clock, so the presets and the calendar month never move with the day
// the catalog is opened.
const NOW = new Date("2026-09-14T10:30:00");

function Dialog({ sendAt = "" }: Readonly<{ sendAt?: string }>) {
  const [chosen, setChosen] = useState(sendAt);
  return (
    <StoryProviders>
      <ScheduleDialog
        open
        onClose={() => {}}
        sendAt={chosen}
        onChoose={setChosen}
        now={NOW}
      />
    </StoryProviders>
  );
}

const meta: Meta<typeof ScheduleDialog> = {
  title: "Patterns/Compose mail/Schedule send",
  component: ScheduleDialog,
};
export default meta;

type Story = StoryObj<typeof ScheduleDialog>;

/** The first step: three presets, and the way to a date of the rep's own. */
export const Presets: Story = { render: () => <Dialog /> };

/**
 * A moment already set: the calendar opens on it, the hour it names is pressed,
 * and the line under the picker says when the message goes out.
 */
export const PickingAnHour: Story = {
  render: () => <Dialog sendAt="2026-09-17T13:00" />,
  play: async () => {
    // `screen`, because Modal portals to document.body rather than into the
    // story's canvas.
    await userEvent.click(
      await screen.findByRole("button", { name: "Select date and time" }),
    );
    await screen.findByRole("button", { name: "13:00", pressed: true });
  },
};

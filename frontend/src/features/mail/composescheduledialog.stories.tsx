// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { screen, userEvent } from "storybook/test";
import { StoryProviders } from "../../testing/story-utils";
import { ScheduleDialog } from "./composescheduledialog";
// The dialog's rows and hour buttons are styled by the composer's stylesheet,
// which compose.tsx loads in the product; the story mounts the dialog alone.
import "./compose.css";

// When a message goes out: the three presets most sends take, then the calendar
// and the four business hours. The hours use tabular figures, one width a digit.

// A fixed clock, so the presets and the calendar month never move with the day
// the catalog is opened.
const NOW = new Date("2026-09-14T10:30:00");
// Late in the afternoon: three of the four hours today have passed.
const LATE = new Date("2026-09-14T15:30:00");

function Dialog({
  sendAt = "",
  now = NOW,
}: Readonly<{ sendAt?: string; now?: Date }>) {
  const [chosen, setChosen] = useState(sendAt);
  return (
    <StoryProviders>
      <ScheduleDialog
        open
        onClose={() => {}}
        sendAt={chosen}
        onChoose={setChosen}
        now={now}
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
 * A moment already set: the calendar opens on it and its hour is pressed.
 * The line under the picker says when the message goes out.
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

/**
 * Late in the day: the days before today and the hours already past are
 * refused, and the picker opens on the one hour left.
 */
export const LateInTheDay: Story = {
  render: () => <Dialog now={LATE} />,
  play: async () => {
    await userEvent.click(
      await screen.findByRole("button", { name: "Select date and time" }),
    );
    await screen.findByRole("button", { name: "17:00", pressed: true });
  },
};

/** A saved moment that has since passed: Schedule send is refused, and says why. */
export const MomentPassed: Story = {
  render: () => <Dialog now={LATE} sendAt="2026-09-14T09:00" />,
  play: async () => {
    await userEvent.click(
      await screen.findByRole("button", { name: "Select date and time" }),
    );
    await screen.findByText("Choose a time in the future.");
  },
};

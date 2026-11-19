// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent } from "storybook/test";
import { StoryProviders } from "../../testing/story-utils";
import { ScheduleMenu } from "./composeschedule";
import "./compose.css";

// The caret beside Send: the other way to send, which opens the schedule dialog.

const meta: Meta<typeof ScheduleMenu> = {
  title: "Patterns/Compose mail/Schedule send menu",
  component: ScheduleMenu,
};
export default meta;

type Story = StoryObj<typeof ScheduleMenu>;

/** Open: the one item that leads to the schedule dialog. */
export const Open: Story = {
  render: () => (
    <StoryProviders>
      <ScheduleMenu onOpen={() => {}} />
    </StoryProviders>
  ),
  play: async () => {
    await userEvent.click(
      await screen.findByRole("button", { name: "Other ways to send" }),
    );
    await screen.findByRole("button", { name: "Schedule send" });
  },
};

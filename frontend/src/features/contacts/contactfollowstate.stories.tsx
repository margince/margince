// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../../testing/story-utils";
import { ContactFollowState } from "./contactfollowstate";

// The two standing choices a reader can make about a quiet contact, as the
// contact page's overview draws them.

function stubState(answer: { state: string; dismissed_until?: string }) {
  stubWithSession(
    { "GET /contacts/p-1/nudge-dismissal": () => jsonResponse(answer) },
    {},
  );
}

const meta: Meta<typeof ContactFollowState> = {
  title: "Records/Contact/Follow state",
  component: ContactFollowState,
  args: { contactId: "p-1" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof ContactFollowState>;

export const Stopped: Story = {
  beforeEach: () => stubState({ state: "stopped" }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByText("You stopped following this contact."),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Resume following" }),
    ).toBeVisible();
  },
};

export const SetAside: Story = {
  beforeEach: () =>
    stubState({
      state: "set_aside",
      dismissed_until: "2026-11-27T09:00:00Z",
    }),
};

export const StoppedDark: Story = { ...Stopped, globals: { theme: "dark" } };

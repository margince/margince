// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { StoryProviders } from "../../../testing/story-utils";
import { SaveRefusalLine } from "./save-refusal-line";

function Line({ message }: Readonly<{ message: string | null }>) {
  return (
    <StoryProviders>
      <SaveRefusalLine message={message} />
    </StoryProviders>
  );
}

const meta: Meta<typeof Line> = {
  title: "Onboarding/Conversation/Save refusal line",
  component: Line,
};
export default meta;

type Story = StoryObj<typeof Line>;

export const Refused: Story = {
  args: { message: "The company name must not be blank." },
  play: async ({ canvasElement }) => {
    await expect(
      await within(canvasElement).findByRole("alert"),
    ).toHaveTextContent("The company name must not be blank.");
  },
};

export const None: Story = {
  args: { message: null },
  play: async ({ canvasElement }) => {
    await expect(within(canvasElement).queryByRole("alert")).toBeNull();
  },
};

export const Phone: Story = {
  tags: ["uat-phone"],
  globals: { viewport: { value: "phone" } },
  args: {
    message:
      "The company name must not be blank, and the website must start with https.",
  },
};

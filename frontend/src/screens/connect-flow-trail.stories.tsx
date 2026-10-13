// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StepTrail } from "./connect-flow-trail";
import { StoryProviders } from "./story-utils";

// The connect dialog's trail at each of its three steps: what is done, where
// the reader is, and what is still ahead.

const meta: Meta<typeof StepTrail> = {
  title: "Settings/You/Connections/Connect flow trail",
  component: StepTrail,
  render: (args) => (
    <StoryProviders>
      <StepTrail {...args} />
    </StoryProviders>
  ),
};
export default meta;
type Story = StoryObj<typeof StepTrail>;

export const Account: Story = { args: { step: 1 } };

export const WhatToBringIn: Story = { args: { step: 2 } };

export const SignIn: Story = { args: { step: 3 } };

export const SignInDark: Story = {
  args: { step: 3 },
  globals: { theme: "dark" },
};

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { ExcuseModal } from "./noticeexcuse";
import { StoryProviders } from "./story-utils";

const meta: Meta = { title: "Patterns/End a notice duty" };
export default meta;

type Story = StoryObj;

const asking = () => (
  <StoryProviders>
    <ExcuseModal
      open
      onClose={() => undefined}
      onConfirm={() => undefined}
      pending={false}
      error={null}
    />
  </StoryProviders>
);

/** Which ground, and the officer's own words for it; Confirm waits for both. */
export const Asking: Story = { render: asking };

export const AskingDark: Story = { globals: { theme: "dark" }, render: asking };

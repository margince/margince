// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { ErasureRefusals } from "./privacy.notices";
import { StoryProviders } from "./story-utils";
import "./privacy.css";

// Why an erasure will not go through, as the confirm dialog says it. Both flags
// can be true at once, which is the state worth a picture: the band owns the
// interval between them, and neither notice interrupts.

const meta: Meta = {
  title: "Patterns/Erasure refusals",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;

/** A statutory retention window outranks the erasure. */
export const LegalHold: Story = {
  render: () => (
    <StoryProviders>
      <ErasureRefusals held movedOn={false} />
    </StoryProviders>
  ),
};

/** Somebody else decided the request first. */
export const AlreadyDecided: Story = {
  render: () => (
    <StoryProviders>
      <ErasureRefusals held={false} movedOn />
    </StoryProviders>
  ),
};

/** Both, which is what the band's own interval exists for. */
export const Both: Story = {
  render: () => (
    <StoryProviders>
      <ErasureRefusals held movedOn />
    </StoryProviders>
  ),
};

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { WriteRefused } from "./common";
import { LookupRunning } from "./contactprovider.notices";
import { StoryProviders } from "./story-utils";

// What the bought-contact-data panel says about itself. The two halves of a
// running lookup are worth reading together: "asking" has no answer yet, and
// "landing" has one and is writing it onto the record.

const meta: Meta = {
  title: "Patterns/Contact data notices",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;

/** A press the server refused, in the server's own words. */
export const Refused: Story = {
  render: () => (
    <StoryProviders>
      <WriteRefused
        titleKey="provider.profile.lookupRefused"
        message="Surfe has no credit left on this key."
      />
    </StoryProviders>
  ),
};

/** Asked, and waiting on the provider. */
export const Asking: Story = {
  render: () => (
    <StoryProviders>
      <LookupRunning asking provider="Surfe" />
    </StoryProviders>
  ),
};

/** Answered, and being written onto the record. */
export const Landing: Story = {
  render: () => (
    <StoryProviders>
      <LookupRunning asking={false} provider="Surfe" />
    </StoryProviders>
  ),
};

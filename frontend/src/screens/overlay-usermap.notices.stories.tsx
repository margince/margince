// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  DirectoryFailed,
  MappingReadFailed,
  MappingSaveRefused,
  ReadCostNotice,
} from "./overlay-usermap.notices";
import { StoryProviders } from "./story-utils";

// What the mirror user-mapping card says about itself. Worth reading together:
// three failures that look alike at a glance and name three different repairs,
// and the one standing notice that is not a failure at all.

const meta: Meta = {
  title: "Patterns/User mapping notices",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;

/** The standing disclosure: what a mapping costs, and what its absence costs. */
export const ReadCost: Story = {
  render: () => (
    <StoryProviders>
      <ReadCostNotice />
    </StoryProviders>
  ),
};

/** The card's own read, failed. */
export const ReadFailed: Story = {
  render: () => (
    <StoryProviders>
      <MappingReadFailed message="HubSpot refused the stored token." />
    </StoryProviders>
  ),
};

/** The picker's directory read, failed — so nobody can be picked. */
export const DirectoryUnreadable: Story = {
  render: () => (
    <StoryProviders>
      <DirectoryFailed message="The HubSpot owners list timed out." />
    </StoryProviders>
  ),
};

/** The write the dialog just attempted, refused. */
export const SaveRefused: Story = {
  render: () => (
    <StoryProviders>
      <MappingSaveRefused message="That HubSpot user is already mapped." />
    </StoryProviders>
  ),
};

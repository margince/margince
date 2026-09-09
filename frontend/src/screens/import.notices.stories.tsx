// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { UndoErrors, UndoInterruptedNotice } from "./import.notices";
import { StoryProviders } from "./story-utils";

// What a reversal says about itself. Both frames need a run that has already
// been committed and then half undone, which is three presses and a stubbed
// failure away in the wizard — so they are posed here instead.
//
// The errored rows are the notice's BODY rather than a list under a band that
// introduces it: the rows are what the reader came for, and one anatomy says
// the same thing.

const meta: Meta = {
  title: "Settings/Data/Data import/Undo notices",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const ERRORED = [
  {
    object: "company" as const,
    id: "01a04298-2000-7000-8000-000000000001",
    reason: "A person still references this company.",
  },
  {
    object: "person" as const,
    id: "01a04298-2000-7000-8000-000000000002",
    reason: "Edited since the import; left exactly as it stood.",
  },
];

/** The reversal halted, and continuing picks up where it stopped. */
export const Interrupted: Story = {
  render: () => (
    <StoryProviders>
      <UndoInterruptedNotice interrupted />
    </StoryProviders>
  ),
};

/** The rows that could not be put back, named with why. */
export const RowsLeftStanding: Story = {
  render: () => (
    <StoryProviders>
      <UndoErrors rows={ERRORED} />
    </StoryProviders>
  ),
};

/** The same list in dark: tone reaches the heading's ink and the rows read on. */
export const RowsLeftStandingDark: Story = {
  globals: { theme: "dark" },
  render: () => (
    <StoryProviders>
      <UndoErrors rows={ERRORED} />
    </StoryProviders>
  ),
};

/** A clean reversal says neither thing. */
export const Quiet: Story = {
  render: () => (
    <StoryProviders>
      <UndoInterruptedNotice interrupted={false} />
      <UndoErrors rows={[]} />
    </StoryProviders>
  ),
};

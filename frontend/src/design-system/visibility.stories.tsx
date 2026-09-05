// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";

import { Badge, Button } from "./atoms";
import { VisibilityBadge, VisibilityLine } from "./visibility";

// Who may read a thing, drawn the same way on every surface. The states worth
// a picture are all five side by side, because a reader tells them apart at a
// glance or not at all — and the line with its verb, which is the shape the
// drawer and the contact panel both draw.

const meta: Meta<typeof VisibilityBadge> = {
  title: "Design System/Visibility",
  component: VisibilityBadge,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof VisibilityBadge>;

/** The whole vocabulary. Open is outlined and quiet, every limit is filled,
 * and withheld is the one caution. */
export const EveryState: Story = {
  render: () => (
    <div style={{ display: "flex", gap: "var(--space-2)", flexWrap: "wrap" }}>
      <VisibilityBadge state="team" />
      <VisibilityBadge state="participants" />
      <VisibilityBadge state="selected" />
      <VisibilityBadge state="private" />
      <VisibilityBadge state="withheld" />
    </div>
  ),
};

/** The mark beside the verb that changes it, as the mail drawer draws it for
 * a captured thread this reader has shared with the organization. */
export const WithItsVerb: Story = {
  render: () => (
    <div style={{ maxWidth: 480 }}>
      <VisibilityLine
        state="team"
        action={<Button small>Make private</Button>}
      />
    </div>
  ),
};

/** A held message says why it is held beside what it is. The reason is a
 * second fact about the same audience, so it stays on the badge's side. */
export const WithAReason: Story = {
  render: () => (
    <div style={{ maxWidth: 480 }}>
      <VisibilityLine
        state="participants"
        marks={<Badge quiet>Marked confidential</Badge>}
        action={<Button small>Change visibility</Button>}
      />
    </div>
  ),
};

/** A reader without standing sees the fact and nothing to press. No empty
 * slot is drawn for the verb they do not have. */
export const NothingToPress: Story = {
  render: () => (
    <div style={{ maxWidth: 480 }}>
      <VisibilityLine state="selected" />
    </div>
  ),
};

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent } from "storybook/test";

import { Badge, Button } from "./atoms";
import { Popover } from "./popover";
import { VisibilityBadge, VisibilityLine } from "./visibility";

// Who may read a thing, drawn the same way on every surface. The states worth
// a picture are all six side by side, because a reader tells them apart at a
// glance or not at all — the line with its verb, which is the shape the mail
// drawer draws, and the chip that opens, which is the record header's.

const meta: Meta<typeof VisibilityBadge> = {
  title: "Components/Labels/Visibility",
  component: VisibilityBadge,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof VisibilityBadge>;

/** The whole vocabulary. Open is outlined and quiet, every limit is filled,
 * and withheld is the one caution. `team` and `workspace` share the open look
 * and the same icon: a message narrowed by its filed record, and a record
 * everyone in the workspace reads. The word is what tells them apart. */
export const EveryState: Story = {
  render: () => (
    <div style={{ display: "flex", gap: "var(--space-2)", flexWrap: "wrap" }}>
      <VisibilityBadge state="team" />
      <VisibilityBadge state="workspace" />
      <VisibilityBadge state="participants" />
      <VisibilityBadge state="selected" />
      <VisibilityBadge state="private" />
      <VisibilityBadge state="withheld" />
    </div>
  ),
};

/** The record header's access chip: the mark as the face of a popover, with
 * a caret saying it opens. Beside a plain badge it keeps the badge's height,
 * so a row of the record's marks stays one line. */
export const ItOpens: Story = {
  render: () => (
    <div
      style={{ display: "flex", gap: "var(--space-4)", alignItems: "center" }}
    >
      <Badge tone="warning">Thin</Badge>
      <Popover label={<VisibilityBadge state="workspace" opens />}>
        All users in the company can see this contact.
      </Popover>
      <Popover label={<VisibilityBadge state="private" opens />}>
        Only you and the users or teams it was shared with can see this contact.
      </Popover>
    </div>
  ),
};

/** The chip reached by Tab: the ring is drawn round the pill itself, at the
 * pill's radius, and not round the button's box it sits in. */
export const ItOpensFocused: Story = {
  render: () => (
    <Popover label={<VisibilityBadge state="private" opens />}>
      Only you and the users or teams it was shared with can see this contact.
    </Popover>
  ),
  play: async () => {
    await userEvent.tab();
  },
};

/** The mark beside the verb that changes it, as the mail drawer draws it for
 * a captured thread this reader has shared with the company. The verb
 * follows the mark rather than taking the far end of the row: on a wide
 * surface — and this one is 480px of a heading that is wider still — a button
 * pushed right is a verb the reader has to travel to and back from. */
export const WithItsVerb: Story = {
  render: () => (
    <div style={{ maxWidth: 480 }}>
      <VisibilityLine
        state="team"
        action={<Button variant="link">Make private</Button>}
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
        marks={<Badge>Marked confidential</Badge>}
        action={<Button variant="link">Change visibility</Button>}
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

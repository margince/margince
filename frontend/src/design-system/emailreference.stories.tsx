// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { EmailReference } from "./emailreference";

// The one citation of an email, in its two shapes and its two readings: on a
// line where the row has room, stacked where a column does not; readable, and
// held from this reader.
const meta: Meta<typeof EmailReference> = {
  title: "Design System/EmailReference",
  component: EmailReference,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof EmailReference>;

export const OnOneLine: Story = {
  render: () => (
    <EmailReference
      subject="AW: Ausbildungsoffensive Bayern"
      occurredAt="Sent 10 d ago"
      onOpen={() => undefined}
    />
  ),
};

// A narrow column: the date sits under the subject rather than beside it, and
// a long subject still gives way rather than pushing the date out of the box.
export const Stacked: Story = {
  render: () => (
    <div style={{ maxWidth: "16rem" }}>
      <EmailReference
        subject="AW: Ausbildungsoffensive Bayern, final agenda and attendees"
        occurredAt="Sent 10 d ago"
        stacked
      />
    </div>
  ),
};

// Held from this reader: the timeline's words for it, and nothing to press.
export const Withheld: Story = {
  render: () => (
    <EmailReference
      subject="Board pack"
      occurredAt="Received 1 d ago"
      withheld
      onOpen={() => undefined}
    />
  ),
};

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { CallAction } from "./callaction";

// Each appearance with a number that dials, one `telUri` refuses, and none.
const meta: Meta<typeof CallAction> = {
  title: "Components/Navigation/Call action",
  component: CallAction,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof CallAction>;

const DIALS = "+49 30 1234 5678";
const REFUSED = "ask reception";

const row = { display: "flex", gap: "var(--space-4)", alignItems: "center" };

export const Value: Story = {
  render: () => (
    <div style={row}>
      <CallAction appearance="value" number={DIALS} />
      <CallAction appearance="value" number={REFUSED} />
    </div>
  ),
};

// A refused number and no number draw nothing, so only the first shows.
export const ButtonForm: Story = {
  name: "Button",
  render: () => (
    <div style={row}>
      <CallAction appearance="button" number={DIALS} label="Call" />
      <CallAction appearance="button" number={REFUSED} label="Call" />
      <CallAction appearance="button" number={undefined} label="Call" />
    </div>
  ),
};

export const Icon: Story = {
  render: () => (
    <div style={row}>
      <CallAction
        appearance="icon"
        number={DIALS}
        label="Call"
        reason="No phone number on this contact."
      />
      <CallAction
        appearance="icon"
        number={REFUSED}
        label="Call"
        reason="No phone number on this contact."
      />
      <CallAction
        appearance="icon"
        number={undefined}
        label="Call"
        reason="No phone number on this contact."
      />
    </div>
  ),
};

// The page's own sentence refuses the call even though a number is on file.
export const IconRefusedByPage: Story = {
  render: () => (
    <div>
      <CallAction
        appearance="icon"
        number={DIALS}
        label="Call"
        reason="No phone number on this contact."
        reasonId="callaction-archived"
      />
      <p id="callaction-archived" className="t-caption">
        This contact is archived and takes no changes.
      </p>
    </div>
  ),
};

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import type { CallOutcome } from "../lib/activitybody";
import { CallOutcomeField } from "./logactivity.calloutcome";

const meta: Meta<typeof CallOutcomeField> = {
  title: "Patterns/Call outcome",
  component: CallOutcomeField,
};
export default meta;
type Story = StoryObj<typeof CallOutcomeField>;

function Outcomes({ initial }: Readonly<{ initial: CallOutcome | "" }>) {
  const [value, onChange] = useState<CallOutcome | "">(initial);
  return <CallOutcomeField value={value} onChange={onChange} />;
}

/** A call opens on "No outcome": saving it as is logs a whole call. */
export const NoOutcomeYet: Story = { render: () => <Outcomes initial="" /> };

/** One press on an answer picks it. */
export const Picked: Story = {
  render: () => <Outcomes initial="" />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const answer = await canvas.findByRole("radio", {
      name: "Left a voicemail",
    });
    await userEvent.click(answer);
    await expect(answer).toBeChecked();
  },
};

export const PickedDark: Story = { ...Picked, globals: { theme: "dark" } };

export const Phone: Story = {
  render: () => <Outcomes initial="call_back_later" />,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

export const PhoneDark: Story = {
  ...Phone,
  globals: { viewport: { value: "phone" }, theme: "dark" },
};

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Checkbox, Radio } from "./atoms";

const meta: Meta<typeof Checkbox> = {
  title: "Components/Forms and input/Checkbox and radio",
  component: Checkbox,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof Checkbox>;

// A disabled control and a pair of radios under one legend: the two states a
// field catalog usually omits and a real form always reaches.
export const States: Story = {
  render: () => (
    <div className="form-stack" style={{ maxWidth: "22rem" }}>
      <Checkbox label="Replace the existing link" defaultChecked />
      <Checkbox label="Include archived records" />
      <Checkbox label="Notify the deal owner" disabled />
      <fieldset className="field-multiselect">
        <legend className="t-label">Target</legend>
        <Radio name="owner-side" label="Owner" defaultChecked />
        <Radio name="owner-side" label="Team" />
      </fieldset>
    </div>
  ),
};

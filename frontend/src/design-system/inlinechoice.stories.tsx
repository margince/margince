// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, screen, userEvent, within } from "storybook/test";
import { LocaleProvider } from "../i18n";
import { ProblemError } from "../screens/common";
import { InlineChoice } from "./inlinechoice";

const meta: Meta = {
  title: "Design System/Inline choice",
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <Story />
      </LocaleProvider>
    ),
  ],
};
export default meta;
type Story = StoryObj;

const OPTIONS = [
  { value: "prospect", label: "Prospect" },
  { value: "customer", label: "Customer" },
];

const labelOf = (value: string) =>
  OPTIONS.find((option) => option.value === value)?.label ?? value;

function Example() {
  const [value, setValue] = useState("prospect");
  return (
    <InlineChoice
      label="Lifecycle"
      value={value}
      options={OPTIONS}
      render={labelOf}
      canEdit
      onSave={async (next) => setValue(next)}
    />
  );
}
export const Editable: Story = { render: () => <Example /> };

export const ReadOnly: Story = {
  render: () => (
    <InlineChoice
      label="Lifecycle"
      value="customer"
      options={OPTIONS}
      render={labelOf}
      canEdit={false}
      readOnlyReason="This company is archived."
      onSave={async () => undefined}
    />
  ),
};

// A refused save keeps the picker open on the reader's answer and says why on
// the ErrorLine beside it.
export const RefusedSave: Story = {
  render: () => (
    <InlineChoice
      label="Lifecycle"
      value="prospect"
      options={OPTIONS}
      render={labelOf}
      canEdit
      onSave={async () => {
        throw new ProblemError({
          code: "version_conflict",
          detail: "Somebody else changed this record.",
        });
      }}
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Change Lifecycle" }),
    );
    await userEvent.click(
      await screen.findByRole("option", { name: "Customer" }),
    );
    await expect(await canvas.findByRole("alert")).toHaveTextContent(
      "Somebody else changed this record.",
    );
  },
};

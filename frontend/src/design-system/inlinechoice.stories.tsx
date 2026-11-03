// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, screen, userEvent, within } from "storybook/test";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { ProblemError } from "../lib/problem";
import { InlineChoice } from "./inlinechoice";

const meta: Meta = {
  title: "Components/Forms and input/Inline choice",
  component: InlineChoice,
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

export const EditableDark: Story = {
  render: () => <Example />,
  globals: { theme: "dark" },
};

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

// The pick waits for Save; a refused save keeps the dialog open on the
// reader's answer and says why as the field's error beneath it.
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
          code: "version_skew",
          detail: "Somebody else changed this record.",
        });
      }}
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: /^Change Lifecycle/ }),
    );
    const dialog = await screen.findByRole("dialog", { name: "Lifecycle" });
    await userEvent.click(within(dialog).getByRole("combobox"));
    await userEvent.click(
      await screen.findByRole("option", { name: "Customer" }),
    );
    await userEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    const refusal = await within(dialog).findByRole("alert");
    await expect(refusal).toHaveTextContent(en["common.versionSkew"]);
    await expect(refusal).toHaveClass("field-error");
  },
};

// A value longer than its column wraps from its start edge, as a cell's text
// does, not centred as a button's label would be.
export const WrappingValue: Story = {
  render: () => (
    <div style={{ inlineSize: "8rem" }}>
      <InlineChoice
        label="Role"
        value="individual"
        options={[{ value: "individual", label: "Individual (own records)" }]}
        render={() => "Individual (own records)"}
        canEdit
        onSave={async () => undefined}
      />
    </div>
  ),
};

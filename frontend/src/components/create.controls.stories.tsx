// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import { Field } from "../design-system/atoms";
import { LocaleProvider, useT } from "../i18n";
import { fieldControl } from "./create.controls";
import { type CreateField, fieldLabel } from "./create.fields";

const meta: Meta = {
  title: "Patterns/Create record/Controls",
  parameters: { layout: "padded" },
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

const nameField: CreateField = {
  key: "full_name",
  labelText: "Full name",
  required: true,
};

const dueField: CreateField = { key: "due_on", labelText: "Due date" };

function Row({ field }: Readonly<{ field: CreateField }>) {
  const t = useT();
  const [value, setValue] = useState(
    field.key === "due_on" ? "2026-03-04" : "",
  );
  return (
    <Field label={fieldLabel(field, t)} required={field.required}>
      {(control) => fieldControl(field, control, value, setValue, t)}
    </Field>
  );
}

function Controls() {
  return (
    <div className="stack">
      <Row field={nameField} />
      <Row field={{ ...dueField, type: "date" }} />
    </div>
  );
}

export const TextAndDate: Story = {
  render: () => <Controls />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const date = await canvas.findByRole("textbox", { name: /Due date/ });
    await expect(date).toBeTruthy();
    await userEvent.click(canvas.getByRole("button", { name: /^Choose date/ }));
    const dialog = await within(document.body).findByRole("dialog", {
      name: /^Choose date/,
    });
    await expect(dialog).toBeTruthy();
  },
};

export const TextAndDatePhone: Story = {
  ...TextAndDate,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

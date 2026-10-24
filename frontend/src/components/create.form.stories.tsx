// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import { LocaleProvider } from "../i18n";
import type { CreateField, FormRows } from "./create.fields";
import { RecordFormBody } from "./create.form";

const meta: Meta = {
  title: "Patterns/Create record/Form with a date",
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

const dealFields: CreateField[] = [
  { key: "name", label: "create.dealName", required: true },
  { key: "expected_close_date", label: "create.expectedClose", type: "date" },
];

function DealForm() {
  const [values, setValues] = useState<Record<string, string>>({
    name: "Acme renewal",
  });
  const [rows, setRows] = useState<FormRows>({});
  return (
    <RecordFormBody
      fields={dealFields}
      values={values}
      setValues={setValues}
      rows={rows}
      setRows={setRows}
      pending={false}
      error={null}
      onSubmit={() => {}}
      onClose={() => {}}
      intent="create"
    />
  );
}

export const UnreadableDay: Story = {
  render: () => <DealForm />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const close = await canvas.findByRole("textbox", {
      name: /Expected close/,
    });
    await userEvent.type(close, "31/02/2027");
    await userEvent.click(canvas.getByRole("button", { name: "Create" }));
    await expect(
      await canvas.findByText("Enter a date as DD/MM/YYYY."),
    ).toBeVisible();
    await expect(close).toHaveAttribute("aria-invalid", "true");
  },
};

export const UnreadableDayPhone: Story = {
  ...UnreadableDay,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

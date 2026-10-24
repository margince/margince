// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, fn, userEvent, within } from "storybook/test";
import { LocaleProvider } from "../i18n";
import type { CreateField, FormRows } from "./create.fields";
import { RecordFormBody } from "./create.form";

type Args = Readonly<{ onSubmit: (values: Record<string, string>) => void }>;

const meta = {
  title: "Patterns/Create record/Form with a date",
  args: { onSubmit: fn() },
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <Story />
      </LocaleProvider>
    ),
  ],
} satisfies Meta<Args>;
export default meta;

type Story = StoryObj<Args>;

const dealFields: CreateField[] = [
  { key: "name", label: "create.dealName", required: true },
  { key: "expected_close_date", label: "create.expectedClose", type: "date" },
];

function DealForm({ onSubmit }: Args) {
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
      onSubmit={onSubmit}
      onClose={() => {}}
      intent="create"
    />
  );
}

export const UnreadableDay: Story = {
  render: (args) => <DealForm onSubmit={args.onSubmit} />,
  play: async ({ args, canvasElement }) => {
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
    await expect(close).toHaveFocus();
    await expect(args.onSubmit).not.toHaveBeenCalled();
  },
};

export const UnreadableDayPhone: Story = {
  ...UnreadableDay,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

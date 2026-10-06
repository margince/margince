// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { Button, Field, TextInput } from "../design-system/atoms";
import { LocaleProvider } from "../i18n";
import { RecordFormDialog } from "./create.dialog";

const meta = {
  title: "Patterns/Create record/Dialog",
  component: RecordFormDialog,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <Story />
      </LocaleProvider>
    ),
  ],
  args: { open: true, title: "New company", onClose: () => undefined },
} satisfies Meta<typeof RecordFormDialog>;
export default meta;

type Story = StoryObj<typeof meta>;

const labels = [
  "Display name",
  "Legal name",
  "Industry",
  "City",
  "Region",
  "Postal code",
  "Country",
  "Website",
];

function fields(count: number) {
  return (
    <form id="story-record-form" className="form-stack">
      {labels.slice(0, count).map((label) => (
        <Field key={label} label={label}>
          {(control) => <TextInput {...control} />}
        </Field>
      ))}
    </form>
  );
}

const actions = (
  <>
    <Button type="button">Cancel</Button>
    <Button variant="primary" type="submit" form="story-record-form">
      Create
    </Button>
  </>
);

export const Form: Story = {
  args: { fieldCount: 4, form: fields(4), actions },
  play: async ({ canvasElement }) => {
    const dialog = await within(canvasElement.ownerDocument.body).findByRole(
      "dialog",
    );
    await expect(dialog.className).toContain("modal-form");
  },
};

export const Drawer: Story = {
  args: { fieldCount: 8, form: fields(8), actions },
  play: async ({ canvasElement }) => {
    const dialog = await within(canvasElement.ownerDocument.body).findByRole(
      "dialog",
    );
    await expect(dialog.className).toContain("modal-drawer");
  },
};

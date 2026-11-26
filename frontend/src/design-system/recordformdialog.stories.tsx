// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import { LocaleProvider } from "../i18n";
import { useShortcutDispatcher } from "../lib/shortcuts";
import { Button, Field, TextInput } from "./atoms";
import { RecordFormDialog } from "./recordformdialog";

const meta = {
  title: "Components/Overlays and layering/Record form dialog",
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

export const FormDark: Story = { ...Form, globals: { theme: "dark" } };

export const DrawerDark: Story = { ...Drawer, globals: { theme: "dark" } };

function CommitDemo() {
  useShortcutDispatcher();
  const [created, setCreated] = useState(false);
  return (
    <>
      <p className="t-body">{created ? "Created" : "Not created yet"}</p>
      <RecordFormDialog
        open
        title="New company"
        onClose={() => undefined}
        fieldCount={8}
        form={
          <form
            id="story-commit-form"
            className="form-stack"
            onSubmit={(event) => {
              event.preventDefault();
              setCreated(true);
            }}
          >
            {labels.map((label) => (
              <Field key={label} label={label}>
                {(control) => <TextInput {...control} />}
              </Field>
            ))}
          </form>
        }
        actions={
          <Button variant="primary" type="submit" form="story-commit-form">
            Create
          </Button>
        }
      />
    </>
  );
}

/** Cmd/Ctrl+Enter from a field creates, in the drawer shape too. */
export const DrawerSubmitsOnCommit: Story = {
  args: { fieldCount: 8, form: fields(8), actions },
  render: () => <CommitDemo />,
  play: async ({ canvasElement }) => {
    const body = within(canvasElement.ownerDocument.body);
    await userEvent.click(await body.findByRole("textbox", { name: "City" }));
    await userEvent.keyboard("{Control>}{Enter}{/Control}");
    await expect(await body.findByText("Created")).toBeInTheDocument();
  },
};

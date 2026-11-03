// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, screen, userEvent, waitFor } from "storybook/test";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { ProblemError } from "../lib/problem";
import { Textarea, TextInput } from "./atoms";
import { DateInput, isoOrBlank } from "./dateinput";
import { EditableValue } from "./editablevalue";
import { FieldEditDialog } from "./fieldeditdialog";
import { FieldGrid, FieldRow } from "./fieldgrid";
import { Select } from "./select";

const meta: Meta = {
  title: "Components/Forms and input/Field edit dialog",
  component: FieldEditDialog,
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

type Kind = "text" | "textarea" | "number" | "date" | "choice";

const INDUSTRIES = [
  { value: "Software", label: "Software" },
  { value: "Retail", label: "Retail" },
  { value: "Manufacturing", label: "Manufacturing" },
];

const STORED: Record<Kind, string> = {
  text: "Acme Robotics GmbH",
  textarea: "Runs three plants in Bavaria and buys service contracts per line.",
  number: "240",
  date: "2026-12-15",
  choice: "Software",
};

const LABEL: Record<Kind, string> = {
  text: "Legal name",
  textarea: "Description",
  number: "Employees",
  date: "Close date",
  choice: "Industry",
};

function Example({
  kind,
  onSave,
}: Readonly<{ kind: Kind; onSave?: (next: string) => Promise<void> }>) {
  const [open, setOpen] = useState(true);
  const [value, setValue] = useState(STORED[kind]);
  const label = LABEL[kind];
  return (
    <FieldGrid>
      <FieldRow label={label}>
        <EditableValue
          editLabel={`Change ${label}`}
          onEdit={() => setOpen(true)}
        >
          {value}
        </EditableValue>
      </FieldRow>
      <FieldEditDialog
        open={open}
        label={label}
        value={value}
        settle={kind === "textarea" ? undefined : (draft) => draft.trim()}
        onClose={() => setOpen(false)}
        onSave={async (next) => {
          await onSave?.(next);
          setValue(next);
        }}
      >
        {({ control, draft, setDraft, saving }) => {
          if (kind === "textarea") {
            return (
              <Textarea
                {...control}
                rows={4}
                value={draft}
                readOnly={saving}
                onChange={(event) => setDraft(event.target.value)}
              />
            );
          }
          if (kind === "date") {
            return (
              <DateInput
                {...control}
                value={isoOrBlank(draft)}
                onChange={(event) => setDraft(event.target.value)}
              />
            );
          }
          if (kind === "choice") {
            return (
              <Select
                {...control}
                value={draft}
                options={INDUSTRIES}
                disabled={saving}
                onChange={setDraft}
              />
            );
          }
          return (
            <TextInput
              {...control}
              type={kind === "number" ? "number" : "text"}
              value={draft}
              readOnly={saving}
              onChange={(event) => setDraft(event.target.value)}
            />
          );
        }}
      </FieldEditDialog>
    </FieldGrid>
  );
}

export const Text: Story = { render: () => <Example kind="text" /> };

export const TextDark: Story = { ...Text, globals: { theme: "dark" } };

export const TextPhone: Story = {
  ...Text,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

export const Paragraph: Story = { render: () => <Example kind="textarea" /> };

export const WholeNumber: Story = { render: () => <Example kind="number" /> };

export const CalendarDay: Story = { render: () => <Example kind="date" /> };

export const Choice: Story = { render: () => <Example kind="choice" /> };

export const Pending: Story = {
  render: () => <Example kind="text" onSave={() => new Promise(() => {})} />,
  play: async () => {
    const user = userEvent.setup();
    const box = await screen.findByRole("textbox", { name: "Legal name" });
    await user.type(box, " AG");
    await user.click(screen.getByRole("button", { name: en["record.save"] }));
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: en["record.save"] }),
      ).toHaveAttribute("aria-busy", "true"),
    );
  },
};

export const Refused: Story = {
  render: () => (
    <Example
      kind="text"
      onSave={async () => {
        throw new ProblemError({
          code: "version_skew",
          detail: "Somebody else changed this record.",
        });
      }}
    />
  ),
  play: async () => {
    const user = userEvent.setup();
    const box = await screen.findByRole("textbox", { name: "Legal name" });
    await user.type(box, " AG");
    await user.click(screen.getByRole("button", { name: en["record.save"] }));
    await waitFor(() =>
      expect(box).toHaveAccessibleDescription(en["common.versionSkew"]),
    );
    await expect(box).toHaveValue("Acme Robotics GmbH AG");
  },
};

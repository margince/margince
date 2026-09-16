// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { LocaleProvider, useT } from "../i18n";
import { contactCreateFields } from "./contactformfields";
import type { FormRow } from "./create";
import { RepeatableRowsField } from "./repeatablerowsfield";

// The repeatable-row field on its own, in the states the row list itself
// decides: a single row disables both move buttons; in a longer list the first
// row loses only move-up, the last only move-down, and every middle row keeps
// both. Each row's move and remove controls carry its ordinal in their
// accessible names ("Move row 3 up", "Remove row 3"), so the list must be long
// enough for those to differ. The field definition is the contact form's real
// phones field, not a story-local copy.
const meta: Meta = {
  title: "Patterns/Create record/Repeatable rows",
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

function PhonesField({ initial }: Readonly<{ initial: FormRow[] }>) {
  const t = useT();
  const phones = contactCreateFields(t).find((field) => field.key === "phones");
  if (!phones) {
    throw new Error(
      "contactCreateFields no longer declares a 'phones' field; point this story at the repeatable field that replaced it",
    );
  }
  const [rows, setRows] = useState(initial);
  return (
    <RepeatableRowsField
      field={phones}
      formId="story-contact"
      rows={rows}
      setRows={setRows}
    />
  );
}

/** One row: nowhere to move, so both reorder buttons are disabled. */
export const SingleRow: Story = {
  render: () => (
    <PhonesField
      initial={[
        { phone: "+49 30 123456", phone_type: "work", is_primary: "true" },
      ]}
    />
  ),
};

/**
 * Four rows: the first disables move-up, the last move-down, the middle two
 * keep both, and the ordinal accessible names run from row 1 to row 4.
 */
export const ReorderableList: Story = {
  render: () => (
    <PhonesField
      initial={[
        { phone: "+49 30 123456", phone_type: "work", is_primary: "true" },
        { phone: "+49 171 555001", phone_type: "mobile" },
        { phone: "+49 30 987654", phone_type: "home" },
        { phone: "+49 89 224466", phone_type: "other" },
      ]}
    />
  ),
};

/** The same list against the dark ground. */
export const ReorderableListDark: Story = {
  ...ReorderableList,
  globals: { theme: "dark" },
};

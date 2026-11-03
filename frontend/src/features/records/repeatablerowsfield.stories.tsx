// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { companyCreateFields } from "../../components/companyform";
import { contactCreateFields } from "../../components/contactformfields";
import type { CreateField, FormRow } from "../../components/create";
import { RepeatableRowsField } from "../../components/repeatablerowsfield";
import {
  type Locale,
  LocaleProvider,
  type Translator,
  useLocale,
  useT,
} from "../../i18n";

// The repeatable-row field on its own, in the states the row list decides.
// A single row disables both move buttons. In a longer list the first row
// loses only move-up, the last only move-down, and middle rows keep both.
// Move and remove controls carry the row's ordinal in their accessible names
// ("Move row 3 up"). The list must be long enough for those to differ.
// The field definitions are the contact and company forms' real ones.
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

type FieldSource = (t: Translator, locale: Locale) => CreateField[];

function RowsField({
  source,
  fieldKey,
  initial,
  labelHidden,
}: Readonly<{
  source: FieldSource;
  fieldKey: string;
  initial: FormRow[];
  labelHidden?: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const field = source(t, locale).find(
    (candidate) => candidate.key === fieldKey,
  );
  if (!field) {
    throw new Error(
      `the form no longer declares a '${fieldKey}' field; point this story at the repeatable field that replaced it`,
    );
  }
  const [rows, setRows] = useState(initial);
  return (
    <RepeatableRowsField
      field={field}
      formId="story-record"
      rows={rows}
      setRows={setRows}
      labelHidden={labelHidden}
    />
  );
}

const fourPhones: FormRow[] = [
  { phone: "+49 30 123456", phone_type: "work", is_primary: "true" },
  { phone: "+49 171 555001", phone_type: "mobile" },
  { phone: "+49 30 987654", phone_type: "home" },
  { phone: "+49 89 224466", phone_type: "other" },
];

/** One row: nowhere to move, so both reorder buttons are disabled. */
export const SingleRow: Story = {
  render: () => (
    <RowsField
      source={contactCreateFields}
      fieldKey="phones"
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
    <RowsField
      source={contactCreateFields}
      fieldKey="phones"
      initial={fourPhones}
    />
  ),
};

/** The same list against the dark ground. */
export const ReorderableListDark: Story = {
  ...ReorderableList,
  globals: { theme: "dark" },
};

/**
 * Inside a dialog titled for the field, the group label is left out.
 * The first row's column labels stay; later rows keep theirs for screen readers.
 */
export const EmailsTitledByDialog: Story = {
  render: () => (
    <RowsField
      source={contactCreateFields}
      fieldKey="emails"
      labelHidden
      initial={[
        { email: "carol@demo.test", email_type: "work", is_primary: "true" },
        { email: "carol.home@demo.test", email_type: "personal" },
      ]}
    />
  ),
};

/** A row with one subfield: the value, then Primary and the row verbs. */
export const DomainsOneSubfield: Story = {
  render: () => (
    <RowsField
      source={companyCreateFields}
      fieldKey="domains"
      initial={[
        { domain: "demo.test", is_primary: "true" },
        { domain: "demo-group.test" },
      ]}
    />
  ),
};

/** A phone-wide column: each row folds to the value over its details. */
export const NarrowColumn: Story = {
  decorators: [
    (Story) => (
      <div style={{ maxInlineSize: "19.125rem" }}>
        <Story />
      </div>
    ),
  ],
  render: () => (
    <RowsField
      source={contactCreateFields}
      fieldKey="phones"
      initial={fourPhones.slice(0, 2)}
    />
  ),
};

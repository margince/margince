// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import { LocaleProvider } from "../i18n";
import { workEmailDomain } from "./contactformfields";
import type { CreateField, FormRows } from "./create.fields";
import { RecordFormBody } from "./create.form";
import type { NameOffer } from "./create.offered";

// An offered name box that a typed work email prefills, while it is untouched.

const NORDLICHT: NameOffer = {
  value: "01930000-0000-7000-8000-0000000000o2",
  label: "Nordlicht Logistik AG",
};

async function offersFor(query: string): Promise<readonly NameOffer[]> {
  return query === "" ? [] : [NORDLICHT];
}

const fields: CreateField[] = [
  { key: "full_name", label: "create.fullName", required: true },
  {
    key: "company_name",
    label: "create.companyName",
    offers: {
      search: offersFor,
      pickedKey: "company_id",
      pickedHint: "create.companyPicked",
      newHint: "create.companyNew",
      prefill: {
        from: workEmailDomain,
        find: offersFor,
        hint: "create.companyPrefilled",
      },
    },
  },
  {
    key: "emails",
    label: "create.email",
    type: "repeatable",
    addLabel: "field.addEmail",
    rowFields: [
      { key: "email", label: "create.email", type: "email", required: true },
    ],
    primaryKey: "is_primary",
  },
];

function PrefillForm() {
  const [values, setValues] = useState<Record<string, string>>({
    full_name: "Dana Buyer",
  });
  const [rows, setRows] = useState<FormRows>({});
  return (
    <RecordFormBody
      fields={fields}
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

const meta = {
  title: "Patterns/Create record/Prefilled company",
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <Story />
      </LocaleProvider>
    ),
  ],
  render: () => <PrefillForm />,
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

export const Untouched: Story = {};

export const PrefilledFromWorkEmail: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const user = userEvent.setup();
    await user.click(canvas.getByText("Add email"));
    await user.type(
      canvas.getByLabelText("Email *"),
      "dana.buyer@nordlicht-logistik.de",
    );
    await expect(
      await canvas.findByText(
        "Matched to an existing company by the work email domain.",
      ),
    ).toBeVisible();
    await expect(canvas.getByRole("combobox", { name: "Company" })).toHaveValue(
      "Nordlicht Logistik AG",
    );
  },
};

export const PrefilledFromWorkEmailPhone: Story = {
  ...PrefilledFromWorkEmail,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

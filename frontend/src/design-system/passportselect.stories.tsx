// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { LocaleProvider } from "../i18n";
import {
  type PassportOption,
  PassportSelect,
  ScopeChips,
} from "./passportselect";

const meta: Meta<typeof PassportSelect> = {
  title: "Design System/Passport select",
  component: PassportSelect,
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
type Story = StoryObj<typeof PassportSelect>;

const PASSPORTS: readonly PassportOption[] = [
  {
    id: "crm-read",
    label: "CRM read",
    scopes: ["companies:read", "contacts:read", "deals:read"],
  },
  {
    id: "crm-write",
    label: "CRM read and write",
    scopes: [
      "companies:read",
      "companies:write",
      "contacts:read",
      "contacts:write",
      "deals:read",
      "deals:write",
      "activities:write",
    ],
  },
  { id: "billing", label: "Billing only", scopes: ["invoices:read"] },
];

function Live(props: Readonly<{ allowEmpty?: boolean; initial?: string }>) {
  const [value, setValue] = useState(props.initial ?? "");
  return (
    <PassportSelect
      options={PASSPORTS}
      value={value}
      onChange={setValue}
      allowEmpty={props.allowEmpty}
      emptyLabel={props.allowEmpty ? "All passports" : undefined}
      ariaLabel="Passport"
    />
  );
}

/**
 * The consent screen's shape: a passport is REQUIRED, so there is no empty
 * choice to fall back to. Picking one shows the scopes it carries, because the
 * name alone is not what a reader is consenting to.
 */
export const RequiresAPassport: Story = {
  render: () => <Live initial="crm-read" />,
};

/**
 * The tool console's shape. The empty choice is an OPTION rather than the
 * select's placeholder: a placeholder is only a face, and this reader has to be
 * able to come BACK to "all passports" after picking one.
 */
export const AllowsAllPassports: Story = {
  render: () => <Live allowEmpty />,
};

/**
 * The chips on their own. A chip means one thing on both surfaces it appears
 * on — a connection receives the scopes of the passport lent to it, so there is
 * no "granted versus not" distinction to draw and none is drawn.
 */
export const Scopes: Story = {
  render: () => (
    <p>
      <ScopeChips labels={PASSPORTS[1].scopes} />
    </p>
  ),
};

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { type BillingContact, BillingContactsPanel } from "./billingcontacts";
import { ContactBillingRoles } from "./contactbillingroles";
import { StoryProviders } from "./story-utils";

// The three states this panel has to keep apart. WITHHELD renders nothing,
// EMPTY says nobody is named, and a populated list reads in invoice order.
// The first two look identical in a screenshot of the record page, which is
// why they are pinned here side by side.

const meta: Meta = {
  title: "Records/Company 360/Billing contacts",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const pat: BillingContact = {
  relationship_id: "r-1",
  contact_id: "c-1",
  full_name: "Pat Okafor",
  role: "recipient",
  title: "Office Manager",
  email: "pat@acme.test",
};

export const Populated: Story = {
  render: () => (
    <StoryProviders>
      <BillingContactsPanel
        contacts={[
          pat,
          {
            ...pat,
            relationship_id: "r-2",
            contact_id: "c-2",
            full_name: "Sam Rivera",
            role: "approver",
            email: "sam@acme.test",
          },
          {
            ...pat,
            relationship_id: "r-3",
            contact_id: "c-3",
            full_name: "Accounts Payable",
            role: "accounts_payable",
            email: null,
          },
        ]}
      />
    </StoryProviders>
  ),
};

// A paying customer with nobody named. A gap worth showing rather than a
// blank panel, because somebody has to decide where the invoice goes.
export const NobodyNamed: Story = {
  render: () => (
    <StoryProviders>
      <BillingContactsPanel contacts={[]} />
    </StoryProviders>
  ),
};

// The contact's side, beside their employers rather than inside them.
export const OnAContact: Story = {
  render: () => (
    <StoryProviders>
      <ContactBillingRoles
        companies={[
          {
            relationship_id: "r-9",
            company_id: "o-1",
            company_name: "Acme",
            role: "approver",
          },
          {
            relationship_id: "r-10",
            company_id: "o-2",
            company_name: "Globex",
            role: "recipient",
          },
        ]}
      />
    </StoryProviders>
  ),
};

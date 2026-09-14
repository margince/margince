// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { ContactBillingRoles } from "./contactbillingroles";
import { StoryProviders } from "./story-utils";
import "./company360.css";

// The contact's side of billing, beside their employers rather than inside
// them: whose invoices this contact handles, one company per row, the role as
// a neutral badge. An external bookkeeper is the case worth drawing — three
// companies, none of them the contact's employer. Withheld and empty both
// render nothing, so neither has a story.

const meta: Meta<typeof ContactBillingRoles> = {
  title: "Records/Contact record/Billing roles",
  component: ContactBillingRoles,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof ContactBillingRoles>;

function roles() {
  return (
    <StoryProviders>
      <div style={{ maxWidth: 420 }}>
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
            {
              relationship_id: "r-11",
              company_id: "o-3",
              company_name: "Initech",
              role: "accounts_payable",
            },
          ]}
        />
      </div>
    </StoryProviders>
  );
}

export const OnAContact: Story = { render: roles };

export const OnAContactDark: Story = {
  globals: { theme: "dark" },
  render: roles,
};

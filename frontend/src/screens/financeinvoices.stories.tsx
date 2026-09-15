// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { RecentInvoices } from "./financeinvoices";
import { StoryProviders } from "./story-utils";
import "./company360.css";

// The finance card's invoice table on its own, one row per status family, so
// the status column can be read down its length: neutral for the states that
// ask nothing of a reader, warn for the ones that need a look, danger for money
// past due, success for settled. How late folds into the same badge rather than
// a caption beside it.

type FinanceSummary = components["schemas"]["CompanyFinanceSummary"];
type FinanceInvoice = components["schemas"]["FinanceInvoice"];

function invoice(over: Partial<FinanceInvoice>): FinanceInvoice {
  return {
    id: "inv-1",
    number: "RE-2026-0512",
    issued_at: "2026-07-01",
    due_at: "2026-07-31",
    status: "open",
    currency: "EUR",
    gross_minor: 155_000,
    open_minor: 155_000,
    ...over,
  };
}

const summary: FinanceSummary = {
  company_id: "o-1",
  state: "connected",
  provider: "offline_demo",
  last_synced_at: "2026-08-10T06:00:00Z",
  recent_invoices: [
    invoice({ id: "inv-1", number: "RE-2026-0540", status: "open" }),
    invoice({
      id: "inv-2",
      number: "RE-2026-0533",
      status: "overdue",
      gross_minor: 89_000,
      open_minor: 89_000,
      days_late: 2,
    }),
    invoice({
      id: "inv-3",
      number: "RE-2026-0521",
      status: "partially_paid",
      gross_minor: 42_000,
      open_minor: 12_000,
    }),
    invoice({
      id: "inv-4",
      number: "RE-2026-0512",
      status: "paid",
      paid_at: "2026-08-04T00:00:00Z",
      open_minor: 0,
      days_late: 4,
    }),
    invoice({
      id: "inv-5",
      number: null,
      status: "draft",
      due_at: null,
      gross_minor: 7_500,
    }),
  ],
};

const meta: Meta<typeof RecentInvoices> = {
  title: "Records/Company 360/Recent invoices",
  component: RecentInvoices,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof RecentInvoices>;

export const EveryStatus: Story = {
  render: () => (
    <StoryProviders>
      <div style={{ maxWidth: 560 }}>
        <RecentInvoices summary={summary} />
      </div>
    </StoryProviders>
  ),
};

// Dark, because the overdue row's tint and the danger badge inside it are two
// derivations of one hue that the dark lift moves together.
export const EveryStatusDark: Story = {
  globals: { theme: "dark" },
  render: EveryStatus.render,
};

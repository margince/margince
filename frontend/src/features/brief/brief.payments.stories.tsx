// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import {
  customerPayments,
  noCustomerPayments,
} from "../../testing/customerpayments.fixtures";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../../testing/story-utils";
import { CustomerPaymentsCard } from "./brief.payments";
import "./brief.css";
import { en as englishCopy } from "../../i18n/en";

const meta: Meta<typeof CustomerPaymentsCard> = {
  title: "Shell/Home/Customer payments",
  component: CustomerPaymentsCard,
};
export default meta;
type Story = StoryObj<typeof CustomerPaymentsCard>;

function payments(over: Parameters<typeof customerPayments>[0] = {}) {
  stubWithSession(
    {
      "GET /finance/customer-payments": () =>
        jsonResponse(customerPayments(over)),
    },
    { finance: ["read"], company: ["read"] },
  );
  return (
    <StoryProviders>
      <CustomerPaymentsCard />
    </StoryProviders>
  );
}

export const Overdue: Story = { render: () => payments() };
export const OverdueDark: Story = { ...Overdue, globals: { theme: "dark" } };
export const Phone: Story = {
  ...Overdue,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
export const Quiet: Story = {
  render: () =>
    payments({
      invoice_count: 0,
      company_count: 0,
      totals: [],
      customers: [],
      data: [],
    }),
};
export const FirstSync: Story = {
  render: () =>
    payments({
      state: "syncing",
      last_synced_at: undefined,
      invoice_count: 0,
      company_count: 0,
      totals: [],
      customers: [],
      data: [],
    }),
};
export const Stale: Story = { render: () => payments({ state: "stale" }) };
export const SyncFailed: Story = { render: () => payments({ state: "error" }) };
export const NotConnected: Story = {
  render: () => payments(noCustomerPayments),
};
export const SeveralCurrencies: Story = {
  render: () =>
    payments({
      totals: [
        { currency: "EUR", amount_minor: 1840000 },
        { currency: "USD", amount_minor: 720000 },
      ],
      customers: customerPayments().customers.map((customer) => ({
        ...customer,
        company_name:
          "A customer with a long company name and regional branches across several countries",
      })),
    }),
};
export const InvoiceDrawer: Story = {
  render: () => payments(),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", {
        name: englishCopy["payments.viewInvoices"],
      }),
    );
    const drawer = await within(document.body).findByRole("dialog", {
      name: englishCopy["payments.viewInvoices"],
    });
    await waitFor(() => expect(drawer).toBeVisible());
  },
};
export const InvoiceDrawerPhone: Story = {
  ...InvoiceDrawer,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

export const SeveralCurrenciesPhone: Story = {
  ...SeveralCurrencies,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

export const LongOverdue: Story = {
  render: () => {
    const current = customerPayments();
    const invoice = current.data[0];
    if (!invoice) throw new Error("the payment story needs one invoice");
    return payments({
      invoice_count: 2,
      totals: [{ currency: "EUR", amount_minor: 5640000 }],
      long_overdue_days: 365,
      long_overdue: {
        invoice_count: 1,
        totals: [{ currency: "EUR", amount_minor: 3800000 }],
        customers: [],
      },
      data: [
        invoice,
        {
          ...invoice,
          invoice_id: "older-invoice",
          number: "OLD-001",
          due_at: "2025-01-01",
          days_overdue: 620,
          open_minor: 3800000,
        },
      ],
    });
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const label = englishCopy["payments.longOverdue"].replace("{count}", "1");
    expect(
      (await canvas.findByText(label)).closest("details"),
    ).not.toHaveAttribute("open");
    await userEvent.click(
      canvas.getByRole("button", {
        name: englishCopy["payments.viewInvoices"],
      }),
    );
    const drawer = within(
      await within(document.body).findByRole("dialog", {
        name: englishCopy["payments.viewInvoices"],
      }),
    );
    expect(drawer.getByText("OLD-001")).not.toBeVisible();
    await userEvent.click(drawer.getByText(label));
    await waitFor(() => expect(drawer.getByText("OLD-001")).toBeVisible());
  },
};

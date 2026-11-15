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
      await canvas.findByRole("button", { name: "View overdue invoices" }),
    );
    const drawer = await within(document.body).findByRole("dialog", {
      name: "View overdue invoices",
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

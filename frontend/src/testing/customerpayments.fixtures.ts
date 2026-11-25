// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

type Payments = components["schemas"]["CustomerPayments"];

export function customerPayments(over: Partial<Payments> = {}): Payments {
  return {
    access_scope: "demo",
    state: "connected",
    provider: "offline_demo",
    provider_name: "Demo accounting",
    last_synced_at: "2026-09-13T08:00:00Z",
    linked_companies: 3,
    company_count: 1,
    invoice_count: 1,
    totals: [{ currency: "EUR", amount_minor: 1840000 }],
    customers: [
      {
        company_id: "00000000-0000-4000-8000-000000000011",
        company_name: "Acme",
        currency: "EUR",
        amount_minor: 1840000,
        invoice_count: 1,
        oldest_days: 24,
      },
    ],
    data: [
      {
        invoice_id: "00000000-0000-4000-8000-000000000021",
        company_id: "00000000-0000-4000-8000-000000000011",
        company_name: "Acme",
        currency: "EUR",
        open_minor: 1840000,
        due_at: "2026-08-20",
        days_overdue: 24,
        number: "INV-001",
      },
    ],
    page: { has_more: false },
    ...over,
  };
}

export const noCustomerPayments: Payments = {
  access_scope: "disconnected",
  state: "no_connection",
  linked_companies: 0,
  company_count: 0,
  invoice_count: 0,
  totals: [],
  customers: [],
  data: [],
  page: { has_more: false },
};

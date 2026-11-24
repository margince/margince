/** @vitest-environment happy-dom */
import { focusManager } from "@tanstack/react-query";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { jsonResponse, render, stubApi } from "../../testing/brief.testkit";
import {
  customerPayments,
  noCustomerPayments,
} from "../../testing/customerpayments.fixtures";
import { meFixture } from "../../testing/mefixture";
import { CustomerPaymentsCard } from "./brief.payments";

const allowed = meFixture({ allow: { company: ["read"], finance: ["read"] } });

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function servePayments(answer = customerPayments()) {
  return stubApi({
    "GET /me": () => jsonResponse(allowed),
    "GET /finance/customer-payments": () => jsonResponse(answer),
  });
}

it("hides disconnected accounting instead of putting ledger setup on Home", async () => {
  servePayments(noCustomerPayments);
  const { container, client } = render(<CustomerPaymentsCard />);
  await waitFor(() => expect(client.isFetching()).toBe(0));
  expect(container.textContent).toBe("");
});

it("does not request financial figures without both read grants", async () => {
  const calls = stubApi({
    "GET /me": () => jsonResponse(meFixture({ allow: { company: ["read"] } })),
  });
  const { container, client } = render(<CustomerPaymentsCard />);
  await waitFor(() => expect(client.isFetching()).toBe(0));
  expect(container.textContent).toBe("");
  expect(calls.some((call) => call.path.includes("customer-payments"))).toBe(
    false,
  );
});

it("shows payment totals and opens the invoice list with a route to company finance", async () => {
  servePayments();
  const user = userEvent.setup();
  render(<CustomerPaymentsCard />);
  await screen.findByText(/18,400.00.*overdue/);
  expect(screen.getByText("1 invoice · 1 customer")).toBeTruthy();
  expect(screen.getByText("Oldest invoice 24 days overdue")).toBeTruthy();
  expect(
    screen.getByRole("link", { name: "Acme" }).getAttribute("href"),
  ).toContain("/finance");
  await user.click(
    screen.getByRole("button", { name: en["payments.viewInvoices"] }),
  );
  const drawer = await screen.findByRole("dialog", {
    name: en["payments.viewInvoices"],
  });
  expect(within(drawer).getByText("INV-001")).toBeTruthy();
  expect(within(drawer).getByRole("link", { name: "Acme" })).toBeTruthy();
});

it("keeps currency totals separate and calls a confirmed empty ledger quiet", async () => {
  servePayments(
    customerPayments({
      totals: [
        { currency: "EUR", amount_minor: 50000 },
        { currency: "USD", amount_minor: 90000 },
      ],
    }),
  );
  const { unmount } = render(<CustomerPaymentsCard />);
  await screen.findByText(/500.00.*overdue/);
  expect(screen.getByText(/900.00.*overdue/)).toBeTruthy();
  expect(screen.queryByText(/1,400/)).toBeNull();
  unmount();
  servePayments(
    customerPayments({
      invoice_count: 0,
      company_count: 0,
      totals: [],
      customers: [],
      data: [],
    }),
  );
  render(<CustomerPaymentsCard />);
  expect(await screen.findByText(en["payments.none"])).toBeTruthy();
});

it("labels first sync and failed sync without inventing a zero", async () => {
  servePayments(
    customerPayments({
      state: "syncing",
      last_synced_at: undefined,
      invoice_count: 0,
      company_count: 0,
      totals: [],
      customers: [],
      data: [],
    }),
  );
  const { unmount } = render(<CustomerPaymentsCard />);
  await screen.findByText(en["finance.syncing"]);
  expect(screen.queryByText(en["payments.none"])).toBeNull();
  unmount();
  servePayments(customerPayments({ state: "error" }));
  render(<CustomerPaymentsCard />);
  await screen.findByText(en["payments.syncFailed"]);
  expect(screen.getByText(/18,400.00.*overdue/)).toBeTruthy();
  expect(screen.getByText(/synced/)).toBeTruthy();
});

it("paginates invoices and replaces the whole read after an access refusal", async () => {
  let fail = false;
  stubApi({
    "GET /me": () => jsonResponse(allowed),
    "GET /finance/customer-payments": (_body, query) => {
      if (fail)
        return jsonResponse(
          { code: "permission_denied", title: "Forbidden", status: 403 },
          403,
        );
      return jsonResponse(
        query.has("cursor")
          ? customerPayments({
              data: customerPayments().data.map((invoice) => ({
                ...invoice,
                invoice_id: "00000000-0000-4000-8000-000000000022",
                number: "INV-002",
              })),
            })
          : customerPayments({ page: { has_more: true, next_cursor: "next" } }),
      );
    },
  });
  const user = userEvent.setup();
  const { container, client } = render(<CustomerPaymentsCard />);
  await user.click(
    await screen.findByRole("button", { name: en["payments.viewInvoices"] }),
  );
  await user.click(screen.getByRole("button", { name: en["list.loadMore"] }));
  await screen.findByText("INV-002");
  expect(screen.getByText("INV-001")).toBeTruthy();
  fail = true;
  await client.invalidateQueries({ queryKey: ["customer-payments"] });
  await waitFor(() => expect(container.textContent).toBe(""));
  expect(screen.queryByText("INV-001")).toBeNull();
});

function serveTwoPages(nextPage: () => Response) {
  stubApi({
    "GET /me": () => jsonResponse(allowed),
    "GET /finance/customer-payments": (_body, query) =>
      query.has("cursor")
        ? nextPage()
        : jsonResponse(
            customerPayments({ page: { has_more: true, next_cursor: "next" } }),
          ),
  });
}

it("keeps the drawer and its rows when Load more fails, and retries that page", async () => {
  let fail = true;
  serveTwoPages(() =>
    fail
      ? jsonResponse(
          { code: "internal", title: "Unavailable", status: 500 },
          500,
        )
      : jsonResponse(
          customerPayments({
            data: customerPayments().data.map((invoice) => ({
              ...invoice,
              invoice_id: "00000000-0000-4000-8000-000000000022",
              number: "INV-002",
            })),
          }),
        ),
  );
  const user = userEvent.setup();
  render(<CustomerPaymentsCard />);
  await user.click(
    await screen.findByRole("button", { name: en["payments.viewInvoices"] }),
  );
  await user.click(screen.getByRole("button", { name: en["list.loadMore"] }));
  const drawer = screen.getByRole("dialog", {
    name: en["payments.viewInvoices"],
  });
  expect(await within(drawer).findByRole("alert")).toBeTruthy();
  expect(within(drawer).getByText("INV-001")).toBeTruthy();
  expect(screen.queryByText(en["common.error"])).toBeNull();
  fail = false;
  await user.click(
    within(drawer).getByRole("button", { name: en["list.loadMore"] }),
  );
  expect(await within(drawer).findByText("INV-002")).toBeTruthy();
  expect(within(drawer).queryByRole("alert")).toBeNull();
});

it("starts the invoice list over when the accounting connection was replaced", async () => {
  let firstPages = 0;
  stubApi({
    "GET /me": () => jsonResponse(allowed),
    "GET /finance/customer-payments": (_body, query) => {
      if (query.has("cursor"))
        return jsonResponse(
          {
            code: "validation_error",
            title: "Unprocessable",
            status: 422,
            details: {
              errors: [
                {
                  field: "cursor",
                  code: "cursor_param_mismatch",
                  message: "the accounting source changed; restart the list",
                },
              ],
            },
          },
          422,
        );
      firstPages++;
      return jsonResponse(
        customerPayments({
          page: { has_more: firstPages === 1, next_cursor: "next" },
        }),
      );
    },
  });
  const user = userEvent.setup();
  render(<CustomerPaymentsCard />);
  await user.click(
    await screen.findByRole("button", { name: en["payments.viewInvoices"] }),
  );
  await user.click(screen.getByRole("button", { name: en["list.loadMore"] }));
  await waitFor(() => expect(firstPages).toBe(2));
  const drawer = await screen.findByRole("dialog", {
    name: en["payments.viewInvoices"],
  });
  expect(await within(drawer).findByText("INV-001")).toBeTruthy();
  await waitFor(() => expect(within(drawer).queryByRole("alert")).toBeNull());
  expect(
    within(drawer).queryByRole("button", { name: en["list.loadMore"] }),
  ).toBeNull();
});

it("hides every figure when Load more is refused for access", async () => {
  serveTwoPages(() =>
    jsonResponse(
      { code: "permission_denied", title: "Forbidden", status: 403 },
      403,
    ),
  );
  const user = userEvent.setup();
  const { container } = render(<CustomerPaymentsCard />);
  await user.click(
    await screen.findByRole("button", { name: en["payments.viewInvoices"] }),
  );
  await user.click(screen.getByRole("button", { name: en["list.loadMore"] }));
  await waitFor(() => expect(container.textContent).toBe(""));
  expect(screen.queryByText("INV-001")).toBeNull();
});

it("drops cached amounts as soon as the reader's authorization changes", async () => {
  let refuse = false;
  stubApi({
    "GET /me": () => jsonResponse(allowed),
    "GET /finance/customer-payments": () =>
      refuse
        ? new Promise<Response>(() => {})
        : jsonResponse(customerPayments()),
  });
  const { client } = render(<CustomerPaymentsCard />);
  await screen.findByText(/18,400.00.*overdue/);
  refuse = true;
  client.setQueryData(["me"], meFixture({ allow: { company: ["read"] } }));
  await waitFor(() =>
    expect(screen.queryByText(/18,400.00.*overdue/)).toBeNull(),
  );
  expect(
    screen.queryByRole("heading", { name: en["payments.title"] }),
  ).toBeNull();
});

it("discards earlier invoice pages when the readable account set changes", async () => {
  let narrowed = false;
  const narrower = customerPayments({
    access_scope: "narrower",
    totals: [{ currency: "EUR", amount_minor: 50000 }],
    customers: customerPayments().customers.map((customer) => ({
      ...customer,
      company_name: "Still visible",
      amount_minor: 50000,
    })),
    data: customerPayments().data.map((invoice) => ({
      ...invoice,
      company_name: "Still visible",
      number: "VISIBLE-002",
      open_minor: 50000,
    })),
  });
  stubApi({
    "GET /me": () => jsonResponse(allowed),
    "GET /finance/customer-payments": (_body, query) => {
      if (query.has("cursor")) narrowed = true;
      return jsonResponse(
        narrowed
          ? narrower
          : customerPayments({ page: { has_more: true, next_cursor: "next" } }),
      );
    },
  });
  const user = userEvent.setup();
  render(<CustomerPaymentsCard />);
  await user.click(
    await screen.findByRole("button", { name: en["payments.viewInvoices"] }),
  );
  expect(screen.getByText("INV-001")).toBeTruthy();
  await user.click(screen.getByRole("button", { name: en["list.loadMore"] }));
  await screen.findByText("VISIBLE-002");
  await waitFor(() => expect(screen.queryByText("INV-001")).toBeNull());
  expect(screen.queryByText(/18,400.00.*overdue/)).toBeNull();
  expect(screen.queryByRole("link", { name: "Acme" })).toBeNull();
  expect(screen.getByText(/500.00.*overdue/)).toBeTruthy();
});

it("starts with one fresh page on focus instead of replaying the whole ledger", async () => {
  let requests = 0;
  stubApi({
    "GET /me": () => jsonResponse(allowed),
    "GET /finance/customer-payments": (_body, query) => {
      requests++;
      return jsonResponse(
        query.has("cursor")
          ? customerPayments({ data: [] })
          : customerPayments({ page: { has_more: true, next_cursor: "next" } }),
      );
    },
  });
  const user = userEvent.setup();
  const { client } = render(<CustomerPaymentsCard />);
  await user.click(
    await screen.findByRole("button", { name: en["payments.viewInvoices"] }),
  );
  await user.click(screen.getByRole("button", { name: en["list.loadMore"] }));
  await waitFor(() => expect(client.isFetching()).toBe(0));
  expect(requests).toBe(2);
  focusManager.setFocused(false);
  focusManager.setFocused(true);
  await waitFor(() => expect(requests).toBe(3));
  await waitFor(() => expect(client.isFetching()).toBe(0));
  expect(requests).toBe(3);
});

it("does not introduce an accounting panel when the first read fails", async () => {
  stubApi({
    "GET /me": () => jsonResponse(allowed),
    "GET /finance/customer-payments": () =>
      jsonResponse(
        { code: "internal", title: "Unavailable", status: 500 },
        500,
      ),
  });
  const { container, client } = render(<CustomerPaymentsCard />);
  await waitFor(() => expect(client.isFetching()).toBe(0));
  expect(container.textContent).toBe("");
});

it("names the accounting source the way a reader knows it, never by its key", async () => {
  servePayments(
    customerPayments({
      provider: "offline_demo",
      provider_name: "Demo accounting",
      last_synced_at: "2026-11-02T09:00:00Z",
    }),
  );
  render(<CustomerPaymentsCard />);
  expect(
    await screen.findByText(/^From Demo accounting · synced/),
  ).toBeTruthy();
  expect(screen.queryByText(/offline_demo/)).toBeNull();
});

it("names no source when the source declares no name", async () => {
  servePayments(
    customerPayments({
      provider: "offline_demo",
      provider_name: undefined,
      last_synced_at: "2026-11-02T09:00:00Z",
    }),
  );
  render(<CustomerPaymentsCard />);
  expect(
    await screen.findByText(/^From your accounting system · synced/),
  ).toBeTruthy();
  expect(screen.queryByText(/offline_demo/)).toBeNull();
});

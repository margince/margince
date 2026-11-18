/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pickOption } from "../../design-system/select-testing";
import { LocaleProvider } from "../../i18n";
import { OfferScreen } from "./offers";

// The draft offer's line editor as a user drives it: what each row lets a
// reader see and change, and what the page shows when the server says no.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function renderOffer() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <OfferScreen id="o-1" />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

const line1 = {
  id: "li-1",
  position: 1,
  product_id: null,
  description: "Consulting hours",
  unit: "hour",
  quantity: 1,
  unit_price_minor: 1000,
  discount_pct: 0,
  tax_rate: 19,
  line_net_minor: 1000,
  line_tax_minor: 190,
  line_total_minor: 1190,
  evidence: null,
  price_grounded: true,
  billing_model: null,
  billing_interval_months: null,
  interval_count: null,
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

const line2 = { ...line1, id: "li-2", position: 2, description: "Workshop" };

function draft(lines: Record<string, unknown>[]) {
  return {
    id: "o-1",
    deal_id: "d-1",
    offer_number: "ANG-2026-0007",
    revision: 1,
    status: "draft",
    currency: "EUR",
    buyer_company_id: null,
    valid_until: "2026-08-01",
    intro_text: null,
    terms_text: null,
    net_minor: 2000,
    tax_minor: 380,
    gross_minor: 2380,
    template_id: null,
    line_items: lines,
    source: "manual",
    captured_by: "human:u1",
    version: 3,
    created_at: "2026-06-01T08:00:00Z",
    updated_at: "2026-06-01T08:00:00Z",
  };
}

type Call = { method: string; url: string; body: unknown };
type Answer = { status: number; body: unknown };

// GETs read the offer; each line write is recorded and answered by `answer`.
function serve(offer: unknown, answer: (call: Call) => Answer): Call[] {
  const calls: Call[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = String(request ? request.url : input);
      const method = request ? request.method : (init?.method ?? "GET");
      const json = (body: unknown, status = 200) =>
        new Response(JSON.stringify(body), {
          status,
          headers: { "Content-Type": "application/json" },
        });
      if (url.includes("/line-items") && method !== "GET") {
        const text = request ? await request.text() : String(init?.body ?? "");
        const call = { method, url, body: text ? JSON.parse(text) : null };
        calls.push(call);
        const { status, body } = answer(call);
        return json(body, status);
      }
      if (url.includes("/offers/")) {
        return json(offer);
      }
      return json({ data: [], page: { has_more: false } });
    }),
  );
  return calls;
}

describe("the Add line form", () => {
  it("starts the next line unclassified once a recurring line is added", async () => {
    const calls = serve(draft([line1]), () => ({
      status: 201,
      body: draft([line1]),
    }));
    const user = userEvent.setup();
    renderOffer();
    await screen.findByText("ANG-2026-0007");

    const billing = () => screen.getByRole("combobox", { name: "Billing" });
    await pickOption(user, billing(), "Recurring");
    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Billing period" }),
      "Yearly",
    );
    await user.type(screen.getByRole("spinbutton", { name: "Periods" }), "2");
    await user.type(screen.getByTestId("new-line-description"), "Support");
    await user.type(screen.getByTestId("new-line-quantity"), "1");
    await user.click(screen.getByTestId("add-line"));

    await waitFor(() => expect(calls).toHaveLength(1));
    expect(calls[0].body).toMatchObject({
      billing_model: "recurring",
      billing_interval_months: 12,
      interval_count: 2,
    });
    await waitFor(() => expect(billing().textContent).toBe("Not specified"));
    expect(
      screen.queryByRole("combobox", { name: "Billing period" }),
    ).toBeNull();
    expect(screen.queryByRole("spinbutton", { name: "Periods" })).toBeNull();
  });
});

describe("a line row", () => {
  it("names each control after its column and its line", async () => {
    serve(draft([line1, line2]), () => ({ status: 500, body: {} }));
    renderOffer();
    await screen.findByText("ANG-2026-0007");

    for (const column of [
      "Position",
      "Description",
      "Unit",
      "Quantity",
      "Unit price",
      "Discount %",
      "Tax %",
    ]) {
      expect(screen.getByLabelText(`${column}, line 2`)).toBeTruthy();
    }
    expect(screen.getByRole("button", { name: "Remove line 2" })).toBe(
      screen.getByTestId("remove-line-li-2"),
    );
  });
});

describe("a refused line edit", () => {
  const reason = "quantity must be greater than 0";
  const refusal = {
    status: 422,
    body: {
      title: "Unprocessable Entity",
      detail: reason,
      code: "validation_error",
    },
  };

  it("puts the saved value back and keeps the reason in that row until the line saves", async () => {
    let refuseLine1 = true;
    const saved = { ...line1, quantity: 2, version: 2 };
    const renamed = { ...line2, description: "Workshop day", version: 2 };
    serve(draft([line1, line2]), (call) => {
      if (call.url.endsWith("/li-2")) {
        return { status: 200, body: draft([line1, renamed]) };
      }
      return refuseLine1
        ? refusal
        : { status: 200, body: draft([saved, renamed]) };
    });
    const user = userEvent.setup();
    renderOffer();
    await screen.findByText("ANG-2026-0007");
    const quantity = () => screen.getByTestId("line-quantity-li-1");
    const row1 = () => screen.getByTestId("line-row-li-1");

    await user.clear(quantity());
    await user.type(quantity(), "0");
    await user.tab();
    expect(await within(row1()).findByText(reason)).toBeTruthy();
    expect(quantity()).toHaveProperty("value", "1");

    const description = screen.getByTestId("line-description-li-2");
    await user.type(description, " day");
    await user.tab();
    await waitFor(() =>
      expect(screen.getByTestId("line-description-li-2")).toHaveProperty(
        "value",
        "Workshop day",
      ),
    );
    expect(screen.getAllByText(reason)).toHaveLength(1);
    expect(within(row1()).getByText(reason)).toBeTruthy();

    refuseLine1 = false;
    await user.clear(quantity());
    await user.type(quantity(), "2");
    await user.tab();
    await waitFor(() => expect(screen.queryByText(reason)).toBeNull());
    expect(quantity()).toHaveProperty("value", "2");
  });

  it("shows a refused removal in its row and leaves the line in place", async () => {
    serve(draft([line1, line2]), () => refusal);
    const user = userEvent.setup();
    renderOffer();
    await screen.findByText("ANG-2026-0007");

    await user.click(screen.getByRole("button", { name: "Remove line 2" }));
    expect(
      await within(screen.getByTestId("line-row-li-2")).findByText(reason),
    ).toBeTruthy();
    expect(
      within(screen.getByTestId("line-row-li-1")).queryByText(reason),
    ).toBeNull();
  });
});

describe("a line's billing", () => {
  it("lets a recurring line with no term take its periods in its row", async () => {
    const quarterly = {
      ...line1,
      billing_model: "recurring",
      billing_interval_months: 3,
      interval_count: null,
    };
    const calls = serve(draft([quarterly]), () => ({
      status: 200,
      body: draft([quarterly]),
    }));
    const user = userEvent.setup();
    renderOffer();
    await screen.findByText("ANG-2026-0007");

    const billing = screen.getByRole("combobox", { name: "Billing, line 1" });
    expect(billing.textContent).toBe("Recurring");
    expect(
      screen.getByRole("combobox", { name: "Billing period, line 1" })
        .textContent,
    ).toBe("Quarterly");
    await user.type(
      screen.getByRole("spinbutton", { name: "Periods, line 1" }),
      "4",
    );
    await user.tab();
    await waitFor(() => expect(calls).toHaveLength(1));
    expect(calls[0].body).toEqual({
      billing_model: "recurring",
      billing_interval_months: 3,
      interval_count: 4,
    });

    await pickOption(user, billing, "One-time");
    await waitFor(() => expect(calls).toHaveLength(2));
    expect(calls[1].body).toEqual({
      billing_model: "one_time",
      billing_interval_months: null,
      interval_count: null,
    });
  });

  it("saves a line made recurring only once it has a period", async () => {
    const calls = serve(draft([line1]), () => ({
      status: 200,
      body: draft([line1]),
    }));
    const user = userEvent.setup();
    renderOffer();
    await screen.findByText("ANG-2026-0007");

    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Billing, line 1" }),
      "Recurring",
    );
    expect(calls).toHaveLength(0);
    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Billing period, line 1" }),
      "Monthly",
    );
    await waitFor(() => expect(calls).toHaveLength(1));
    expect(calls[0].body).toEqual({
      billing_model: "recurring",
      billing_interval_months: 1,
      interval_count: null,
    });
  });
});

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
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

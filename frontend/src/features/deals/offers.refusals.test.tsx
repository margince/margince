/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Locale } from "../../i18n";
import { LocaleProvider } from "../../i18n";
import { en as englishCopy } from "../../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
} from "../../testing/story-utils";
import type { Offer } from "./offers";
import { SendOfferAction } from "./offers.actions";
import { EditOfferHeaderModal } from "./offers.header";
import { OfferLineEditor } from "./offers.lines";

// An offer form names the field it refuses by its label and states the limit
// in the unit the reader typed, in the reader's language.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function render(ui: ReactNode, locale: Locale = "en") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial={locale}>{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

const line = {
  id: "li-1",
  position: 1,
  product_id: null,
  description: "Consulting hours",
  unit: "hour",
  quantity: 10,
  unit_price_minor: 10000,
  discount_pct: 0,
  tax_rate: 19,
  line_net_minor: 100000,
  line_tax_minor: 19000,
  line_total_minor: 119000,
  evidence: null,
  price_grounded: true,
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

const offer = {
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
  net_minor: 100000,
  tax_minor: 19000,
  gross_minor: 119000,
  template_id: null,
  line_items: [line],
  source: "manual",
  captured_by: "human:u1",
  version: 3,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
} as Offer;

function refusal(field: string, code: string, params: Record<string, string>) {
  return jsonResponse(
    {
      code: "validation_error",
      detail: `\`${field}\` refused`,
      details: { errors: [{ field, code, message: "refused", params }] },
    },
    422,
  );
}

describe("the add-line form", () => {
  it.each([
    ["en", "Discount %", "Discount % must be from 0% to 100%."],
    ["de", "Rabatt %", "Rabatt % muss zwischen 0 % und 100 % liegen."],
    ["vi", "Chiết khấu %", "Chiết khấu % phải từ 0% đến 100%."],
  ] as const)(
    "refuses a discount over 100 on its field before sending (%s)",
    async (locale, label, sentence) => {
      const user = userEvent.setup();
      const posted: unknown[] = [];
      installFetchStub({
        "POST /offers/o-1/line-items": (body) => {
          posted.push(body);
          return jsonResponse(offer);
        },
      });
      render(<OfferLineEditor offer={offer} />, locale);
      const discount = screen.getByLabelText(label);
      await user.type(discount, "150");
      expect(await screen.findByText(sentence)).toBeTruthy();
      expect(discount.getAttribute("aria-invalid")).toBe("true");
      await user.click(screen.getByTestId("add-line"));
      expect(posted).toEqual([]);
    },
  );

  it("refuses a negative tax rate in percent", async () => {
    const user = userEvent.setup();
    installFetchStub({});
    render(<OfferLineEditor offer={offer} />);
    await user.type(screen.getByTestId("new-line-tax"), "-5");
    expect(await screen.findByText("Tax % must be at least 0%.")).toBeTruthy();
  });

  it("puts the server's refusal on the field it names, in the reader's unit, until it is edited", async () => {
    const user = userEvent.setup();
    installFetchStub({
      "POST /offers/o-1/line-items": () =>
        refusal("unit_price_minor", "out_of_range", { max: "100000" }),
    });
    render(<OfferLineEditor offer={offer} />);
    await user.type(screen.getByTestId("new-line-description"), "Workshop");
    await user.type(screen.getByTestId("new-line-unit-price"), "2000");
    await user.click(screen.getByTestId("add-line"));

    const sentence = "Unit price must be at most €1,000.00.";
    expect(await screen.findByText(sentence)).toBeTruthy();
    const price = screen.getByTestId("new-line-unit-price");
    expect(price.getAttribute("aria-invalid")).toBe("true");
    await waitFor(() => expect(document.activeElement).toBe(price));
    expect(screen.queryByText(/unit_price_minor/)).toBeNull();

    await user.clear(price);
    await user.type(price, "5");
    await user.tab();
    await waitFor(() => expect(screen.queryByText(sentence)).toBeNull());
  });
});

describe("a line's cells", () => {
  it("says another line has the position, not the server's conflict", async () => {
    const user = userEvent.setup();
    installFetchStub({
      "PATCH /offers/o-1/line-items/li-1": () =>
        jsonResponse(
          {
            code: "conflict",
            detail: "position 2 is already taken on this offer: conflict",
          },
          409,
        ),
    });
    render(<OfferLineEditor offer={offer} />);
    const position = screen.getByTestId("line-position-li-1");
    await user.clear(position);
    await user.type(position, "2");
    await user.tab();
    expect(
      await screen.findByText(englishCopy["offer.positionTaken"]),
    ).toBeTruthy();
    expect(screen.queryByText(/: conflict/)).toBeNull();
  });

  it("refuses a discount over 100 before saving it", async () => {
    const user = userEvent.setup();
    const patched: unknown[] = [];
    installFetchStub({
      "PATCH /offers/o-1/line-items/li-1": (body) => {
        patched.push(body);
        return jsonResponse(offer);
      },
    });
    render(<OfferLineEditor offer={offer} />);
    const discount = screen.getByTestId("line-discount-li-1");
    await user.clear(discount);
    await user.type(discount, "150");
    await user.tab();
    expect(
      await screen.findByText("Discount % must be from 0% to 100%."),
    ).toBeTruthy();
    expect(patched).toEqual([]);
  });
});

describe("the header edit", () => {
  it("refuses a valid-until day the server cannot store on its field", async () => {
    const user = userEvent.setup();
    const patched: unknown[] = [];
    installFetchStub({
      "GET /me": meRoute({ offer: ["read", "update"] }),
      "PATCH /offers/o-1": (body) => {
        patched.push(body);
        return jsonResponse(offer);
      },
    });
    render(<EditOfferHeaderModal open onClose={() => {}} offer={offer} />);
    const dialog = await screen.findByRole("dialog");
    const day = within(dialog).getByRole("textbox", { name: "Valid until" });
    await user.clear(day);
    await user.type(day, "31/12/9999");
    await user.tab();
    const sentence = "Valid until must be from 02/01/0001 to 30/12/9999.";
    expect(await within(dialog).findByText(sentence)).toBeTruthy();
    await user.click(within(dialog).getByRole("button", { name: "Save" }));
    expect(patched).toEqual([]);
  });
});

describe("sending", () => {
  it.each([
    ["en", "Send", /^The Valid until date of this offer has passed\./],
    ["de", "Senden", /Gültig bis/],
  ] as const)(
    "says how to get past a lapsed valid-until day (%s)",
    async (locale, verb, sentence) => {
      const user = userEvent.setup();
      installFetchStub({
        "POST /offers/o-1/send": () =>
          refusal("valid_until", "offer_lapsed", {}),
      });
      render(<SendOfferAction offer={offer} />, locale);
      await user.click(screen.getByTestId("send-offer"));
      const dialog = await screen.findByRole("dialog");
      await user.click(within(dialog).getByRole("button", { name: verb }));
      expect(await within(dialog).findByText(sentence)).toBeTruthy();
      expect(within(dialog).queryByText(/regenerate it/)).toBeNull();
    },
  );
});

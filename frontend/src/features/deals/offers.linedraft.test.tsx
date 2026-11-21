/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { jsonResponse } from "../../testing/story-utils";
import { OfferScreen } from "./offers";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const line = {
  id: "li-1",
  position: 1,
  product_id: null,
  description: "Consulting",
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
};

type Held = { patch: Record<string, unknown>; release: () => void };

// Every line PATCH waits for the test to release it, and answers with the
// line as that PATCH wrote it.
function stubHeldSaves(held: Held[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = String(request ? request.url : input);
      const method = request ? request.method : (init?.method ?? "GET");
      if (url.includes("/line-items/") && method === "PATCH") {
        const patch = request
          ? await request.json()
          : JSON.parse(String(init?.body));
        await new Promise<void>((release) => held.push({ patch, release }));
        return jsonResponse({
          ...offer,
          line_items: [{ ...line, ...patch, version: 2 }],
        });
      }
      if (url.includes("/offers/")) {
        return jsonResponse(offer);
      }
      return jsonResponse({ data: [], page: { has_more: false } });
    }),
  );
}

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

describe("an offer line cell whose earlier save is still on its way", () => {
  it.each([
    ["line-description-li-1", " plus", " travel", "Consulting plus travel"],
    ["line-quantity-li-1", "0", "5", "1005"],
  ])(
    "%s keeps the text typed after the save started",
    async (testId, first, second, expected) => {
      const held: Held[] = [];
      stubHeldSaves(held);
      const user = userEvent.setup();
      renderOffer();
      await screen.findByText("ANG-2026-0007");
      const cell = screen.getByTestId(testId) as HTMLInputElement;

      await user.click(cell);
      await user.type(cell, first);
      await user.tab();
      await waitFor(() => expect(held).toHaveLength(1));

      await user.click(cell);
      await user.type(cell, second);
      await act(async () => {
        held[0].release();
      });
      expect(screen.getByTestId(testId)).toBe(cell);
      expect(cell.value).toBe(expected);
      expect(document.activeElement).toBe(cell);

      await user.tab();
      await waitFor(() => expect(held).toHaveLength(2));
      expect(Object.values(held[1].patch)[0]).toEqual(
        testId.includes("quantity") ? Number(expected) : expected,
      );
      await act(async () => {
        held[1].release();
      });
      await waitFor(() => expect(cell.value).toBe(expected));
    },
  );

  it("saves newer text typed and left before the earlier answer arrives", async () => {
    const held: Held[] = [];
    stubHeldSaves(held);
    const user = userEvent.setup();
    renderOffer();
    await screen.findByText("ANG-2026-0007");
    const cell = screen.getByTestId(
      "line-description-li-1",
    ) as HTMLInputElement;

    await user.click(cell);
    await user.type(cell, "!");
    await user.tab();
    await waitFor(() => expect(held).toHaveLength(1));
    await user.click(cell);
    await user.type(cell, "?");
    await user.tab();
    await waitFor(() => expect(held).toHaveLength(2));
    expect(held[1].patch).toEqual({ description: "Consulting!?" });

    await act(async () => {
      held[0].release();
      held[1].release();
    });
    await waitFor(() => expect(cell.value).toBe("Consulting!?"));
  });

  it("takes the saved value when nothing newer was typed", async () => {
    const held: Held[] = [];
    stubHeldSaves(held);
    const user = userEvent.setup();
    renderOffer();
    await screen.findByText("ANG-2026-0007");
    const cell = screen.getByTestId(
      "line-description-li-1",
    ) as HTMLInputElement;

    await user.click(cell);
    await user.type(cell, "!");
    await user.tab();
    await waitFor(() => expect(held).toHaveLength(1));
    await act(async () => {
      held[0].release();
    });
    await waitFor(() => expect(cell.value).toBe("Consulting!"));
    expect(held).toHaveLength(1);
  });
});

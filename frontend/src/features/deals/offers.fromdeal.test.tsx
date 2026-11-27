/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en as englishCopy } from "../../i18n/en";
import { renderWithQueryClient } from "../../testing/queryrender";
import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import type { DealCommercial } from "./deal360/deallinestotal";
import type { Offer } from "./offers";
import { OfferLinesPanel } from "./offers.fromdeal";

// An offer made from the deal's products changes only through the deal. It
// says when the deal moved on, and a draft takes the deal's lines again.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const AT = "2026-10-01T09:00:00Z";

const OFFER: Offer = {
  id: "o-1",
  deal_id: "d-1",
  offer_number: "A-1001",
  revision: 1,
  status: "draft",
  currency: "EUR",
  net_minor: 20_000,
  tax_minor: 0,
  gross_minor: 20_000,
  line_items: [],
  source: "manual",
  ai_generated: false,
  captured_by: "human:u-1",
  version: 1,
  source_commercial_revision: 1,
  created_at: AT,
  updated_at: AT,
};

const DEAL_AT = (revision: number): DealCommercial => ({
  deal_id: "d-1",
  mode: "lines",
  commercial_revision: revision,
  currency: "EUR",
  complete: true,
  missing: [],
  money_withheld: false,
  writable: true,
  lines: [],
});

function show(offer: Offer, revision: number, posted: unknown[] = []) {
  installFetchStub({
    "GET /deals/d-1/lines": () => jsonResponse(DEAL_AT(revision)),
    "POST /offers/o-1/refresh-from-deal": (body) => {
      posted.push(body);
      return jsonResponse({ ...offer, source_commercial_revision: revision });
    },
  });
  renderWithQueryClient(<OfferLinesPanel offer={offer} />);
}

describe("an offer made from the deal", () => {
  it("names its source and offers no line editing while the deal stands still", async () => {
    show(OFFER, 1);
    expect(
      await screen.findByText(englishCopy["offer.fromDeal.title"]),
    ).toBeTruthy();
    expect(
      screen.queryByRole("button", {
        name: englishCopy["offer.fromDeal.refresh"],
      }),
    ).toBeNull();
  });

  it("says the deal changed and refreshes the draft at the deal's revision", async () => {
    const posted: unknown[] = [];
    show(OFFER, 3, posted);
    expect(
      await screen.findByText(englishCopy["offer.fromDeal.stale"]),
    ).toBeTruthy();
    await userEvent.setup().click(
      screen.getByRole("button", {
        name: englishCopy["offer.fromDeal.refresh"],
      }),
    );
    await waitFor(() => expect(posted).toEqual([{ expected_revision: 3 }]));
  });

  it("tells a sent offer to regenerate rather than refresh", async () => {
    show({ ...OFFER, status: "sent" }, 3);
    expect(
      await screen.findByText(/Regenerate a revision to offer the new terms/),
    ).toBeTruthy();
    expect(
      screen.queryByRole("button", {
        name: englishCopy["offer.fromDeal.refresh"],
      }),
    ).toBeNull();
  });

  it("lists the lines it took from the deal", async () => {
    show(
      {
        ...OFFER,
        line_items: [
          {
            id: "li-1",
            position: 1,
            description: "Platform licence",
            unit: "piece",
            quantity: 2,
            unit_price_minor: 10_000,
            discount_pct: 0,
            tax_rate: 0,
            line_net_minor: 20_000,
            line_tax_minor: 0,
            line_total_minor: 20_000,
            price_grounded: true,
            created_at: AT,
            updated_at: AT,
          },
        ],
      },
      1,
    );
    expect(await screen.findByText("Platform licence")).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Line items" })).toBeTruthy();
  });
});

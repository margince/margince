/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { renderWithQueryClient } from "../../../testing/queryrender";
import { installFetchStub, jsonResponse } from "../../../testing/story-utils";
import type { DealCommercial } from "./deallines";
import { DealProductsCard } from "./dealproducts";

// A deal priced from its own lines. The card lists them with their net value
// and says what is missing. The editor saves against the revision it read.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const DEAL = "d-1";
const AT = "2026-10-01T09:00:00Z";

const BANANAS: DealCommercial = {
  deal_id: DEAL,
  mode: "lines",
  amount_basis: "lines_net_committed",
  commercial_revision: 1,
  currency: "EUR",
  complete: true,
  missing: [],
  money_withheld: false,
  writable: true,
  committed_net_minor: 20_000,
  known_committed_net_minor: 20_000,
  arr_minor: 0,
  lines: [
    {
      id: "l-1",
      position: 1,
      product_id: "p-banana",
      catalog_name: "Bananas",
      description: "Bananas",
      unit: "piece",
      quantity: 100,
      unit_price_minor: 200,
      currency: "EUR",
      discount_pct: 0,
      billing_model: "one_time",
      line_net_minor: 20_000,
      committed_net_minor: 20_000,
      missing: [],
      created_at: AT,
      version: 1,
      updated_at: AT,
    },
  ],
};

const ESTIMATE: DealCommercial = {
  ...BANANAS,
  mode: "estimate",
  amount_basis: "estimate",
  commercial_revision: 0,
  lines: [],
  committed_net_minor: null,
  known_committed_net_minor: null,
};

function show(state: DealCommercial, posted: unknown[] = []) {
  installFetchStub({
    [`GET /deals/${DEAL}/products`]: () => jsonResponse({ data: [] }),
    [`GET /deals/${DEAL}/lines`]: () => jsonResponse(state),
    [`POST /deals/${DEAL}/lines`]: (body) => {
      posted.push(body);
      return jsonResponse(BANANAS);
    },
  });
  renderWithQueryClient(
    <DealProductsCard
      dealId={DEAL}
      currency="EUR"
      typedAmount={state.mode === "lines" ? null : "€1,000.00"}
    />,
  );
}

describe("a deal priced from its lines", () => {
  it("lists the lines and the net value they add up to", async () => {
    show(BANANAS);
    expect(await screen.findByText("Bananas")).toBeTruthy();
    expect(screen.getByText("Net value: €200.00, without tax")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Edit products" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Set products" })).toBeNull();
  });

  it("says what is still missing beside the part already known", async () => {
    show({
      ...BANANAS,
      complete: false,
      committed_net_minor: null,
      missing: ["unit_price"],
    });
    expect(
      await screen.findByText(
        "Pricing incomplete. Known so far: €200.00. Still missing: a price.",
      ),
    ).toBeTruthy();
  });

  it("shows no figure to a reader the amount is masked from", async () => {
    show({ ...BANANAS, money_withheld: true, committed_net_minor: null });
    expect(
      await screen.findByText("Your role does not show this deal’s value."),
    ).toBeTruthy();
  });

  it("warns that the lines replace a typed value, and says so to the server", async () => {
    const posted: unknown[] = [];
    show(ESTIMATE, posted);
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "Price from products" }),
    );
    const dialog = await screen.findByRole("dialog");
    expect(
      within(dialog).getByText(/Saving replaces the typed value of €1,000.00/),
    ).toBeTruthy();
    await user.type(
      within(dialog).getByRole("textbox", { name: "Custom item" }),
      "Ripening",
    );
    await user.click(within(dialog).getByRole("button", { name: "Add item" }));
    await user.type(
      within(dialog).getByRole("spinbutton", { name: "Quantity" }),
      "2",
    );
    await user.click(
      within(dialog).getByRole("button", { name: "Save products" }),
    );
    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toMatchObject({
      expected_revision: 0,
      replace_estimate: true,
      description: "Ripening",
      quantity: 2,
      billing_model: "one_time",
    });
  });
});

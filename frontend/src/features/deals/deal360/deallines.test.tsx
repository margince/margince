/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pickOption } from "../../../design-system/select-testing";
import { renderWithQueryClient } from "../../../testing/queryrender";
import { installFetchStub, jsonResponse } from "../../../testing/story-utils";
import type { DealCommercial } from "./deallinestotal";
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
      canWrite
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
      within(dialog).getByRole("textbox", { name: "Quantity" }),
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

  it("keeps Enter in the custom item field from saving the lines", async () => {
    const posted: unknown[] = [];
    show(ESTIMATE, posted);
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "Price from products" }),
    );
    const dialog = await screen.findByRole("dialog");
    const custom = within(dialog).getByRole<HTMLInputElement>("textbox", {
      name: "Custom item",
    });
    await user.type(custom, "Ripening");
    await user.click(within(dialog).getByRole("button", { name: "Add item" }));
    await user.type(
      within(dialog).getByRole("textbox", { name: "Quantity" }),
      "2",
    );
    await user.type(custom, "Delivery{Enter}");
    expect(posted).toEqual([]);
    expect(screen.queryByRole("dialog")).toBe(dialog);
    expect(custom.value).toBe("Delivery");
  });

  it("sends a negotiated price and a new name for a product picked from the catalogue", async () => {
    const posted: unknown[] = [];
    show(ESTIMATE, posted);
    installFetchStub({
      [`GET /deals/${DEAL}/products`]: () => jsonResponse({ data: [] }),
      [`GET /deals/${DEAL}/lines`]: () => jsonResponse(ESTIMATE),
      "GET /products": () =>
        jsonResponse({
          data: [
            {
              id: "p-banana",
              name: "Bananas",
              unit: "piece",
              unit_price_minor: 200,
              currency: "EUR",
            },
          ],
          next_cursor: null,
          has_more: false,
        }),
      [`POST /deals/${DEAL}/lines`]: (body) => {
        posted.push(body);
        return jsonResponse(BANANAS);
      },
    });
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "Price from products" }),
    );
    const dialog = await screen.findByRole("dialog");
    await user.type(
      within(dialog).getByRole("searchbox", { name: "Add a product" }),
      "Ban",
    );
    await user.click(
      await within(dialog).findByRole("button", { name: /Bananas/ }),
    );
    const description = within(dialog).getByRole("textbox", {
      name: "Description",
    });
    await user.clear(description);
    await user.type(description, "Ripe bananas");
    const price = within(dialog).getByRole("textbox", {
      name: "Selling price",
    });
    await user.clear(price);
    await user.type(price, "1.80");
    await user.click(
      within(dialog).getByRole("button", { name: "Save products" }),
    );
    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toMatchObject({
      product_id: "p-banana",
      description: "Ripe bananas",
      unit_price_minor: 180,
    });
  });

  it("starts a picked product at its rate card price, and sends the price the seller saw", async () => {
    const posted: unknown[] = [];
    show(ESTIMATE, posted);
    installFetchStub({
      [`GET /deals/${DEAL}/products`]: () => jsonResponse({ data: [] }),
      [`GET /deals/${DEAL}/lines`]: () => jsonResponse(ESTIMATE),
      "GET /products": () =>
        jsonResponse({
          data: [
            {
              id: "p-banana",
              name: "Bananas",
              unit: "piece",
              unit_price_minor: 200,
              currency: "EUR",
            },
          ],
          next_cursor: null,
          has_more: false,
        }),
      [`POST /deals/${DEAL}/lines`]: (body) => {
        posted.push(body);
        return jsonResponse(BANANAS);
      },
    });
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "Price from products" }),
    );
    const dialog = await screen.findByRole("dialog");
    await user.type(
      within(dialog).getByRole("searchbox", { name: "Add a product" }),
      "Ban",
    );
    await user.click(
      await within(dialog).findByRole("button", { name: /Bananas/ }),
    );
    expect(
      within(dialog).getByRole("textbox", { name: "Selling price" }),
    ).toHaveProperty("value", "2.00");
    await user.click(
      within(dialog).getByRole("button", { name: "Save products" }),
    );
    await waitFor(() => expect(posted).toHaveLength(1));
    // Sent as shown, so a rate card change before the save is not taken.
    expect(posted[0]).toMatchObject({ unit_price_minor: 200 });
  });

  it("drops a rate card price once the lines take another currency", async () => {
    const posted: unknown[] = [];
    installFetchStub({
      [`GET /deals/${DEAL}/products`]: () => jsonResponse({ data: [] }),
      [`GET /deals/${DEAL}/lines`]: () =>
        jsonResponse({ ...ESTIMATE, currency: null }),
      "GET /products": () =>
        jsonResponse({
          data: [
            {
              id: "p-banana",
              name: "Bananas",
              unit: "piece",
              unit_price_minor: 200,
              currency: "EUR",
            },
          ],
          next_cursor: null,
          has_more: false,
        }),
      [`POST /deals/${DEAL}/lines`]: (body) => {
        posted.push(body);
        return jsonResponse(BANANAS);
      },
    });
    renderWithQueryClient(
      <DealProductsCard
        canWrite
        dealId={DEAL}
        currency=""
        typedAmount={null}
      />,
    );
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "Price from products" }),
    );
    const dialog = await screen.findByRole("dialog");
    const currency = within(dialog).getByRole("combobox", { name: "Currency" });
    await pickOption(user, currency, "EUR");
    await user.type(
      within(dialog).getByRole("searchbox", { name: "Add a product" }),
      "Ban",
    );
    await user.click(
      await within(dialog).findByRole("button", { name: /Bananas/ }),
    );
    const price = () =>
      within(dialog).getByRole("textbox", { name: "Selling price" });
    expect(price()).toHaveProperty("value", "2.00");

    await pickOption(user, currency, "USD");
    expect(price()).toHaveProperty("value", "");
    // Saved with no price, the line would take a euro rate card price.
    const save = within(dialog).getByRole("button", { name: "Save products" });
    expect(save).toHaveProperty("disabled", true);
    expect(
      within(dialog).getByText(
        "This product is priced in EUR. Enter its price.",
      ),
    ).toBeTruthy();
    await user.type(price(), "2.40");
    expect(save).toHaveProperty("disabled", false);
    await user.click(save);
    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toMatchObject({ currency: "USD", unit_price_minor: 240 });
  });

  it("sends the tax rate and discount typed on a custom item", async () => {
    const posted: unknown[] = [];
    show(ESTIMATE, posted);
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "Price from products" }),
    );
    const dialog = await screen.findByRole("dialog");
    await user.type(
      within(dialog).getByRole("textbox", { name: "Custom item" }),
      "Installation",
    );
    await user.click(within(dialog).getByRole("button", { name: "Add item" }));
    await user.type(
      within(dialog).getByRole("textbox", { name: "Tax %" }),
      "19",
    );
    await user.type(
      within(dialog).getByRole("textbox", { name: "Discount %" }),
      "10",
    );
    await user.click(
      within(dialog).getByRole("button", { name: "Save products" }),
    );
    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toMatchObject({
      description: "Installation",
      tax_rate: 19,
      discount_pct: 10,
    });
  });

  it("changes a saved line's tax rate and leaves its untouched discount alone", async () => {
    const patched: unknown[] = [];
    installFetchStub({
      [`GET /deals/${DEAL}/products`]: () => jsonResponse({ data: [] }),
      [`GET /deals/${DEAL}/lines`]: () =>
        jsonResponse({
          ...BANANAS,
          lines: [
            { ...BANANAS.lines[0], tax_rate: 7, tax_category: "reduced" },
          ],
        }),
      [`PATCH /deals/${DEAL}/lines/l-1`]: (body) => {
        patched.push(body);
        return jsonResponse(BANANAS);
      },
    });
    renderWithQueryClient(
      <DealProductsCard
        dealId={DEAL}
        currency="EUR"
        typedAmount={null}
        canWrite
      />,
    );
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "Edit products" }),
    );
    const dialog = await screen.findByRole("dialog");
    const tax = within(dialog).getByRole("textbox", { name: "Tax %" });
    expect(tax).toHaveProperty("value", "7");
    await user.clear(tax);
    await user.type(tax, "19");
    await user.click(
      within(dialog).getByRole("button", { name: "Save products" }),
    );
    await waitFor(() => expect(patched).toHaveLength(1));
    expect(patched[0]).toEqual({ tax_rate: 19, expected_revision: 1 });
  });

  it("tells products of one name apart by their SKU and rate card price", async () => {
    show(ESTIMATE);
    installFetchStub({
      [`GET /deals/${DEAL}/products`]: () => jsonResponse({ data: [] }),
      [`GET /deals/${DEAL}/lines`]: () => jsonResponse(ESTIMATE),
      "GET /products": () =>
        jsonResponse({
          data: [
            {
              id: "p-eur",
              name: "Widget",
              sku: "W-EU",
              unit: "piece",
              unit_price_minor: 3333,
              currency: "EUR",
            },
            {
              id: "p-jpy",
              name: "Widget",
              sku: null,
              unit: "hour",
              unit_price_minor: 4000,
              currency: "JPY",
            },
          ],
          next_cursor: null,
          has_more: false,
        }),
    });
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "Price from products" }),
    );
    const dialog = await screen.findByRole("dialog");
    await user.type(
      within(dialog).getByRole("searchbox", { name: "Add a product" }),
      "Wid",
    );
    expect(
      await within(dialog).findByText("W-EU · €33.33 / Piece"),
    ).toBeTruthy();
    expect(within(dialog).getByText("JP¥4,000 / Hour")).toBeTruthy();
  });

  it("says which changes saved when one in the middle fails, and keeps the dialog open", async () => {
    const patched: unknown[] = [];
    installFetchStub({
      [`GET /deals/${DEAL}/products`]: () => jsonResponse({ data: [] }),
      [`GET /deals/${DEAL}/lines`]: () => jsonResponse(BANANAS),
      [`PATCH /deals/${DEAL}/lines/l-1`]: (body) => {
        patched.push(body);
        return jsonResponse({ ...BANANAS, commercial_revision: 2 });
      },
      [`POST /deals/${DEAL}/lines`]: () =>
        jsonResponse(
          {
            type: "about:blank",
            title: "Validation failed",
            status: 422,
            detail: "description is required",
          },
          422,
        ),
    });
    renderWithQueryClient(
      <DealProductsCard
        dealId={DEAL}
        currency="EUR"
        typedAmount={null}
        canWrite
      />,
    );
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "Edit products" }),
    );
    const dialog = await screen.findByRole("dialog");
    const quantity = within(dialog).getByRole("textbox", {
      name: "Quantity",
    });
    await user.clear(quantity);
    await user.type(quantity, "150");
    await user.type(
      within(dialog).getByRole("textbox", { name: "Custom item" }),
      "Ripening",
    );
    await user.click(within(dialog).getByRole("button", { name: "Add item" }));
    await user.click(
      within(dialog).getByRole("button", { name: "Save products" }),
    );
    expect(
      await within(dialog).findByText(/1 of 2 changes were saved/),
    ).toBeTruthy();
    expect(patched).toHaveLength(1);
    expect(screen.getByRole("dialog")).toBeTruthy();
  });
});

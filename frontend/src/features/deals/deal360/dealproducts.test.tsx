/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../../api/schema";
import { renderWithQueryClient } from "../../../testing/queryrender";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  permissionDenied,
} from "../../../testing/story-utils";
import { DealProductsCard } from "./dealproducts";

// The products a deal sells. The accepted offer fills them and a writer may set
// them by hand; product reports read them. A deal with no accepted offer had no
// screen to set them on, so it was missing from every product report.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

type DealProduct = components["schemas"]["DealProduct"];

const DEAL = "d-1";
const ROUTE = `/deals/${DEAL}/products`;

const FROM_OFFER: DealProduct[] = [
  {
    product_id: "p-erp",
    product_name: "ERP licence",
    source: "offer",
    amount_minor: 1_200_000,
    currency: "EUR",
    component_kind: "software",
    updated_at: "2026-10-01T09:00:00Z",
  },
];

function show(refusedReasonId?: string) {
  renderWithQueryClient(
    <DealProductsCard
      dealId={DEAL}
      currency="EUR"
      refusedReasonId={refusedReasonId}
    />,
  );
}

describe("the deal's products", () => {
  it("lists each product with its amount and where it came from", async () => {
    installFetchStub({
      "GET /me": meRoute({ deal: ["read", "update"] }),
      [`GET ${ROUTE}`]: () => jsonResponse({ data: FROM_OFFER }),
    });
    show();
    expect(await screen.findByText("ERP licence")).toBeTruthy();
    expect(screen.getByText("€12,000.00")).toBeTruthy();
    expect(screen.getByText("From the offer")).toBeTruthy();
  });

  it("says a deal with no products has none yet", async () => {
    installFetchStub({ [`GET ${ROUTE}`]: () => jsonResponse({ data: [] }) });
    show();
    expect(await screen.findByText(/No products yet/)).toBeTruthy();
  });

  it("tells a reader who may not see the products so, rather than showing none", async () => {
    installFetchStub({ [`GET ${ROUTE}`]: () => permissionDenied() });
    show();
    expect(
      await screen.findByText("You may not see the products on this deal."),
    ).toBeTruthy();
    expect(screen.queryByText(/No products yet/)).toBeNull();
  });

  it("replaces the offer's products with the list set by hand", async () => {
    const sent: unknown[] = [];
    const saved: DealProduct[] = [
      {
        ...FROM_OFFER[0],
        product_id: "p-impl",
        product_name: "Implementation",
        source: "manual",
        amount_minor: 500_000,
      },
    ];
    installFetchStub({
      [`GET ${ROUTE}`]: () => jsonResponse({ data: FROM_OFFER }),
      "GET /products": () =>
        jsonResponse({
          data: [{ id: "p-impl", name: "Implementation" }],
          next_cursor: null,
          has_more: false,
        }),
      [`PUT ${ROUTE}`]: (body) => {
        sent.push(body);
        return jsonResponse({ data: saved });
      },
    });
    show();
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "Set products" }),
    );
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText(/Saving replaces them/)).toBeTruthy();

    await user.click(
      within(dialog).getByRole("button", { name: "Remove ERP licence" }),
    );
    await user.type(
      within(dialog).getByRole("searchbox", { name: "Add a product" }),
      "Impl",
    );
    await user.click(
      await within(dialog).findByRole("button", { name: /Implementation/ }),
    );
    await user.type(
      within(dialog).getByRole("spinbutton", {
        name: "Amount for Implementation",
      }),
      "5000",
    );
    await user.click(
      within(dialog).getByRole("button", { name: "Save products" }),
    );

    await waitFor(() =>
      expect(sent).toEqual([
        { products: [{ product_id: "p-impl", amount_minor: 500_000 }] },
      ]),
    );
    expect(await screen.findByText("Set by hand")).toBeTruthy();
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("refuses the edit on a deal that takes no changes, and points at why", async () => {
    installFetchStub({
      [`GET ${ROUTE}`]: () => jsonResponse({ data: FROM_OFFER }),
    });
    show("deal-refused");
    const edit = await screen.findByRole("button", { name: "Set products" });
    expect(edit.getAttribute("aria-describedby")).toContain("deal-refused");
    expect(edit.hasAttribute("disabled")).toBe(true);
    await userEvent.setup().click(edit);
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});

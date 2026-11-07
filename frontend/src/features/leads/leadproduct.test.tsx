/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LeadScreen } from "./leads";
import { jsonResponse, lead, render, stubFetch } from "./leads.testkit";

// The product a lead is interested in. Product reports count the lead under
// it, so a writer names, changes or clears it on the lead's details.

beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.localStorage.clear();
  window.location.hash = "";
});

const PRODUCTS = [
  { id: "p-suite", name: "Suite" },
  { id: "p-onboarding", name: "Onboarding" },
];

function serve(served: Record<string, unknown>) {
  const patches: Record<string, unknown>[] = [];
  stubFetch(async (url, method, request) => {
    const path = new URL(url).pathname;
    const product = PRODUCTS.find((row) =>
      path.endsWith(`/products/${row.id}`),
    );
    if (product) {
      return jsonResponse(product);
    }
    if (path.endsWith("/products")) {
      return jsonResponse({
        data: PRODUCTS,
        page: { next_cursor: null, has_more: false },
      });
    }
    if (method === "PATCH") {
      const body: Record<string, unknown> = JSON.parse(await request.text());
      patches.push(body);
      return jsonResponse({ ...served, ...body, version: 2 });
    }
    return jsonResponse(served);
  });
  return patches;
}

describe("a lead's product", () => {
  it("names the product the lead asks about", async () => {
    serve({ ...lead, product_id: "p-suite" });
    render(<LeadScreen id="l-1" />);
    expect(await screen.findByText("Suite")).toBeTruthy();
  });

  it("changes the product to one picked from the catalog", async () => {
    const patches = serve({ ...lead, product_id: "p-suite" });
    render(<LeadScreen id="l-1" />);
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "Change product" }),
    );
    const dialog = within(screen.getByRole("dialog"));
    await user.type(dialog.getByRole("combobox"), "Onb");
    await user.click(await dialog.findByRole("option", { name: "Onboarding" }));
    await waitFor(() =>
      expect(patches).toEqual([{ product_id: "p-onboarding" }]),
    );
  });

  it("clears the product", async () => {
    const patches = serve({ ...lead, product_id: "p-suite" });
    render(<LeadScreen id="l-1" />);
    await userEvent.click(
      await screen.findByRole("button", { name: "Clear the product" }),
    );
    await waitFor(() => expect(patches).toEqual([{ product_id: null }]));
  });
});

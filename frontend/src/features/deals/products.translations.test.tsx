// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { ProductTranslationsDialog } from "./products.translations";

// A product's text in further languages: listed, added and removed.

const PRODUCT: components["schemas"]["Product"] = {
  id: "p-1",
  name: "Retainer",
  unit: "unit",
  unit_price_minor: 50000,
  currency: "EUR",
  default_tax_rate: 19,
  active: true,
  source: "manual",
  captured_by: "human:u1",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function stubApi() {
  const writes: { method: string; url: string; body: unknown }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request =
        input instanceof Request ? input : new Request(String(input), init);
      if (request.method === "GET") {
        return Response.json({
          data: [
            {
              language: "de",
              name: "Pauschale",
              description: "Monatliche Betreuung",
              version: 1,
              updated_at: "2026-11-01T08:00:00Z",
            },
          ],
        });
      }
      const raw = await request.text();
      writes.push({
        method: request.method,
        url: request.url,
        body: raw ? JSON.parse(raw) : undefined,
      });
      return request.method === "DELETE"
        ? new Response(null, { status: 204 })
        : Response.json({
            language: "fr",
            name: "Forfait",
            version: 1,
            updated_at: "2026-11-01T08:00:00Z",
          });
    }),
  );
  return writes;
}

function renderDialog() {
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <ProductTranslationsDialog product={PRODUCT} onClose={vi.fn()} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

it("lists the product's languages and saves a new one under its code", async () => {
  const writes = stubApi();
  renderDialog();
  const user = userEvent.setup();
  const dialog = await screen.findByRole("dialog");
  expect(await within(dialog).findByText("de · Pauschale")).toBeInTheDocument();
  await user.type(
    within(dialog).getByRole("textbox", {
      name: en["product.translationLanguage"],
    }),
    "FR",
  );
  await user.type(
    within(dialog).getByRole("textbox", { name: en["product.name"] }),
    "Forfait",
  );
  await user.click(
    within(dialog).getByRole("button", {
      name: en["product.translationSave"],
    }),
  );
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]?.method).toBe("PUT");
  expect(writes[0]?.url).toMatch(/\/products\/p-1\/translations\/fr$/);
  expect(writes[0]?.body).toEqual({ name: "Forfait" });
});

it("removes a language", async () => {
  const writes = stubApi();
  renderDialog();
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "Remove de" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]?.method).toBe("DELETE");
  expect(writes[0]?.url).toMatch(/\/products\/p-1\/translations\/de$/);
});

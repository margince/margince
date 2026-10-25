// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import type { Offer } from "./offers";
import { EditOfferHeaderModal } from "./offers.header";

const OFFER: Offer = {
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
  net_minor: 0,
  tax_minor: 0,
  gross_minor: 0,
  template_id: null,
  line_items: [],
  source: "manual",
  ai_generated: false,
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
  const writes: Request[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request =
        input instanceof Request ? input : new Request(String(input), init);
      if (request.method !== "GET") {
        writes.push(request);
      }
      return Response.json({ data: [], page: { next_cursor: null } });
    }),
  );
  return writes;
}

it("refuses a save while the valid-until day cannot be read, and keeps the stored day", async () => {
  const writes = stubApi();
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <EditOfferHeaderModal open onClose={vi.fn()} offer={OFFER} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  const user = userEvent.setup();
  const box = await screen.findByRole("textbox", { name: "Valid until" });
  await user.clear(box);
  await user.type(box, "31/02/2027");

  await user.click(screen.getByRole("button", { name: "Save" }));

  expect(writes).toHaveLength(0);
  expect(screen.getByRole("alert")).toHaveTextContent(/^Enter a date as /);
  expect(box).toHaveAttribute("aria-invalid", "true");
  expect(box).toHaveValue("31/02/2027");
});

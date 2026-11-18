// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { offer } from "./deals.testkit";
import type { Offer } from "./offers";
import { OfferHeaderFields } from "./offers.summary";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function renderHeader(shown: Offer) {
  const paths: string[] = [];
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request =
        input instanceof Request ? input : new Request(String(input), init);
      const path = new URL(request.url).pathname;
      paths.push(path);
      if (path.endsWith("/me")) {
        return Response.json({ user: { id: "u1" }, authorization: {} });
      }
      if (path.endsWith("/offer-templates")) {
        return Response.json({
          data: [{ id: "tpl-1", name: "Standard terms" }],
          page: { next_cursor: null },
        });
      }
      return Response.json({ data: [], page: { next_cursor: null } });
    }),
  );
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <OfferHeaderFields offer={shown} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { paths, client };
}

it("names the offer's template", async () => {
  renderHeader(offer({ template_id: "tpl-1" }));
  expect(await screen.findByText("Standard terms")).toBeInTheDocument();
});

// The template list only names the offer's template, so an offer with none
// has nothing to read it for.
it("reads no template list for an offer without a template", async () => {
  const { paths, client } = renderHeader(offer({ template_id: null }));
  // Once the reader is known, the scope no longer holds the read back.
  await vi.waitFor(() =>
    expect(
      client
        .getQueryCache()
        .findAll({ queryKey: ["offer-templates-all"] })
        .some((query) => query.queryKey[1] !== ""),
    ).toBe(true),
  );
  expect(paths.filter((p) => p.endsWith("/offer-templates"))).toEqual([]);
});

it("shows the offer's revision, valid-until day, intro and terms", async () => {
  renderHeader(
    offer({
      status: "sent",
      valid_until: "2026-08-01",
      intro_text: "Dear buyer,\nhere is our offer.",
      terms_text: "Payable in 30 days.",
    }),
  );
  expect(await screen.findByText("Revision")).toBeInTheDocument();
  expect(screen.getByText("01/08/2026")).toBeInTheDocument();
  expect(screen.getByText(/here is our offer/)).toBeInTheDocument();
  expect(screen.getByText("Payable in 30 days.")).toBeInTheDocument();
});

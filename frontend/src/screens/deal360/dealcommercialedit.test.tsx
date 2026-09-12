/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { DealCommercialEdit } from "./dealcommercialedit";

// The two rules this form has to hold that the server also holds, because a
// reader who meets either as a 422 has already done the work twice.
//
//   - Expected ARR needs a CURRENCY. The server refuses a figure with nothing
//     to price it in, so the field is not offered without one.
//   - A reader who was not SHOWN the money must not be able to clear it. The
//     read mask withholds amount, ARR and currency as one unit.

type Deal = components["schemas"]["Deal"];

const deal = (over: Partial<Deal>): Deal =>
  ({
    id: "d1",
    name: "Seasonal payroll",
    status: "open",
    version: 4,
    source: "manual",
    captured_by: "u1",
    created_at: "2026-09-01T10:00:00Z",
    updated_at: "2026-09-01T10:00:00Z",
    ...over,
  }) as Deal;

type Sent = { body: Record<string, unknown>; ifMatch: string | null };

function stubFetch(sent: Sent[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      if (req.method === "PATCH") {
        sent.push({
          body: await req.clone().json(),
          ifMatch: req.headers.get("If-Match"),
        });
      }
      return new Response(JSON.stringify({ id: "d1", version: 5 }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
}

function show(d: Deal) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <LocaleProvider>
        <DealCommercialEdit open onClose={() => {}} deal={d} sources={[]} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("offers Expected ARR only once the deal has a currency", () => {
  stubFetch([]);
  show(deal({ currency: "EUR", amount_minor: 500_000 }));
  expect(screen.getByText("Expected ARR")).toBeTruthy();
});

it("says why, rather than refusing, when there is no currency to price in", () => {
  stubFetch([]);
  show(deal({}));
  // The server answers AmountCurrencyPairError here. Stating the reason is
  // what stops a reader meeting it as a 422 after filling the form.
  expect(screen.queryByText("Expected ARR")).toBeNull();
  expect(screen.getByText(/needs a currency/i)).toBeTruthy();
});

it("never sends the money of a reader who was not shown it", async () => {
  const sent: Sent[] = [];
  stubFetch(sent);
  // The mask withholds amount, ARR and currency together, so this reader sees
  // nulls where a figure stands. Sending a zero they never entered would
  // clear a price they are not permitted to read.
  show(
    deal({
      masked_fields: ["expected_arr_minor"],
      commercial_motion: "renewal",
    }),
  );
  expect(screen.queryByText("Expected ARR")).toBeNull();
  await userEvent.click(screen.getByRole("button", { name: "Save context" }));
  expect(sent).toHaveLength(1);
  expect("expected_arr_minor" in sent[0].body).toBe(false);
  expect(sent[0].ifMatch).toBe("4");
});

it("sends an unpicked field as null, which is how the wire says nobody answered", async () => {
  const sent: Sent[] = [];
  stubFetch(sent);
  show(deal({ commercial_motion: "renewal", currency: "EUR" }));
  await userEvent.click(screen.getByRole("button", { name: "Save context" }));
  expect(sent[0].body.priority).toBe(null);
  expect(sent[0].body.acquisition_source).toBe(null);
  // Re-sent as it stands: the reader changed nothing about it.
  expect(sent[0].body.commercial_motion).toBe("renewal");
});

it("keeps a zero ARR apart from no ARR at all", async () => {
  const sent: Sent[] = [];
  stubFetch(sent);
  // The server keeps both and refuses only a negative. A deal explicitly worth
  // nothing per year is a different record from one nobody has priced, and
  // collapsing them would null out a stored zero on an unrelated save.
  show(deal({ currency: "EUR", expected_arr_minor: 0 }));
  await userEvent.click(screen.getByRole("button", { name: "Save context" }));
  expect(sent[0].body.expected_arr_minor).toBe(0);
});

it("can take an ARR back off a deal", async () => {
  const sent: Sent[] = [];
  stubFetch(sent);
  // MoneyInput deliberately ignores an emptied box and restores the last
  // figure on blur, so without its own control there is no way to clear this.
  show(deal({ currency: "EUR", expected_arr_minor: 1_200_000 }));
  await userEvent.click(screen.getByRole("button", { name: "Remove" }));
  await userEvent.click(screen.getByRole("button", { name: "Save context" }));
  expect(sent[0].body.expected_arr_minor).toBe(null);
});

it("offers no ARR editor while the figure comes from an accepted offer", async () => {
  const sent: Sent[] = [];
  stubFetch(sent);
  // The server refuses a manual edit while that provenance stands, and refuses
  // the WHOLE patch — so an editor here would lose the reader's motion and
  // priority changes along with the ARR they were not allowed to make.
  show(
    deal({
      currency: "EUR",
      expected_arr_minor: 1_200_000,
      arr_source_offer_id: "of-1",
    }),
  );
  expect(screen.queryByText("Expected ARR")).toBeNull();
  expect(screen.getByText(/from the accepted offer/i)).toBeTruthy();
  await userEvent.click(screen.getByRole("button", { name: "Save context" }));
  expect("expected_arr_minor" in sent[0].body).toBe(false);
});

it("pins the save to the reading the form opened on", async () => {
  const sent: Sent[] = [];
  stubFetch(sent);
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const view = (d: Deal) => (
    <QueryClientProvider client={qc}>
      <LocaleProvider>
        <DealCommercialEdit open onClose={() => {}} deal={d} sources={[]} />
      </LocaleProvider>
    </QueryClientProvider>
  );
  const { rerender } = render(view(deal({ version: 4, priority: "high" })));
  // A background refetch bumps the version while the form sits open holding
  // pre-refetch values. Pinning to the LIVE version would let the conflict
  // check pass and silently overwrite whatever moved.
  rerender(view(deal({ version: 9, priority: "low" })));
  await userEvent.click(screen.getByRole("button", { name: "Save context" }));
  expect(sent[0].ifMatch).toBe("4");
});

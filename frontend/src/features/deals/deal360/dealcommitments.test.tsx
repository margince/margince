/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../../api/schema";
import { type Locale, LocaleProvider } from "../../../i18n";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  permissionDenied,
} from "../../../testing/story-utils";
import { DealCommitmentsCard } from "./dealcommitments";

// The deal's customer commitments as the server judged them: the speaker
// named once, a past-due promise asking whether it happened, and a stated
// total the server found to differ from the deal. The card decides none of
// these itself, so a row the server did not mark is drawn unmarked.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

type DealCommitments = components["schemas"]["DealCommitments"];

const DEAL = "d-1";
const ROUTE = `GET /deals/${DEAL}/commitments`;

const ROW = {
  contact_id: "p-1",
  contact_name: "Minh Tran",
  source_activity_id: "a-1",
  source_kind: "email",
  occurred_at: "2026-07-01T09:00:00Z",
  occurred_precision: "instant",
} as const;

const COMMITMENTS: DealCommitments = {
  complete: true,
  has_more: false,
  data: [
    {
      ...ROW,
      id: "c-late",
      body: "Minh Tran agreed to sign the NDA.",
      source_quote: "Yes, I will sign the NDA by Friday.",
      due_at: "2026-07-24T16:59:59Z",
      overdue: true,
    },
    {
      ...ROW,
      id: "c-price",
      body: "Committed to proceeding with the Gold package.",
      source_quote: "We will proceed with the Gold package at €42,000.",
      due_at: "2026-11-20T16:59:59Z",
      overdue: false,
      differs_from_deal: { amount_minor: 4_200_000, currency: "EUR" },
    },
    {
      ...ROW,
      id: "c-deposit",
      body: "Will pay the deposit.",
      source_quote: "We will pay a deposit of €10,000 next week.",
      due_at: null,
      overdue: false,
    },
  ],
};

function show(locale: Locale = "en") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial={locale}>
        <DealCommitmentsCard dealId={DEAL} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("the deal's customer commitments", () => {
  it("names the speaker once", async () => {
    installFetchStub({
      "GET /me": meRoute({ contact: ["read", "update"] }),
      [ROUTE]: () => jsonResponse(COMMITMENTS),
    });
    show();
    expect(await screen.findByText("Agreed to sign the NDA.")).toBeTruthy();
    expect(screen.queryByText(/Minh Tran agreed/)).toBeNull();
  });

  it("asks whether an overdue commitment happened and settles the answer", async () => {
    const settled: unknown[] = [];
    installFetchStub({
      "GET /me": meRoute({ contact: ["read", "update"] }),
      [ROUTE]: () => jsonResponse(COMMITMENTS),
      "POST /claims/c-late/settle": (body) => {
        settled.push(body);
        return new Response(null, { status: 204 });
      },
    });
    show();
    expect(await screen.findByText("Overdue: did it happen?")).toBeTruthy();
    expect(screen.getAllByText("Overdue: did it happen?")).toHaveLength(1);
    await userEvent
      .setup()
      .click(await screen.findByRole("button", { name: "Done" }));
    await waitFor(() => expect(settled).toEqual([{ outcome: "done" }]));
  });

  it("offers no answer to a reader who may not update the contact", async () => {
    installFetchStub({
      "GET /me": meRoute({ contact: ["read"] }),
      [ROUTE]: () => jsonResponse(COMMITMENTS),
    });
    show();
    expect(await screen.findByText("Overdue: did it happen?")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Done" })).toBeNull();
    expect(screen.queryByRole("button", { name: "No longer true" })).toBeNull();
  });

  it("marks only the total the server found to differ, with its amount", async () => {
    installFetchStub({
      "GET /me": meRoute({ contact: ["read"] }),
      [ROUTE]: () => jsonResponse(COMMITMENTS),
    });
    show();
    expect(
      await screen.findByText("Differs from the deal value (€42,000)"),
    ).toBeTruthy();
    expect(screen.getAllByText(/Differs from the deal value/)).toHaveLength(1);
  });

  it("speaks Vietnamese to a Vietnamese reader", async () => {
    installFetchStub({
      "GET /me": meRoute({ contact: ["read"] }),
      [ROUTE]: () => jsonResponse(COMMITMENTS),
    });
    show("vi");
    expect(
      await screen.findByText("Đã quá hạn: đã thực hiện chưa?"),
    ).toBeTruthy();
    expect(screen.getByText(/^Khác với giá trị deal \(/)).toBeTruthy();
  });
});

describe("a card with nothing to list", () => {
  it("says the customer owes nothing when the list is complete", async () => {
    installFetchStub({
      "GET /me": meRoute({ contact: ["read"] }),
      [ROUTE]: () =>
        jsonResponse({ complete: true, has_more: false, data: [] }),
    });
    show();
    expect(
      await screen.findByText(
        "The customer has no open commitments on this deal.",
      ),
    ).toBeTruthy();
    expect(screen.queryByText(/are left out/)).toBeNull();
  });

  it("says commitments were left out when the reader sees only part", async () => {
    installFetchStub({
      "GET /me": meRoute({ contact: ["read"] }),
      [ROUTE]: () =>
        jsonResponse({ complete: false, has_more: false, data: [] }),
    });
    show();
    expect(
      await screen.findByText(
        "Some commitments are left out because you cannot read their contact or message.",
      ),
    ).toBeTruthy();
  });

  it("tells a reader without access so instead of an empty card", async () => {
    installFetchStub({
      "GET /me": meRoute({ contact: ["read"] }),
      [ROUTE]: permissionDenied,
    });
    show();
    expect(
      await screen.findByText(/You cannot read the customer’s commitments/),
    ).toBeTruthy();
    expect(
      screen.queryByText("The customer has no open commitments on this deal."),
    ).toBeNull();
  });

  it("says the read failed when the server could not answer", async () => {
    installFetchStub({
      "GET /me": meRoute({ contact: ["read"] }),
      [ROUTE]: () =>
        jsonResponse({ title: "Internal error", status: 500 }, 500),
    });
    show();
    expect(await screen.findByText(/.+/, { selector: ".t-danger" })).toBeTruthy();
    expect(
      screen.queryByText("The customer has no open commitments on this deal."),
    ).toBeNull();
  });
});

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { readFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { DealIdentityFacts, DealSubtitle } from "./dealheaderfacts";

// The facts a rep checks first, on the record's head: value, stage, owner —
// the same three questions DealIdentityLine's dot-joined sentence answered,
// now as the named cells every other record's head carries.
//
// The owner was once on no part of the page — not the header, not the rail,
// not the readings — so "whose deal is this" could only be answered by
// opening Edit.

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

// The owner and the partner/company references each resolve through their
// own read (the roster, EntityRef's name lookup). None of these tests cares
// about a resolved NAME — each checks the fallback, the mask, or a word
// beside the reference — so every route answers the same empty, honest page,
// on every test rather than the ones that ask for a reference by hand.
beforeEach(() => {
  vi.stubGlobal("fetch", () => Promise.resolve(jsonResponse({ data: [] })));
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function show(node: React.ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{node}</LocaleProvider>
    </QueryClientProvider>,
  );
}

const stages = [
  { id: "st-1", name: "Qualified" },
  { id: "st-2", name: "Proposal" },
];

describe("the facts strip says what a deal is worth, where it is, and whose it is", () => {
  it("names the stage rather than showing its id", () => {
    show(
      <DealIdentityFacts
        deal={{ amount_minor: 6_400_000, currency: "EUR", stage_id: "st-1" }}
        stages={stages}
        locale="en"
      />,
    );
    expect(screen.getByText("Qualified")).toBeInTheDocument();
    expect(screen.queryByText("st-1")).not.toBeInTheDocument();
  });

  it("says a deal is unassigned rather than leaving the owner blank", () => {
    show(
      <DealIdentityFacts
        deal={{ amount_minor: 1000, currency: "EUR", stage_id: "st-1" }}
        stages={stages}
        locale="en"
      />,
    );
    // An empty value here reads as a rendering fault. "Unassigned" is a fact
    // about the deal, and it is the one a rep acts on.
    expect(screen.getByText(/Unassigned/)).toBeInTheDocument();
  });

  it("names the field it may not show rather than printing a dash", () => {
    // A rep who may read the deal but not its amount. A bare "—" would read
    // as "this deal has no value", which is a different and wrong statement —
    // and a lone lock among named cells says only that something is hidden.
    show(
      <DealIdentityFacts
        deal={{
          amount_minor: null,
          currency: "EUR",
          stage_id: "st-1",
          masked_fields: ["amount_minor"],
        }}
        stages={stages}
        locale="en"
      />,
    );
    expect(screen.getByText("Value")).toBeInTheDocument();
    expect(screen.queryByText("—")).not.toBeInTheDocument();
  });

  it("says how a won deal was won when no contract carried it", () => {
    // The server treats this answer as load-bearing — the whole justification
    // for letting the deal close without paperwork is that the gap becomes
    // countable — and nothing read it back, so the rep who answered could not
    // check their own answer.
    show(
      <DealIdentityFacts
        deal={{
          amount_minor: 1000,
          currency: "EUR",
          stage_id: "st-1",
          status: "won",
          won_without_contract_reason: "purchase_order",
        }}
        stages={stages}
        locale="en"
      />,
    );
    expect(
      screen.getByText(en["deals.winReasonPurchaseOrder"]),
    ).toBeInTheDocument();
  });

  it("prints the words a contact wrote rather than the category they chose", () => {
    // `other` is the only reason carrying a detail, and the detail is the only
    // part of this answer somebody typed. "Something else: renewed on a
    // handshake" says the category twice and buries it.
    show(
      <DealIdentityFacts
        deal={{
          amount_minor: 1000,
          currency: "EUR",
          stage_id: "st-1",
          status: "won",
          won_without_contract_reason: "other",
          won_without_contract_detail: "Renewed on a handshake at the fair",
        }}
        stages={stages}
        locale="en"
      />,
    );
    expect(
      screen.getByText("Renewed on a handshake at the fair"),
    ).toBeInTheDocument();
    expect(
      screen.queryByText(en["deals.winReasonOther"]),
    ).not.toBeInTheDocument();
  });

  it("keeps a long hand-written reason readable rather than only hoverable", () => {
    // The strip is a row of SHORT cells, so this one is bounded — but
    // VISUALLY, with the whole string in the DOM. A tooltip wants a mouse, is
    // ignored by most screen readers, and never appears for a keyboard or
    // touch reader. The contact most likely to look is the one checking the
    // words they just typed.
    const long =
      `Renewed on a handshake at the trade fair ${"and again ".repeat(20)}`.trim();
    show(
      <DealIdentityFacts
        deal={{
          amount_minor: 1000,
          currency: "EUR",
          stage_id: "st-1",
          status: "won",
          won_without_contract_reason: "other",
          won_without_contract_detail: long,
        }}
        stages={stages}
        locale="en"
      />,
    );
    expect(screen.getByText(long)).toBeInTheDocument();
    expect(screen.getByText(long)).toHaveClass("deal-win-detail");
    expect(screen.getByText("Qualified")).toBeInTheDocument();
  });

  // The bound is on WIDTH, never on content — asserted against the
  // stylesheet, because jsdom applies none and a rendered tree cannot tell a
  // wrapped value from a clipped one.
  it("bounds the won-reason detail's width and never its content", () => {
    const css = readFileSync(
      join(resolve(__dirname, ".."), "dealstatus.css"),
      "utf8",
    );
    const rule = /\.deal-win-detail\s*\{([^}]*)\}/.exec(css);
    expect(rule, ".deal-win-detail is gone from dealstatus.css").not.toBeNull();
    const body = rule?.[1] ?? "";

    expect(body).toMatch(/max-width:/);
    for (const clip of [
      /overflow\s*:\s*hidden/,
      /text-overflow\s*:/,
      /white-space\s*:\s*nowrap/,
      /line-clamp\s*:/,
    ]) {
      expect(
        body,
        `.deal-win-detail clips its content (${clip.source}); the value is what a controller is shown`,
      ).not.toMatch(clip);
    }
  });

  it("says nothing about paperwork on a won deal a contract carried", () => {
    // The ordinary case, and it needs no cell. A cell on every won deal would
    // bury the ones that need reading — the whole point of showing this at all.
    show(
      <DealIdentityFacts
        deal={{
          amount_minor: 1000,
          currency: "EUR",
          stage_id: "st-1",
          status: "won",
        }}
        stages={stages}
        locale="en"
      />,
    );
    for (const key of [
      "deals.winReasonPurchaseOrder",
      "deals.winReasonVerbal",
      "deals.winReasonRenewalByEmail",
      "deals.winReasonImported",
      "deals.winReasonOther",
    ] as const) {
      expect(screen.queryByText(en[key])).not.toBeInTheDocument();
    }
  });

  it("never prints a stage id the pipeline cannot name", () => {
    // The case a null stage_id CANNOT test: an id that is present and does not
    // resolve. A deal read before its pipeline finishes loading has stages
    // empty, which reaches the fallback with a real uuid in hand, and printing
    // it puts a machine identifier where a reader expects "Qualified".
    const foreign = "01a02be8-c8d5-7d9b-bb60-a5e1ad68533c";
    show(
      <DealIdentityFacts
        deal={{ amount_minor: 1000, currency: "EUR", stage_id: foreign }}
        stages={stages}
        locale="en"
      />,
    );
    expect(screen.getByText("—")).toBeInTheDocument();
    expect(screen.queryByText(foreign)).not.toBeInTheDocument();
  });

  it("names how the record reached Margince, behind the provenance tag", async () => {
    show(
      <DealIdentityFacts
        deal={{
          amount_minor: 1000,
          currency: "EUR",
          stage_id: "st-1",
          source: "csv import",
        }}
        stages={stages}
        locale="en"
      />,
    );
    await userEvent.setup().click(screen.getByRole("button"));
    expect(await screen.findByText("csv import")).toBeInTheDocument();
  });

  it("says which partner brought the deal, sourced or influenced", () => {
    show(
      <DealIdentityFacts
        deal={{
          amount_minor: 1000,
          currency: "EUR",
          stage_id: "st-1",
          partner_company_id: "p1",
          partner_attribution: "influenced",
        }}
        stages={stages}
        locale="en"
      />,
    );
    expect(screen.getByText("helped by")).toBeInTheDocument();
  });
});

describe("the subtitle names the company beside the deal's name", () => {
  it("draws nothing when the deal names no company", () => {
    const { container } = show(<DealSubtitle deal={{ company_id: null }} />);
    expect(container.querySelector(".record-sub")).toBeNull();
  });

  it("names the field it may not show rather than a bare dash", () => {
    show(
      <DealSubtitle
        deal={{ company_id: null, masked_fields: ["company_id"] }}
      />,
    );
    expect(screen.getByText("Company")).toBeInTheDocument();
  });
});

describe("the close date says how much it is worth believing", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-08-24T00:00:00Z"));
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it("marks a date no human confirmed", () => {
    show(
      <DealIdentityFacts
        deal={{
          expected_close_date: "2099-09-30",
          close_date_provisional: true,
        }}
        stages={stages}
        locale="en"
      />,
    );
    expect(
      screen.getByText(/provisional, not confirmed by a human/),
    ).toBeInTheDocument();
  });

  it("says nothing about confirmation when a human set the date", () => {
    show(
      <DealIdentityFacts
        deal={{
          expected_close_date: "2099-09-30",
          close_date_provisional: false,
        }}
        stages={stages}
        locale="en"
      />,
    );
    expect(screen.getByText(/in [\d,]+ days/)).toBeInTheDocument();
    expect(screen.queryByText(/provisional/)).toBeNull();
  });

  it("counts the days a deal is already past, in the reader's own plural", () => {
    // The arm nothing read back: a date behind us. "1 days past the date" is
    // the wording a catalogue that spells the plural once produces on the one
    // day a rep is most likely to be looking.
    show(
      <DealIdentityFacts
        deal={{ expected_close_date: "2026-08-23" }}
        stages={stages}
        locale="en"
      />,
    );
    expect(screen.getByText("1 day past the date")).toBeInTheDocument();
  });

  // The cell stays where a date is missing: when a deal lands is a question
  // the reader came with, and a cell that disappears answers it with silence.
  it("keeps the cell and says so when nobody has set a date", () => {
    show(<DealIdentityFacts deal={{}} stages={stages} locale="en" />);
    expect(screen.getByText(en["deal.strip.close.none"])).toBeInTheDocument();
  });
});

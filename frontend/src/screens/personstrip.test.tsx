/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { PersonStrip } from "./personstrip";

type Person360 = components["schemas"]["Person360"];

// The strip and the consent gate answer the same question — has this person
// ever written to us — from the same field, `activity.direction`. They are
// computed in different places (this file counts the 360's rows; the server's
// consent verdict runs its own query), so a reader who sees "1 in" beside "they
// have never written to you" cannot tell which half to believe and learns to
// trust neither.
//
// These pin the counting rule this side owns. What makes the two agree is that
// both read `direction`, and a change here that started counting something else
// — a participant row, a thread the person appears on — is what would split
// them.

// The three directions the contract admits (Activity.direction), so the
// fixture cannot drift into values the server never sends.
type Direction = "inbound" | "outbound" | "internal";

function viewWith(directions: readonly Direction[]): Person360 {
  return {
    person: { id: "p1", full_name: "Marine Raucoules" },
    activities: {
      data: directions.map((direction, i) => ({
        id: `a${i}`,
        kind: "email",
        direction,
        occurred_at: "2026-08-11T12:00:00Z",
      })),
    },
  } as unknown as Person360;
}

function renderStrip(view: Person360, locale: "en" | "de" | "vi" = "en") {
  render(
    <LocaleProvider initial={locale}>
      <PersonStrip view={view} consentVerdict={undefined} />
    </LocaleProvider>,
  );
}

function viewWithDeal(amountMinor: number, currency: string): Person360 {
  return {
    person: { id: "p1", full_name: "Marine Raucoules" },
    activities: { data: [] },
    commercial: {
      deal: {
        id: "d1",
        title: "ERP rollout",
        amount_minor: amountMinor,
        currency,
      },
    },
  } as unknown as Person360;
}

describe("the reciprocity reading", () => {
  it("counts nothing inbound for a person who has only been written TO", () => {
    renderStrip(viewWith(["outbound", "outbound"]));

    // The state behind the reported contradiction: two messages on the
    // timeline, both ours. The consent gate calls this "they have never
    // written to you", and the strip must not claim otherwise.
    expect(screen.getByText("0 in · 2 out")).toBeTruthy();
  });

  it("counts an inbound message once it exists", () => {
    renderStrip(viewWith(["inbound", "outbound"]));

    expect(screen.getByText("1 in · 1 out")).toBeTruthy();
  });

  it("counts nothing for a person with no captured activity", () => {
    renderStrip(viewWith([]));

    expect(screen.getByText("0 in · 0 out")).toBeTruthy();
  });

  // "internal" is a real direction the contract admits, and it is not evidence
  // that they wrote to us. Counting "not outbound" as inbound is the shape that
  // would make the strip claim a message the consent gate cannot see — which is
  // exactly the contradiction this pins.
  it("counts an internal row as neither", () => {
    renderStrip(viewWith(["outbound", "internal"]));

    expect(screen.getByText("0 in · 1 out")).toBeTruthy();
  });
});

// The open-deal slot used to run its own formatter: a hard-coded k/1000 tier
// and a three-entry symbol table, locale-blind, exported from a screen and
// used by personcards too. It reads through `formatMoneyCompact` now, the same
// one company360 uses for the same glance on the account page.
describe("the open-deal reading", () => {
  it("abbreviates in the READER's conventions, not in one table's", () => {
    // The defect the shared formatter closes: a table that says "k" says it to
    // everybody. German abbreviates at the million and writes the symbol after
    // the figure, and neither is something a screen gets to decide.
    renderStrip(viewWithDeal(1_999_999_900, "EUR"), "en");
    expect(screen.getByText("€20m")).toBeTruthy();

    cleanup();
    renderStrip(viewWithDeal(1_999_999_900, "EUR"), "de");
    expect(screen.getByText("20 Mio. €")).toBeTruthy();
  });

  it("reads the scale off the CURRENCY, so a zero-decimal one is not divided", () => {
    // ₫18,000,000 is eighteen million dong, not a hundred and eighty thousand.
    // A hard-coded /100 understated every zero-decimal currency a hundredfold.
    renderStrip(viewWithDeal(18_000_000, "VND"), "en");
    expect(screen.getByText("₫18m")).toBeTruthy();
  });

  it("keeps a credit's sign in front of the figure", () => {
    renderStrip(viewWithDeal(-9_500_000, "EUR"), "en");
    expect(screen.getByText("-€95k")).toBeTruthy();
  });
});

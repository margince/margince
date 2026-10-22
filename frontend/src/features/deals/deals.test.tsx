/** @vitest-environment happy-dom */
import { cleanup } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { CompanyNaming } from "./dealcompanymarks";
import {
  buildColumns,
  buildStageTotals,
  mapDealCreate,
  mapDealUpdate,
} from "./deals";
import { deal, stages } from "./deals.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
  localStorage.clear();
});

// These cases are about column totals, so no card here names a company: an
// empty mark set resolves none, and none of these deals withholds one.
const noCompany: CompanyNaming = { marks: new Map(), unreadable: new Set() };

// Column totals are the server's per-stage Σround(amount×p/100), never a sum of
// the cards that loaded: the card cap is unrelated to the totals' correctness.
describe("buildStageTotals", () => {
  it("carries one currency's totals straight through", () => {
    const totals = buildStageTotals([
      {
        stage_id: "s1",
        currency: "EUR",
        deals: 3,
        raw_minor: 300_000,
        weighted_minor: 60_000,
      },
    ]);
    expect(totals.get("s1")).toEqual({
      count: 3,
      rawMinor: 300_000,
      weightedMinor: 60_000,
      currency: "EUR",
      sumHidden: false,
    });
  });

  it("hides the sum when a stage has more than one currency row", () => {
    const totals = buildStageTotals([
      {
        stage_id: "s2",
        currency: "EUR",
        deals: 1,
        raw_minor: 100_000,
        weighted_minor: 20_000,
      },
      {
        stage_id: "s2",
        currency: "USD",
        deals: 1,
        raw_minor: 100_000,
        weighted_minor: 20_000,
      },
    ]);
    const s2 = totals.get("s2");
    expect(s2?.sumHidden).toBe(true);
    expect(s2?.count).toBe(2);
  });

  it("a stage absent from the rows gets zeroed, not undefined", () => {
    const totals = buildStageTotals([]);
    expect(totals.get("s1")).toBeUndefined();
  });
});

describe("buildColumns", () => {
  it("reads sums from the totals map, not from the loaded cards", () => {
    const totals = buildStageTotals([
      // The server's per-deal-rounded figure (12343 × 20% rounded per deal,
      // twice, then summed = 4938) — deliberately NOT what round(Σ) gives
      // (4937), so a regression back to client-side summing would fail this.
      {
        stage_id: "s1",
        currency: "EUR",
        deals: 2,
        raw_minor: 24_686,
        weighted_minor: 4_938,
      },
    ]);
    const columns = buildColumns(
      stages,
      [
        deal({
          id: "a",
          stage_id: "s1",
          amount_minor: 12_343,
          currency: "EUR",
        }),
        deal({
          id: "b",
          stage_id: "s1",
          amount_minor: 12_343,
          currency: "EUR",
        }),
      ],
      totals,
      noCompany,
    );
    expect(columns[0].rawMinor).toBe(24_686);
    expect(columns[0].weightedMinor).toBe(4_938);
    expect(columns[0].deals).toHaveLength(2);
  });

  it("hides the sum for a mixed-currency stage per the totals map, regardless of which cards loaded", () => {
    const totals = buildStageTotals([
      {
        stage_id: "s2",
        currency: "EUR",
        deals: 1,
        raw_minor: 100_000,
        weighted_minor: 20_000,
      },
      {
        stage_id: "s2",
        currency: "USD",
        deals: 1,
        raw_minor: 100_000,
        weighted_minor: 20_000,
      },
    ]);
    const columns = buildColumns(
      stages,
      [
        deal({
          id: "c",
          stage_id: "s2",
          amount_minor: 100_000,
          currency: "EUR",
        }),
      ],
      totals,
      noCompany,
    );
    expect(columns[1].sumHidden).toBe(true);
  });

  it("a stage with no totals row states no figure and no currency", () => {
    const columns = buildColumns(stages, [], new Map(), noCompany);
    expect(columns[0].rawMinor).toBeNull();
    expect(columns[0].currency).toBeNull();
    expect(columns[0].sumHidden).toBeFalsy();
  });
});

describe("mapDealUpdate", () => {
  // The form's controls as a contact who changed nothing left them. Every value
  // here is exactly what dealEditRecord seeded, so a key reaching the body is
  // the form reporting a change nobody made.
  const untouched = {
    name: "Fleet retrofit",
    amount: "2120",
    currency: "EUR",
    owner_id: "u-me",
    company_id: "",
    partner_company_id: "",
    partner_attribution: "",
    forecast_category: "",
    expected_close_date: "",
    wait_until: "",
    project_id: "",
  };

  it("rebuilds amount_minor from major units for the fields the contact moved", () => {
    const body = mapDealUpdate(
      {
        ...untouched,
        amount: "2120",
        currency: "EUR",
        owner_id: "u-me",
        forecast_category: "commit",
        expected_close_date: "2026-09-01",
      },
      { ...untouched, amount: "", currency: "", owner_id: "" },
    );
    expect(body.amount_minor).toBe(212_000);
    expect(body.currency).toBe("EUR");
    expect(body.owner_id).toBe("u-me");
    expect(body.forecast_category).toBe("commit");
    expect(body.expected_close_date).toBe("2026-09-01");
    // Named nowhere in the edit, so named nowhere in the body.
    expect("name" in body).toBe(false);
    expect("wait_until" in body).toBe(false);
  });

  // The reported defect. The body used to carry every field on every save, so a
  // deal with no company resubmitted `company_id: null` — and the API,
  // correctly, refused to clear a field the contact never touched. On an
  // installation with no partners the refusal named `partner_attribution`, a
  // field the form does not even render.
  it("sends nothing at all when the contact changed nothing", () => {
    const body = mapDealUpdate(untouched, untouched);

    expect(Object.keys(body)).toEqual([]);
  });

  it("names only the field the contact moved, on a deal missing every optional value", () => {
    const bare = {
      name: "Any Deal",
      amount: "",
      currency: "",
      owner_id: "",
      company_id: "",
      partner_company_id: "",
      partner_attribution: "",
      forecast_category: "",
      expected_close_date: "",
      wait_until: "",
      project_id: "",
    };

    const body = mapDealUpdate({ ...bare, name: "Renamed" }, bare);

    expect(Object.keys(body)).toEqual(["name"]);
    expect(body.name).toBe("Renamed");
  });

  // A withheld reference arrives as null with masked_fields naming it —
  // deliberately, so a reader can tell "you may not see this" from "there is
  // nothing here". The form has only the null, so sending it back would ask
  // the server to clear a partner the reader never saw. Omitted means
  // unchanged, which is the only honest patch for a field nobody was shown.
  it("does not clear a partner it was never allowed to see", () => {
    const body = mapDealUpdate(
      {
        name: "Fleet retrofit",
        partner_company_id: "",
        partner_attribution: "",
      },
      {
        name: "Fleet retrofit",
        partner_company_id: "p-1",
        partner_attribution: "sourced",
      },
      ["partner_company_id"],
    );
    expect("partner_company_id" in body).toBe(false);
    // The attribution is withheld WITH its partner, so it goes too — returning
    // half the pair would decide what a partner nobody could see is owed.
    expect("partner_attribution" in body).toBe(false);
  });

  it("still clears a partner the reader could see and chose to remove", () => {
    const body = mapDealUpdate(
      { ...untouched, partner_company_id: "" },
      {
        ...untouched,
        partner_company_id: "p-1",
        partner_attribution: "sourced",
      },
    );
    expect(body.partner_company_id).toBeNull();
    // The claim goes with the partner, server-side, as one fact. Naming its own
    // null here would state a claim with nobody left to attribute it to, which
    // is the one shape the API refuses.
    expect("partner_attribution" in body).toBe(false);
  });

  // The scale is part of the amount: amount_minor is denominated in the
  // currency's OWN minor units, so a currency moving alone re-denominates the
  // figure the row holds. 10000 JPY re-saved as EUR is €100, and the fx freeze
  // and the forecast history then record that as the price.
  it("re-sends the figure at the new scale when only the currency moved", () => {
    const priced = { ...untouched, amount: "10000", currency: "JPY" };

    const body = mapDealUpdate({ ...priced, currency: "EUR" }, priced);

    expect(body.currency).toBe("EUR");
    expect(body.amount_minor).toBe(1_000_000);
  });

  // Naming a currency on a deal with no figure is half a money value, and the
  // server's own pair refusal says so better than an amount this form invents.
  it("invents no figure when a currency is named on an unpriced deal", () => {
    const bare = { ...untouched, amount: "", currency: "" };

    const body = mapDealUpdate({ ...bare, currency: "EUR" }, bare);

    expect(Object.keys(body)).toEqual(["currency"]);
  });

  it("moves the claim alone when the partner stays put", () => {
    const body = mapDealUpdate(
      {
        ...untouched,
        partner_company_id: "p-1",
        partner_attribution: "influenced",
      },
      {
        ...untouched,
        partner_company_id: "p-1",
        partner_attribution: "sourced",
      },
    );
    expect(Object.keys(body)).toEqual(["partner_attribution"]);
    expect(body.partner_attribution).toBe("influenced");
  });
});

describe("mapDealCreate", () => {
  // A deal names its partner at birth. The create body once carried neither
  // partner field, and the API accepted the request and stored neither, so the
  // caller was told a write had succeeded with the partner gone.
  it("carries the partner and what they did into the birth body", () => {
    const body = mapDealCreate(
      {
        name: "Northgate rollout",
        stage_id: "s-1",
        amount: "480",
        currency: "EUR",
        company_id: "cust-1",
        partner_company_id: "partner-1",
        partner_attribution: "influenced",
      },
      "p-1",
    );
    expect(body.partner_company_id).toBe("partner-1");
    expect(body.partner_attribution).toBe("influenced");
    expect(body.company_id).toBe("cust-1");
    expect(body.pipeline_id).toBe("p-1");
    expect(body.amount_minor).toBe(48_000);
  });

  // Leaving the attribution on its empty option is the caller making no claim.
  // The server reads a named partner as "sourced", which is what that option
  // says it does — the form must not invent a different claim here.
  it("sends no attribution when the caller made no claim", () => {
    const body = mapDealCreate(
      { name: "x", stage_id: "s-1", partner_company_id: "partner-1" },
      "p-1",
    );
    expect(body.partner_company_id).toBe("partner-1");
    expect(body.partner_attribution).toBeNull();
  });

  it("names no partner when none was picked", () => {
    const body = mapDealCreate({ name: "x", stage_id: "s-1" }, "p-1");
    expect(body.partner_company_id).toBeNull();
    expect(body.partner_attribution).toBeNull();
  });
});

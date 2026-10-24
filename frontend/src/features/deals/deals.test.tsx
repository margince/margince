/** @vitest-environment happy-dom */
import { cleanup } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { UTC_ZONE } from "../../format/timezone";
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
describe("server-valued stage totals", () => {
  it("keeps the server base value and rate date across mixed-currency cards", () => {
    const totals = buildStageTotals(
      [
        {
          stage_id: "s1",
          deals: 3,
          priced_deals: 2,
          base_minor: 24_686,
          weighted_base_minor: 4_938,
        },
      ],
      "EUR",
      "2026-10-01",
      UTC_ZONE,
    );
    expect(totals.get("s1")).toEqual({
      count: 3,
      baseMinor: 24_686,
      weightedBaseMinor: 4_938,
      currency: "EUR",
      asOf: "2026-10-01",
      timezone: UTC_ZONE,
      unpricedCount: 1,
    });
    const columns = buildColumns(
      stages,
      [deal({ stage_id: "s1", amount_minor: 1, currency: "USD" })],
      totals,
      noCompany,
    );
    expect(columns[0]).toMatchObject({
      rawMinor: 24_686,
      weightedMinor: 4_938,
      currency: "EUR",
      count: 3,
      asOf: "2026-10-01",
      timezone: UTC_ZONE,
      unpricedCount: 1,
      sumHidden: false,
    });
  });
  it("keeps a missing price absent and states how many deals were excluded", () => {
    const totals = buildStageTotals(
      [
        {
          stage_id: "s1",
          deals: 2,
          priced_deals: 0,
          base_minor: null,
          weighted_base_minor: null,
        },
      ],
      "EUR",
      "2026-10-01",
    );
    expect(totals.get("s1")).toMatchObject({
      baseMinor: null,
      weightedBaseMinor: null,
      unpricedCount: 2,
    });
  });
  it("keeps unavailable totals distinct from zero and preserves withholding", () => {
    expect(buildStageTotals([]).get("s1")).toBeUndefined();
    const columns = buildColumns(
      stages,
      [],
      new Map(),
      noCompany,
      "Totals withheld",
    );
    expect(columns[0]).toMatchObject({
      rawMinor: null,
      currency: null,
      sumHidden: true,
      sumHiddenReason: "Totals withheld",
    });
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
      "EUR",
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
      "EUR",
    );
    expect(body.partner_company_id).toBe("partner-1");
    expect(body.partner_attribution).toBeNull();
  });

  it("names no partner when none was picked", () => {
    const body = mapDealCreate({ name: "x", stage_id: "s-1" }, "p-1", "EUR");
    expect(body.partner_company_id).toBeNull();
    expect(body.partner_attribution).toBeNull();
  });
});

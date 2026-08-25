import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { type Locale, type Translator, translate } from "../i18n";
import {
  canonical,
  FACT_FIELD_LABELS,
  factFieldLabelKey,
  groupFacts,
} from "./factview";

type OrganizationFact = components["schemas"]["OrganizationFact"];

function fact(over: Partial<OrganizationFact> = {}): OrganizationFact {
  return {
    // The stored row's id, so a claim written from this fact can cite
    // something the reader can open.
    id: "00000000-0000-4000-8000-000000000001",
    category: "market",
    field: "served_industry",
    value: "E-Commerce",
    value_key: "e-commerce",
    source: "site_read",
    captured_by: "agent:site-read",
    updated_at: "2026-07-29T07:37:00Z",
    ...over,
  };
}

// The real catalogue lookup, not a stub: the ordering cases below turn on what
// a label actually SAYS in German, so a double that returned the key would
// prove the sort ran and nothing about what it produced.
const tFor =
  (locale: Locale): Translator =>
  (key, params) =>
    translate(locale, key, params);

const values = (groups: ReturnType<typeof groupFacts>, category: string) =>
  groups.find((g) => g.category === category)?.facts.map((f) => f.value) ?? [];

const fields = (groups: ReturnType<typeof groupFacts>, category: string) =>
  groups.find((g) => g.category === category)?.facts.map((f) => f.field) ?? [];

describe("canonical", () => {
  it("collapses the spellings a scrape produces of one value", () => {
    expect(canonical("Shop-Devs")).toBe(canonical("Shop Devs"));
    expect(canonical("shop devs")).toBe(canonical("Shop-Devs"));
  });

  it("keeps diacritics, which carry meaning in German", () => {
    expect(canonical("Prüfung")).not.toBe(canonical("Prufung"));
  });
});

describe("groupFacts", () => {
  it("shows one row for two spellings of the same fact", () => {
    const groups = groupFacts(
      [fact({ value: "Shop Devs" }), fact({ value: "Shop-Devs" })],
      tFor("en"),
      "en",
    );
    expect(values(groups, "market")).toHaveLength(1);
  });

  it("keeps the same value under two different fields apart", () => {
    // A company can genuinely be both a partner and a named customer, and the
    // field is what distinguishes the two statements.
    const groups = groupFacts(
      [
        fact({ category: "signal", field: "partner", value: "bitExpert" }),
        fact({
          category: "signal",
          field: "named_customer",
          value: "bitExpert",
        }),
      ],
      tFor("en"),
      "en",
    );
    expect(values(groups, "signal")).toHaveLength(2);
  });

  it("collapses one offering listed as product, service and capability", () => {
    const groups = groupFacts(
      [
        fact({ category: "offering", field: "capability", value: "PaaS" }),
        fact({ category: "offering", field: "service", value: "PaaS" }),
        fact({ category: "offering", field: "product", value: "PaaS" }),
      ],
      tFor("en"),
      "en",
    );
    const offering = groups.find((g) => g.category === "offering");
    expect(offering?.facts).toHaveLength(1);
    // Product is the most concrete of the three, so it is the one kept.
    expect(offering?.facts[0].field).toBe("product");
  });

  it("collapses on the server's key, which is what real values need", () => {
    // Facts read off a site are "Name - what it does". The server already
    // normalized the name into value_key; recomputing identity from the whole
    // displayed string left these as two rows, so the collapse did nothing on
    // exactly the shape production sends.
    const groups = groupFacts(
      [
        fact({
          category: "offering",
          field: "product",
          value: "Frontic - Commerce platform",
          value_key: "frontic",
        }),
        fact({
          category: "offering",
          field: "service",
          value: "Frontic - Storefront delivery",
          value_key: "frontic",
        }),
      ],
      tFor("en"),
      "en",
    );
    expect(groups.find((g) => g.category === "offering")?.facts).toHaveLength(
      1,
    );
  });

  it("keeps a human value even when a machine one holds the better field", () => {
    // Product outranks service, and a human outranks a machine. Applying the
    // field rank first let a site read's product hide a human's service for
    // the same offering, which is machine over human.
    const groups = groupFacts(
      [
        fact({
          category: "offering",
          field: "product",
          value: "PaaS",
          source: "site_read",
          confidence: 0.9,
        }),
        fact({
          category: "offering",
          field: "service",
          value: "PaaS",
          source: "human",
        }),
      ],
      tFor("en"),
      "en",
    );
    const offering = groups.find((g) => g.category === "offering");
    expect(offering?.facts).toHaveLength(1);
    expect(offering?.facts[0].source).toBe("human");
  });

  it("keeps a human-held value over a more confident machine one", () => {
    const groups = groupFacts(
      [
        fact({ value: "E-Commerce", source: "site_read", confidence: 0.9 }),
        fact({ value: "e commerce", source: "human", confidence: 0.1 }),
      ],
      tFor("en"),
      "en",
    );
    expect(groups[0].facts[0].source).toBe("human");
  });

  it("orders the most confident fact first", () => {
    const groups = groupFacts(
      [
        fact({ value: "Agenturen", confidence: 0.2 }),
        fact({ value: "Shopbetreiber", confidence: 0.9 }),
      ],
      tFor("en"),
      "en",
    );
    expect(values(groups, "market")[0]).toBe("Shopbetreiber");
  });

  it("orders categories the same way every time", () => {
    const groups = groupFacts(
      [
        fact({ category: "signal", field: "technology", value: "Redis" }),
        fact({ category: "company", field: "phone", value: "+49 30 1" }),
        fact({ category: "offering", field: "product", value: "Frontic" }),
      ],
      tFor("en"),
      "en",
    );
    expect(groups.map((g) => g.category)).toEqual([
      "company",
      "offering",
      "signal",
    ]);
  });

  it("omits a category with no facts rather than drawing an empty heading", () => {
    const groups = groupFacts([fact({ category: "market" })], tFor("en"), "en");
    expect(groups.map((g) => g.category)).toEqual(["market"]);
  });

  it("breaks a field tie on the label the reader SEES, not the wire name", () => {
    // The row draws `t(factFieldLabelKey(field))`, so an order computed from
    // the identifiers is an order in words nobody reads. English and German
    // disagree here in opposite directions, which is the whole point:
    // employee_range < founded_year as identifiers and as "Employees" before
    // "Founded", but "Gegründet" comes before "Mitarbeitende".
    const tie = [
      fact({
        category: "company",
        field: "employee_range",
        value: "120",
        value_key: "120",
      }),
      fact({
        category: "company",
        field: "founded_year",
        value: "2014",
        value_key: "2014",
      }),
    ];
    expect(fields(groupFacts(tie, tFor("en"), "en"), "company")).toEqual([
      "employee_range",
      "founded_year",
    ]);
    expect(fields(groupFacts(tie, tFor("de"), "de"), "company")).toEqual([
      "founded_year",
      "employee_range",
    ]);
  });

  it("breaks a value tie in the READER's alphabet, not in code units", () => {
    // Both values are rendered, so the tiebreaker is a list a person scans.
    // Code-unit order puts every accented vowel after Z, which is why a German
    // reader used to find "Ähnliche Marken" below "Zielgruppe" — the two
    // orderings disagree here, and only one of them is what the reader expects.
    const tie = [
      fact({ value: "Zielgruppe", value_key: "zielgruppe" }),
      fact({ value: "Ähnliche Marken", value_key: "ahnliche-marken" }),
      fact({ value: "Absatzmarkt", value_key: "absatzmarkt" }),
    ];
    expect(values(groupFacts(tie, tFor("de"), "de"), "market")).toEqual([
      "Absatzmarkt",
      "Ähnliche Marken",
      "Zielgruppe",
    ]);
  });

  it("names every field the schema allows", () => {
    // The exhaustiveness is enforced by the TYPE: FACT_FIELD_LABELS is a
    // Record<FactField, MessageKey>, so a fact field added to the contract
    // fails the typecheck until it is named here. This walks that map to prove
    // the second half — every entry resolves to its own key, so no field
    // reaches a German reader as English snake_case. A hand-written list would
    // have proved neither: its annotation rejects invalid names without
    // requiring the valid ones.
    const fields = Object.keys(
      FACT_FIELD_LABELS,
    ) as OrganizationFact["field"][];
    expect(fields.length).toBeGreaterThan(0);
    for (const field of fields) {
      expect(factFieldLabelKey(field)).toBe(`co.factField.${field}`);
    }
  });
});

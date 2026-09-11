import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import {
  counterpartyRef,
  edgeOptions,
  endpointBody,
  type RelationshipScope,
} from "./relationships";

type Relationship = components["schemas"]["Relationship"];

// The relationship picker's kind→endpoint mapping mirrors the backend
// rel_*_shape CHECK constraints (migration 0007). These pure-function specs
// pin that mapping so the UI can never offer a (scope, kind) it can't satisfy
// — the mismatch that used to reach the server as a "endpoint shape is
// required" 422. Interactive coverage of the picker lives in contacts.test.tsx
// / companies.test.tsx; this file is the invariant itself.

const personScope: RelationshipScope = { person_id: "p-1" };
const companyScope: RelationshipScope = { company_id: "o-1" };
const dealScope: RelationshipScope = { deal_id: "d-1" };

function baseRel(over: Partial<Relationship>): Relationship {
  return {
    id: "rel-1",
    kind: "employment",
    is_current_primary: false,
    source: "manual",
    captured_by: "human:u-1",
    version: 1,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...over,
  };
}

describe("edgeOptions — creatable kinds per scope", () => {
  it("a person anchors employment (→company) and deal_stakeholder (→deal), nothing company↔company", () => {
    expect(edgeOptions(personScope)).toEqual([
      { kind: "employment", entity: "company", field: "company_id" },
      { kind: "deal_stakeholder", entity: "deal", field: "deal_id" },
    ]);
  });

  // A deal had no scope of its own, so a stakeholder was creatable only from
  // the PERSON's side: adding a champion meant knowing which contact to open
  // first, and a deal nobody had linked from a contact page had no way in.
  it("a deal anchors its stakeholders and nothing else", () => {
    expect(edgeOptions(dealScope)).toEqual([
      { kind: "deal_stakeholder", entity: "person", field: "person_id" },
    ]);
  });

  it("a company anchors employment (→person) and the three company↔company kinds (→counterparty), never deal_stakeholder", () => {
    expect(edgeOptions(companyScope)).toEqual([
      { kind: "employment", entity: "person", field: "person_id" },
      {
        kind: "partner_of",
        entity: "company",
        field: "counterparty_company_id",
      },
      {
        kind: "referred_by",
        entity: "company",
        field: "counterparty_company_id",
      },
      {
        kind: "co_sell_with",
        entity: "company",
        field: "counterparty_company_id",
      },
    ]);
    expect(
      edgeOptions(companyScope).some((o) => o.kind === "deal_stakeholder"),
    ).toBe(false);
  });
});

describe("endpointBody — the picked id lands on exactly one field", () => {
  it("maps each field to its own key and no other", () => {
    expect(endpointBody("company_id", "x")).toEqual({
      company_id: "x",
    });
    expect(endpointBody("person_id", "x")).toEqual({ person_id: "x" });
    expect(endpointBody("counterparty_company_id", "x")).toEqual({
      counterparty_company_id: "x",
    });
    expect(endpointBody("deal_id", "x")).toEqual({ deal_id: "x" });
  });
});

describe("counterpartyRef — the other end of an existing edge, typed for EntityRef", () => {
  it("a deal edge resolves to the deal regardless of scope", () => {
    const rel = baseRel({ kind: "deal_stakeholder", deal_id: "d-1" });
    expect(counterpartyRef(rel, personScope)).toEqual({
      kind: "deal",
      id: "d-1",
    });
  });

  // From the deal, the far end is the PERSON — the deal is the anchor, not the
  // counterparty, so returning the deal here would point every row at the page
  // the reader is already on.
  it("a stakeholder edge resolves to the person when the deal is the scope", () => {
    const rel = baseRel({
      kind: "deal_stakeholder",
      deal_id: "d-1",
      person_id: "p-1",
    });
    expect(counterpartyRef(rel, dealScope)).toEqual({
      kind: "person",
      id: "p-1",
    });
  });

  it("names no far end for a deal edge that carries no person", () => {
    expect(counterpartyRef(baseRel({ deal_id: "d-1" }), dealScope)).toBeNull();
  });

  it("a company↔company edge resolves to the counterparty company from the anchor side", () => {
    const rel = baseRel({
      kind: "partner_of",
      company_id: "o-1",
      counterparty_company_id: "o-2",
    });
    expect(counterpartyRef(rel, companyScope)).toEqual({
      kind: "company",
      id: "o-2",
    });
  });

  it("resolves to the OTHER company when the same edge is viewed from the counterparty side", () => {
    // The company list filter matches on either end, so this partner_of edge also
    // appears on o-2's tab; the far end there is the anchor o-1, never o-2.
    const rel = baseRel({
      kind: "partner_of",
      company_id: "o-1",
      counterparty_company_id: "o-2",
    });
    expect(counterpartyRef(rel, { company_id: "o-2" })).toEqual({
      kind: "company",
      id: "o-1",
    });
  });

  it("an employment edge resolves to whichever endpoint the scope is NOT", () => {
    const rel = baseRel({
      kind: "employment",
      person_id: "p-1",
      company_id: "o-1",
    });
    // From the person's 360 the counterparty is the company; from the company's, the person.
    expect(counterpartyRef(rel, personScope)).toEqual({
      kind: "company",
      id: "o-1",
    });
    expect(counterpartyRef(rel, companyScope)).toEqual({
      kind: "person",
      id: "p-1",
    });
  });

  it("returns null when no counterparty endpoint is present", () => {
    expect(
      counterpartyRef(baseRel({ person_id: "p-1" }), personScope),
    ).toBeNull();
  });
});

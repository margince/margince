import { describe, expect, it } from "vitest";
import { ENTITY, ENTITY_KINDS, recordRoute, SCREEN_ENTITY } from "./entity";

describe("ENTITY registry", () => {
  it("covers exactly the five record kinds (no activity)", () => {
    expect([...ENTITY_KINDS]).toEqual([
      "contact",
      "company",
      "deal",
      "lead",
      "project",
    ]);
    expect(Object.keys(ENTITY).sort()).toEqual([
      "company",
      "contact",
      "deal",
      "lead",
      "project",
    ]);
  });

  it("maps each kind to its 360 route", () => {
    expect(ENTITY.contact.route("p-1")).toEqual({
      screen: "contacts",
      id: "p-1",
    });
    expect(ENTITY.company.route("o-1")).toEqual({
      screen: "companies",
      id: "o-1",
    });
    expect(ENTITY.deal.route("d-1")).toEqual({ screen: "deals", id: "d-1" });
    expect(ENTITY.lead.route("l-1")).toEqual({ screen: "leads", id: "l-1" });
    expect(ENTITY.project.route("pr-1")).toEqual({
      screen: "projects",
      id: "pr-1",
    });
  });

  it("reverses every route into SCREEN_ENTITY, with nothing left over", () => {
    expect(SCREEN_ENTITY).toEqual({
      contacts: "contact",
      companies: "company",
      deals: "deal",
      leads: "lead",
      projects: "project",
    });
    // Derived, not restated: adding a kind to ENTITY must extend the reverse map
    // on its own, or the breadcrumb quietly falls back to a raw uuid.
    for (const kind of ENTITY_KINDS) {
      expect(SCREEN_ENTITY[ENTITY[kind].route("x").screen]).toBe(kind);
    }
  });

  it("leaves a screen with no record segment unresolved", () => {
    expect(SCREEN_ENTITY.reports).toBeUndefined();
    expect(SCREEN_ENTITY.tasks).toBeUndefined();
  });
});

// Whether a typed reference off the wire may be offered as a link at all.
//
// Three surfaces ask it: an approval's undo — the offer is only honest for an
// approval naming a record with a history panel — a worklist row's subject, and
// a notification's target. One rule, so one function, and these are its cases.
describe("recordRoute", () => {
  it("routes each record kind to the screen that holds its history", () => {
    expect(recordRoute("deal", "d1")).toEqual({ screen: "deals", id: "d1" });
    expect(recordRoute("company", "o1")).toEqual({
      screen: "companies",
      id: "o1",
    });
    expect(recordRoute("contact", "p1")).toEqual({
      screen: "contacts",
      id: "p1",
    });
    expect(recordRoute("lead", "l1")).toEqual({ screen: "leads", id: "l1" });
    expect(recordRoute("project", "pr1")).toEqual({
      screen: "projects",
      id: "pr1",
    });
  });

  // A reference that names no record — a step-up approval, a held scheduled
  // send, a notice about a backlog rather than a row — points at nothing a page
  // could show, so there is nothing to offer.
  it("offers nothing when nothing is named", () => {
    expect(recordRoute(null, null)).toBeUndefined();
    expect(recordRoute(undefined, undefined)).toBeUndefined();
    expect(recordRoute("deal", null)).toBeUndefined();
    expect(recordRoute(null, "d1")).toBeUndefined();
  });

  // activity is served by the history engine and has no record page. Offering
  // it would send a reader to a screen that cannot answer, which is worse than
  // not offering: the link would promise a page the product cannot reach.
  it("offers nothing for a kind with no record page", () => {
    expect(recordRoute("activity", "a1")).toBeUndefined();
    expect(recordRoute("approval", "ap1")).toBeUndefined();
    expect(recordRoute("", "x1")).toBeUndefined();
  });
});

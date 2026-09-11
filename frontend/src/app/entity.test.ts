import { describe, expect, it } from "vitest";
import { ENTITY, ENTITY_KINDS, SCREEN_ENTITY } from "./entity";

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
      "deal",
      "lead",
      "contact",
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

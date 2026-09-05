import { describe, expect, it } from "vitest";
import { meFixture } from "../app/mefixture";
import {
  holds,
  SETTINGS_GROUPS,
  SETTINGS_PAGES,
  type SettingsPageId,
  visibleSettingsPages,
} from "./settingscatalog";

// The catalog is the single answer four surfaces resolve from, so what it says
// has to be checkable without rendering any of them.

describe("the catalog's shape", () => {
  it("gives every page a unique id", () => {
    const ids = SETTINGS_PAGES.map((page) => page.id);
    expect(new Set(ids).size).toBe(ids.length);
  });

  it("places every page in a declared group", () => {
    for (const page of SETTINGS_PAGES) {
      expect(SETTINGS_GROUPS).toContain(page.group);
    }
  });

  it("leaves no group empty", () => {
    // An empty group renders as a heading with nothing under it. Catching it
    // here is cheaper than noticing it in a screenshot.
    for (const group of SETTINGS_GROUPS) {
      expect(SETTINGS_PAGES.some((page) => page.group === group)).toBe(true);
    }
  });
});

describe("who may open what", () => {
  // The snapshot states each case names, built the way the app receives them.
  const nobody = undefined;
  const rep = meFixture({
    roles: ["rep"],
    allow: {
      person: ["read"],
      pipeline: ["read"],
      custom_field: ["read"],
      tag: ["read"],
      product: ["read"],
      capture_settings: ["read"],
      knowledge_corpus: ["read"],
      automation: ["read"],
    },
  });

  function visibleIds(snapshot: Parameters<typeof visibleSettingsPages>[0]) {
    return visibleSettingsPages(snapshot).map((page) => page.id);
  }

  it("shows the personal pages to everyone, including before /me resolves", () => {
    // These carry no grant because they are about the reader themselves. They
    // must survive the loading window too, or the rail flashes empty on every
    // first paint.
    expect(visibleIds(nobody)).toEqual([
      "account",
      "voice",
      "agents",
      "connections",
      "capture-activity",
      "members",
      "teams",
    ]);
  });

  it("opens the sales pages a rep's own grants already carry", () => {
    // The point of the redesign: these were hidden behind an operator-seat
    // check while the API answered a rep 200 on every one of them.
    const seen = visibleIds(rep);
    for (const id of [
      "pipelines",
      "leads",
      "fields",
      "tags",
      "products",
      "capture",
      "knowledge",
      "automations",
    ] satisfies SettingsPageId[]) {
      expect(seen).toContain(id);
    }
  });

  it("withholds the governance pages from that same rep", () => {
    const seen = visibleIds(rep);
    for (const id of [
      "audit",
      "system-health",
      "extensions",
      "reset",
      "roles",
      "authentication",
      "seats",
    ] satisfies SettingsPageId[]) {
      expect(seen).not.toContain(id);
    }
  });

  it("opens privacy to a rep, because its purposes list is gated on person.read", () => {
    // Not a widening: consent/store.go's ListPurposes calls Require(person,
    // read), so a rep already reads it. A page that refused them here would
    // disagree with the endpoint behind it.
    expect(visibleIds(rep)).toContain("privacy");
  });
});

describe("requirements that are not permissions", () => {
  it("withholds the company page when the installation lacks the surface", () => {
    // organization.read alone. The company profile ANDs its grant with a
    // deployment flag, so a reader holding only that grant sees nothing when
    // the flag is off — the surface may genuinely not exist here.
    const holder = meFixture({
      allow: { organization: ["read"] },
      settingsAvailability: { company_context: false },
    });
    expect(visibleSettingsPages(holder).map((p) => p.id)).not.toContain(
      "company",
    );
  });

  it("shows it once the installation has it", () => {
    const holder = meFixture({
      allow: { organization: ["read"] },
      settingsAvailability: { company_context: true },
    });
    expect(visibleSettingsPages(holder).map((p) => p.id)).toContain("company");
  });

  it("withholds it when /me carries no availability at all", () => {
    // A server older than the field, or a snapshot cached before it shipped.
    // Absent is not permission: it has to read as "no such surface here".
    const holder = meFixture({
      allow: { organization: ["read"] },
      settingsAvailability: null,
    });
    expect(visibleSettingsPages(holder).map((p) => p.id)).not.toContain(
      "company",
    );
  });

  it("still shows it to a reader whose OTHER grant carries the page", () => {
    // The flag gates one card, not the page: installation_settings.read opens
    // the company page regardless, and treating the flag as a page-level
    // condition would hide a surface the reader may use.
    const admin = meFixture({
      allow: { installation_settings: ["read"] },
      settingsAvailability: { company_context: false },
    });
    expect(visibleSettingsPages(admin).map((p) => p.id)).toContain("company");
  });
});

describe("holds — the evaluator the four surfaces share", () => {
  it("denies a grant arm while /me is unresolved", () => {
    expect(
      holds({ kind: "grant", object: "person", action: "read" }, undefined),
    ).toBe(false);
  });

  it("reads `any` as at-least-one and `all` as every", () => {
    const me = meFixture({ allow: { person: ["read"] } });
    const person = { kind: "grant", object: "person", action: "read" } as const;
    const deal = { kind: "grant", object: "deal", action: "read" } as const;

    expect(holds({ kind: "any", of: [person, deal] }, me)).toBe(true);
    expect(holds({ kind: "all", of: [person, deal] }, me)).toBe(false);
    expect(holds({ kind: "all", of: [person] }, me)).toBe(true);
  });
});

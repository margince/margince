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

// Every page whose requirement DIFFERS from the entry that shipped, named with
// the reader it now admits and the reason the server already admits them.
//
// These exist because a review found the previous cases could not see any of
// it: they drove one rep-shaped fixture, and a widening only shows up under an
// ASYMMETRIC snapshot — a reader holding the new object and not the old one.
// Each case below is a deliberate correction of a client that disagreed with
// the server, and stating them as tests is what stops the next author reading
// the difference as an accident.
describe("pages that admit readers the old register refused", () => {
  function opens(page: SettingsPageId, allow: Parameters<typeof meFixture>[0]) {
    return visibleSettingsPages(meFixture(allow)).some((p) => p.id === page);
  }

  it("opens extensions to a grant holder, not only the literal admin role", () => {
    // `GET /v1/extensions` gates on extension_access.read, which ops holds. The
    // old entry asked whether the reader WAS an admin, so it refused an ops
    // principal the server answers 200 — the client disagreeing with the
    // authority, which is what this redesign exists to stop.
    expect(
      opens("extensions", {
        roles: ["ops"],
        allow: { extension_access: ["read"] },
      }),
    ).toBe(true);
    expect(opens("extensions", { roles: ["rep"], allow: {} })).toBe(false);
  });

  it("opens system-health on the job-health grant", () => {
    // Same shape: the old entry was `isAdmin || embedding_reindex.read`, and
    // its own comment said nobody below ops had anything to read there — which
    // is the audience the grant already names.
    expect(
      opens("system-health", {
        roles: ["ops"],
        allow: { job_health: ["read"] },
      }),
    ).toBe(true);
    expect(opens("system-health", { roles: ["rep"], allow: {} })).toBe(false);
  });

  it("opens audit on audit_log alone, without person.read", () => {
    // privacy/auditlog.go requires exactly audit_log.read. The old entry folded
    // the audit trail in with the consent registry, so a reader could hold the
    // audit grant and still be refused the page carrying it.
    expect(
      opens("audit", { roles: ["admin"], allow: { audit_log: ["read"] } }),
    ).toBe(true);
    expect(
      opens("audit", { roles: ["rep"], allow: { person: ["read"] } }),
    ).toBe(false);
  });

  it("opens seats to management on seat_usage without license", () => {
    // The split shipped for this reader: capacity without commercial standing.
    // The old License entry asked for `license.read`, which management does not
    // hold, so the page management is meant to use was hidden from them.
    expect(
      opens("seats", {
        roles: ["management"],
        allow: { seat_usage: ["read"] },
      }),
    ).toBe(true);
    expect(opens("seats", { roles: ["rep"], allow: {} })).toBe(false);
  });

  it("opens authentication on its own grants", () => {
    // Split out of the old General entry, which asked for installation
    // settings, the company-context flag or FX rates — none of which is
    // authentication, and all of which a reader can lack while holding this.
    expect(
      opens("authentication", {
        roles: ["management"],
        allow: { authentication_policy: ["read"] },
      }),
    ).toBe(true);
    expect(
      opens("authentication", {
        roles: ["rep"],
        allow: { installation_settings: ["read"] },
      }),
    ).toBe(false);
  });

  it("splits the AI page by what each half actually reads", () => {
    // A narrowing, not a widening: management held `automation` and so reached
    // the whole combined page. Now they reach the parts their grants cover and
    // NOT the routing editor, which asks for ai_routing they do not hold.
    const management = {
      roles: ["management"],
      allow: { automation: ["read"], ai_diagnostics: ["read"] },
    } satisfies Parameters<typeof meFixture>[0];
    expect(opens("automations", management)).toBe(true);
    expect(opens("usage", management)).toBe(true);
    expect(opens("model-calls", management)).toBe(true);
    expect(opens("models", management)).toBe(false);
  });
});

describe("requirements that are not permissions — the composed units", () => {
  it("does not open integrations on a grant nobody in the old predicate had", () => {
    // `integrations.read` is held by every seeded role including read_only, so
    // admitting it would put the page in front of everyone. The old predicate
    // asked for overlay, webhook or a composed unit, and this keeps to that.
    expect(
      visibleSettingsPages(
        meFixture({ roles: ["rep"], allow: { integrations: ["read"] } }),
      ).some((p) => p.id === "integrations"),
    ).toBe(false);
  });

  it("opens integrations to an overlay or webhook reader", () => {
    // Spelled out rather than looped over a computed key: a computed key widens
    // `allow` to a string index and loses the object/action checking that makes
    // a misspelling here a compile error rather than a silently denied grant.
    expect(
      visibleSettingsPages(
        meFixture({ allow: { overlay_connection: ["read"] } }),
      ).some((p) => p.id === "integrations"),
    ).toBe(true);
    expect(
      visibleSettingsPages(
        meFixture({ allow: { webhook_subscription: ["read"] } }),
      ).some((p) => p.id === "integrations"),
    ).toBe(true);
  });
});

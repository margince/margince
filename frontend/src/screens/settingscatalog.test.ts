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

describe("the reset page needs the deployment's consent as well as the grant", () => {
  // Two conditions of different kinds, and the page needs BOTH.
  //
  // `system_reset:delete` says this reader may wipe an installation that permits
  // wiping. `data_reset_available` says whether this one does — the compiled
  // default is false in every posture, so most installations have no such
  // destination whoever is reading. Offering a reset the server would refuse is
  // worse here than anywhere else in settings.
  function opensReset(
    allow: NonNullable<Parameters<typeof meFixture>[0]>["allow"],
    armed: boolean,
  ) {
    const me = meFixture({ roles: ["admin"], allow });
    return visibleSettingsPages({ ...me, data_reset_available: armed }).some(
      (page) => page.id === "reset",
    );
  }

  it("opens when the grant and the flag are both there", () => {
    expect(opensReset({ system_reset: ["delete"] }, true)).toBe(true);
  });

  it("stays shut on an unarmed installation, whatever the grant says", () => {
    expect(opensReset({ system_reset: ["delete"] }, false)).toBe(false);
  });

  it("stays shut for a reader without the grant, however armed", () => {
    expect(opensReset({}, true)).toBe(false);
  });

  it("stays shut when /me carries no flag at all", () => {
    // The state a fixture cannot reach by accident: a server older than the
    // field, or a snapshot cached before it shipped. `meFixture` always supplies
    // it, so without this case the `?? false` can be flipped to `?? true` with
    // every other test still green — and what that ships is a wipe offered on an
    // installation that never consented to one.
    const me = meFixture({
      roles: ["admin"],
      allow: { system_reset: ["delete"] },
    });
    const { data_reset_available: _absent, ...withoutFlag } = me;
    expect(
      visibleSettingsPages(withoutFlag as typeof me).some(
        (p) => p.id === "reset",
      ),
    ).toBe(false);
  });

  it("is not opened by a READ of the same object", () => {
    // The verb is the whole gate here: `delete` is the only one that means
    // "may wipe this", and a read of system_reset means nothing at all.
    expect(opensReset({ system_reset: ["read"] }, true)).toBe(false);
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
// Each page opens on the grant its cards actually ask for, and the two moved
// together.
//
// The previous shape of this block held the opposite: pages narrowed to what
// their cards honoured, with every case written to FAIL when a card was
// rewritten. Those cases fired, which is what brought the change here — the
// gates below are their other half.
describe("a page and its cards ask the same question", () => {
  function opens(page: SettingsPageId, allow: Parameters<typeof meFixture>[0]) {
    return visibleSettingsPages(meFixture(allow)).some((p) => p.id === page);
  }

  it("opens extensions to an ops holder of both grants the card reads", () => {
    // The card is TWO reads behind one flag: the unit inventory from
    // `GET /v1/extensions` (extension_access) and every role's grant on every
    // object from `GET /v1/roles` (role_admin, identity/roles.go). Ops is
    // seeded both, which is why the page is ops's at all.
    expect(
      opens("extensions", {
        roles: ["ops"],
        allow: { extension_access: ["read"], role_admin: ["read"] },
      }),
    ).toBe(true);
    // The inventory grant ALONE opens a page whose card 403s on its second
    // request — an unreadable page, which is worse than a closed one.
    expect(
      opens("extensions", {
        roles: ["ops"],
        allow: { extension_access: ["read"] },
      }),
    ).toBe(false);
    expect(opens("extensions", { roles: ["rep"], allow: {} })).toBe(false);
  });

  it("opens system-health on either of its two cards' grants", () => {
    // A union that is really a union: the job report and the reindex are
    // different reads, and a holder of one finds the other card withheld.
    expect(
      opens("system-health", {
        roles: ["ops"],
        allow: { job_health: ["read"] },
      }),
    ).toBe(true);
    expect(
      opens("system-health", {
        roles: ["ops"],
        allow: { embedding_reindex: ["read"] },
      }),
    ).toBe(true);
    expect(opens("system-health", { roles: ["rep"], allow: {} })).toBe(false);
  });

  it("opens audit on audit_log, without needing the admin role", () => {
    expect(
      opens("audit", { roles: ["management"], allow: { audit_log: ["read"] } }),
    ).toBe(true);
    expect(
      opens("audit", { roles: ["rep"], allow: { person: ["read"] } }),
    ).toBe(false);
  });

  it("opens seats to a seat_usage holder, which is the reader the split shipped for", () => {
    // LicenseCard reads the entitlement for a `license` holder and falls back
    // to `/installation/seat-usage` for this one — capacity without commercial
    // standing, which is what management needs and may have.
    expect(
      opens("seats", {
        roles: ["management"],
        allow: { seat_usage: ["read"] },
      }),
    ).toBe(true);
    expect(
      opens("seats", { roles: ["admin"], allow: { license: ["read"] } }),
    ).toBe(true);
    expect(opens("seats", { roles: ["rep"], allow: {} })).toBe(false);
  });

  it("opens the AI diagnostics pages on ai_diagnostics", () => {
    // The three cards asked `automation:update` — a write verb guarding a GET,
    // from when the runtime's spend was operator information. The object is its
    // own now, so management reads what it spends without holding the
    // automation editor.
    const management = {
      roles: ["management"],
      allow: { ai_diagnostics: ["read"] },
    } satisfies Parameters<typeof meFixture>[0];
    expect(opens("usage", management)).toBe(true);
    expect(opens("model-calls", management)).toBe(true);
    // Models too, and NOT because the routing editor is theirs — it is not,
    // and the card refuses them. `AiHealthCard` reads on `ai_diagnostics` and
    // Models is the only page rendering it, so a page shut on the routing
    // grant alone would put a card behind a door its own reader cannot open.
    expect(opens("models", management)).toBe(true);
  });

  // The other half of that: the page opens for a diagnostics holder because of
  // ONE card, so the routing grant must still be what opens it for a routing
  // holder. A page requiring both would shut out the operator who came to edit
  // the bindings.
  it("opens models on either the routing grant or the diagnostics one", () => {
    expect(
      opens("models", { roles: ["ops"], allow: { ai_routing: ["read"] } }),
    ).toBe(true);
    expect(
      opens("models", { roles: ["rep"], allow: { automation: ["read"] } }),
    ).toBe(false);
  });

  // The purposes card is what `consent_config` administers, but the object
  // buys no READ — the list stays on `person` (consent/store.go ListPurposes)
  // and only the writes moved. A page opening on it would be a page whose every
  // card is withheld. Invisible in the seeded roles, where every consent holder
  // also holds `person:read`; a custom role is where it would have shown.
  it("does not open privacy on a grant that reads nothing on it", () => {
    expect(
      opens("privacy", {
        roles: ["custom"],
        allow: { consent_config: ["read", "create"] },
      }),
    ).toBe(false);
    // The grant that actually reads the purposes list does open it.
    expect(
      opens("privacy", { roles: ["rep"], allow: { person: ["read"] } }),
    ).toBe(true);
  });

  it("opens authentication on its own grant, and not on the one every role holds", () => {
    // `installation_settings:read` is held by every seeded role — a rep reads
    // it for the base currency — so a page opening on it would put the
    // installation's sign-in policy in front of the whole workspace.
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

  it("opens leads on custom_field, which is what its three cards read", () => {
    expect(
      opens("leads", { roles: ["rep"], allow: { custom_field: ["read"] } }),
    ).toBe(true);
    expect(
      opens("leads", { roles: ["rep"], allow: { pipeline: ["read"] } }),
    ).toBe(false);
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

import { describe, expect, it } from "vitest";
import type { RbacAction, RbacObject } from "../app/capability";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { translate } from "../i18n";
import {
  type CapabilityExpression,
  holds,
  readingIsTheAct,
  SETTINGS_GROUPS,
  SETTINGS_PAGES,
  type SettingsPageId,
  type SettingsScope,
  settingsReach,
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

// Every page declares whose state it changes, and the nav turns that into a
// label beside the heading (`scopeKey`, settingsnav.tsx). A page whose scope is
// wrong tells a reader a company-wide switch is theirs alone, which is the one
// mistake this field exists to prevent — so the values are checked here rather
// than only where they render.
describe("the scope each page declares", () => {
  it("gives every page a scope the catalogs can label", () => {
    for (const page of SETTINGS_PAGES) {
      expect(translate("en", `settings.scope.${page.scope}`)).toBeTruthy();
    }
  });

  // A CENSUS, not a sample. The earlier version of this suite named a few
  // pages per scope and let the rest ride, and four pages were wrong under it:
  // `integrations`, `models` and `privacy` each said "Company" over a card
  // writing the whole installation, and `extensions` said "Installation" over a
  // card writing one workspace's role grants. Every one of them passed.
  //
  // So every page is named here with the reason, and the two directions are
  // checked below: a new page with no entry fails, and an entry for a page that
  // no longer exists fails too. A census that can fail SHORT has already failed.
  //
  // The rule being applied: scope names whose state the page CHANGES, read off
  // the endpoints its cards write, not the heading the page sits under and not
  // what it merely reads. A read-only page describes what it shows.
  const DECLARED_SCOPE: Record<SettingsPageId, SettingsScope> = {
    // Wholly the reader's own.
    account: "self",
    voice: "self",
    agents: "self",

    // Pages whose cards genuinely split across two scopes. `integrations`
    // is one: PATCH /integrations/settings is "the installation's
    // provider-lookup posture" by its own contract summary, while webhooks,
    // the overlay mapping and the workspace extension units on the same page
    // stay inside one workspace.
    integrations: "mixed",

    // Personal pages carrying one shared surface each. A badge reading "Only
    // you" over either would tell a reader a company-wide switch is private to
    // them. ConnectorsCard's second panel connects the workspace's Telegram
    // bot; CaptureExclusionsCard's workspace rules keep a correspondent out of
    // the CRM for everybody.
    connections: "mixed",
    "capture-activity": "mixed",

    // The installation's own facts, by their contract summaries.
    company: "installation",
    authentication: "installation",
    seats: "installation",
    // PUT /ai/routing re-points which vendor processes the installation's text;
    // /ai/provider-keys writes the installation key vault.
    models: "installation",
    // Both retention endpoints are installation-wide. What this page destroys,
    // it destroys everywhere.
    privacy: "installation",
    // POST /embeddings/reindex rebuilds the installation's embed store.
    "system-health": "installation",
    reset: "installation",

    // One workspace's state.
    members: "workspace",
    teams: "workspace",
    pipelines: "workspace",
    stageautomation: "workspace",
    leads: "workspace",
    fields: "workspace",
    tags: "workspace",
    products: "workspace",
    capture: "workspace",
    knowledge: "workspace",
    import: "workspace",
    automations: "workspace",
    // Read-only pages: no write at all, so the scope describes what they SHOW.
    usage: "workspace",
    "model-calls": "workspace",
    audit: "workspace",
    // Reads the installation's unit inventory, but its only write is
    // PATCH /roles/{key}/objects/{object} — one workspace's role grants.
    extensions: "workspace",
  };

  it("declares the scope this census names, for every page", () => {
    for (const page of SETTINGS_PAGES) {
      expect(page.scope).toBe(DECLARED_SCOPE[page.id]);
    }
  });

  // The other direction. Without this, deleting a page leaves a stale entry
  // that nothing reads, and the census quietly describes a catalog that moved.
  it("names every page in the catalog and nothing else", () => {
    expect(Object.keys(DECLARED_SCOPE).sort()).toEqual(
      SETTINGS_PAGES.map((page) => page.id).sort(),
    );
  });
});

// `changes` decides PROMINENCE, not permission: the rail carries what a reader
// can act on, and everything else they may open stays on the settings home and
// in search. A wrong value here does not lock anybody out — it puts a page a
// reader cannot use in the furniture they navigate every day, or drops a page
// they work in out of it.
//
// A census, for the reason the scope census exists: naming three pages per
// value and letting twenty ride is how six wrong scopes shipped and four of
// them passed. Each entry says which card it was read off.
describe("what each page lets a reader change", () => {
  // `changes` decides PROMINENCE, not permission: the rail carries what a reader
  // can act on, and everything else they may open stays on the settings home and
  // in search. A wrong value does not lock anybody out — it puts a page a reader
  // cannot use in the furniture they navigate every day, or drops a page they
  // work in out of it.
  //
  // The census renders the WHOLE expression, not the object names in it. An
  // earlier version flattened to names and every one of these passed it: Import
  // asking create-OR-update where its card needs both, the missing delete verbs
  // on webhooks and overlays, Authentication missing the OAuth cards' own grant,
  // and the absent seat ceiling. Verbs, AND-versus-OR and the ceiling are
  // exactly where the defects were, so they are exactly what it has to compare.
  function render(expression: CapabilityExpression): string {
    switch (expression.kind) {
      case "always":
        return "always";
      case "seat":
        return "full-seat";
      case "reading-is-the-act":
        return "same-as-requires";
      case "grant":
        return `${expression.object}:${expression.action}`;
      case "availability":
        return `available:${expression.key}`;
      case "flag":
        return `flag:${expression.flag}`;
      case "units":
        return `units:${expression.scope}`;
      case "any":
        return `any(${expression.of.map(render).join(", ")})`;
      case "all":
        return `all(${expression.of.map(render).join(", ")})`;
    }
  }

  // Every page, with the reason its shape is what it is. `all(full-seat, ...)`
  // is a mutating page: the seat ceiling above the grants, exactly as
  // `useCanWrite` folds it for the controls themselves.
  const DECLARED_CHANGES: Record<SettingsPageId, string> = {
    // The reader's own rows, with no grant between them and the control.
    account: "always",
    agents: "always",
    connections: "always",
    "capture-activity": "always",
    // Under the reader's own heading and still a grant: voice-dna.tsx asks the
    // write, so a read-only seat consults this page.
    voice:
      "all(full-seat, any(any(voice_profile:update, voice_profile:create)))",

    company:
      "all(full-seat, any(any(installation_settings:update), all(any(organization:update, organization:create), available:company_context), any(fx_rate:update, fx_rate:create)))",
    // The OAuth cards save through `capture_settings`, a different grant from
    // the sign-in card's.
    authentication:
      "all(full-seat, any(any(installation_settings:update), any(capture_settings:update)))",

    members:
      "all(full-seat, any(any(user_admin:update, user_admin:create), user_admin:delete))",
    teams: "all(full-seat, any(any(team_admin:update, team_admin:create)))",
    seats: "same-as-requires",

    pipelines:
      "all(full-seat, any(any(pipeline:update, pipeline:create), pipeline:delete))",
    // The report is read-only, but the per-transition switches beneath it are
    // not: turning a transition on is a pipeline write, so a rep who may only
    // READ pipelines opens the page and finds every switch refused.
    stageautomation: "all(full-seat, any(any(pipeline:update)))",
    leads:
      "all(full-seat, any(any(custom_field:update, custom_field:create), custom_field:delete))",
    fields:
      "all(full-seat, any(any(custom_field:update, custom_field:create)))",
    tags: "all(full-seat, any(any(tag:update, tag:create), tag:delete))",
    products:
      "all(full-seat, any(any(product:update, product:create), product:delete, any(offer_template:update, offer_template:create), offer_template:delete))",

    capture:
      "all(full-seat, any(any(capture_settings:update), any(organization:update)))",
    // Delete included on both, and the composed-unit arm outside the ceiling:
    // ExtensionUnitsCard's Open link asks for no grant at all.
    integrations:
      "any(all(full-seat, any(any(integrations:update, integrations:create), integrations:delete, any(webhook_subscription:update, webhook_subscription:create), webhook_subscription:delete, any(overlay_connection:update, overlay_connection:create), overlay_connection:delete)), units:workspace)",
    knowledge: "all(full-seat, any(any(knowledge_corpus:create)))",
    // BOTH verbs: ImportCard's own gate is `mayCreate && mayAdvance`.
    import:
      "all(full-seat, any(all(any(import_run:create), any(import_run:update))))",

    models: "all(full-seat, any(any(ai_routing:update)))",
    automations:
      "all(full-seat, any(any(automation:update, automation:create), automation:delete))",
    usage:
      "all(full-seat, any(any(ai_model_rate:update, ai_model_rate:create)))",
    "model-calls": "same-as-requires",

    privacy:
      "all(full-seat, any(any(consent_config:create), any(retention_policy:update, retention_policy:create), retention_policy:delete, any(privacy_request:update), any(person:update)))",
    audit: "same-as-requires",
    // The reindex takes the seat; watching the queue beside it is a read, and
    // watching a stalled queue is an operator acting.
    "system-health":
      "any(all(full-seat, any(any(embedding_reindex:update))), job_health:read)",
    extensions: "all(full-seat, any(any(role_admin:update)))",
    reset: "all(full-seat, system_reset:delete, flag:data_reset_available)",
  };

  it("declares the expression this census names, for every page", () => {
    for (const page of SETTINGS_PAGES) {
      expect(`${page.id}: ${render(page.changes)}`).toBe(
        `${page.id}: ${DECLARED_CHANGES[page.id]}`,
      );
    }
  });

  // The other direction: a page added with no entry fails, and an entry for a
  // page that no longer exists fails too.
  it("names every page in the catalog and nothing else", () => {
    expect(Object.keys(DECLARED_CHANGES).sort()).toEqual(
      SETTINGS_PAGES.map((page) => page.id).sort(),
    );
  });

  // Every mutating page folds the seat, and no page's `requires` does. The
  // second half matters as much: `requires` decides who may OPEN a page, and a
  // read seat may open everything its grants open — folding the ceiling there
  // would be a permission change wearing the clothes of a tidier rail.
  it("puts the seat ceiling on what a page changes and never on what opens it", () => {
    for (const page of SETTINGS_PAGES) {
      expect(render(page.requires)).not.toContain("full-seat");
    }
    const mutating = SETTINGS_PAGES.filter(
      (page) =>
        page.changes.kind !== "always" &&
        page.changes.kind !== "reading-is-the-act",
    );
    for (const page of mutating) {
      expect(`${page.id}: ${render(page.changes)}`).toContain("full-seat");
    }
  });

  // The sentinel is only ever a `changes`, and `holds` must refuse it rather
  // than guess: it names the page's own requirement, which no expression can
  // carry. Anything evaluating it directly has skipped `settingsReach`.
  it("refuses to resolve the reading-is-the-act sentinel on its own", () => {
    expect(holds(readingIsTheAct, meFixture({ roles: ["admin"] }))).toBe(false);
  });

  // A page whose `requires` is a mutation must not use the sentinel: the
  // sentinel resolves to `requires`, which carries no seat ceiling, so a read
  // seat holding the grant would be told the page is theirs to work in.
  // Automations and Reset were both written that way and both were wrong.
  it("never resolves the sentinel to a requirement that mutates", () => {
    for (const page of SETTINGS_PAGES) {
      if (page.changes.kind !== "reading-is-the-act") {
        continue;
      }
      expect(`${page.id}: ${render(page.requires)}`).not.toMatch(
        /:(create|update|delete)/,
      );
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

  // A reader holding exactly one object, at exactly one verb. `readOn` in the
  // testkit carries a floor of its own, which is right for a case about one
  // page and wrong for a case about one GRANT — the floor would supply the
  // very read under test.
  //
  // Built by assignment rather than as an object literal with a computed key:
  // a computed key widens the spec to `{ [x: string]: string[] }`, which does
  // not satisfy GrantSpec and only fails in `tsc -b` — where test files are
  // typechecked — rather than under vitest, which transpiles without checking.
  function grantOf(object: RbacObject, actions: RbacAction[]) {
    const allow: GrantSpec = {};
    allow[object] = actions;
    return meFixture({ roles: ["rep"], allow });
  }
  const readsOnly = (object: RbacObject) => grantOf(object, ["read"]);
  const writes = (object: RbacObject) =>
    grantOf(object, ["read", "create", "update"]);

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
    ]);
  });

  // Members and Teams used to be on that list. `GET /users` still answers any
  // authenticated caller and must — the share and assignee pickers read it —
  // but a directory is not an administration page, and a reader who may not
  // invite, change a role or switch a seat off has nothing to do on either.
  it("withholds members from a reader holding no user_admin", () => {
    expect(visibleIds(readsOnly("person"))).not.toContain("members");
    // Any authority over the roster opens it, and the READ is one of them —
    // it is what carries the role keys and the widened status view.
    expect(visibleIds(readsOnly("user_admin"))).toContain("members");
  });

  it("withholds teams from a reader holding neither team verb nor the roster read", () => {
    expect(visibleIds(readsOnly("person"))).not.toContain("teams");
    // The team object's READ is not one of its arms: teams.go takes create and
    // update, and nothing on the page answers to a `team_admin:read`.
    expect(visibleIds(readsOnly("team_admin"))).not.toContain("teams");
    expect(visibleIds(writes("team_admin"))).toContain("teams");
  });

  // The roster's privileged projection is what Teams RENDERS: `team_ids` rides
  // `user_admin:read` (handlers_roster.go), not the team object. So a reader
  // holding that read alone still opens Teams — they see who is in which team
  // and may change none of it — and the checkboxes are disabled rather than
  // absent, because the list IS the answer they came for.
  it("opens teams to the roster read that carries membership", () => {
    expect(visibleIds(readsOnly("user_admin"))).toContain("teams");
  });

  // The seat switch WITHOUT the read does not open it, and that is the fix
  // rather than an omission. `include_inactive` is honoured only for a caller
  // who passes `user_admin:read` (handlers_roster.go), so this holder would
  // reach a roster that never shows them a deactivated member to reactivate —
  // a page offering two affordances that cannot work.
  it("does not open members for the deactivate verb without the roster read", () => {
    const seatSwitch = meFixture({
      roles: ["custom"],
      allow: { user_admin: ["delete"] },
    });
    expect(visibleSettingsPages(seatSwitch).map((p) => p.id)).not.toContain(
      "members",
    );
    // With the read beside it, the same holder gets the page.
    expect(
      visibleSettingsPages(
        meFixture({
          roles: ["custom"],
          allow: { user_admin: ["read", "delete"] },
        }),
      ).map((p) => p.id),
    ).toContain("members");
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

  // The four pages whose subject is the installation's own configuration. Each
  // was reachable by a rep because the grant that opened it is a READ every
  // seeded role holds — the base currency, what an automation ran, whether
  // capture is working, the person record behind the purposes list.
  //
  // Every absence below is paired with the presence that proves the case is not
  // vacuous: an authority that refused everyone would pass the first half
  // alone, and that is exactly how a permission test goes green while saying
  // nothing.
  it.each([
    ["company", "installation_settings"],
    ["integrations", "overlay_connection"],
    ["automations", "automation"],
  ] as const)(
    "withholds %s from a rep who only reads %s, and opens it to its writer",
    (page, object) => {
      expect(visibleIds(readsOnly(object))).not.toContain(page);
      expect(visibleIds(writes(object))).toContain(page);
    },
  );

  // Privacy is the fourth page but not the same shape: its arms stay READS,
  // because `retention_policy` and `privacy_request` are held by nobody below
  // admin and ops — the read already says whose page it is. What was wrong was
  // the third arm, `person:read`, which every seeded role holds and which is
  // why a rep opened the governance page at all.
  //
  // The purposes card still reads through `person` server-side and must keep
  // doing so; it feeds the Person 360. A card narrower than its page withholds
  // itself, which is the safe direction.
  // Management is seeded `consent_config:read` and NOTHING else on this page —
  // no retention, no request queue. Dropping the `person` arm without this pair
  // locked the one role deliberately granted the consent vocabulary out of the
  // only page that renders it. The pair is what keeps them in without letting a
  // rep back: a rep holds `person` and no consent grant at all.
  it("opens privacy to the consent vocabulary's own reader, and to nobody else holding person", () => {
    const management = meFixture({
      roles: ["management"],
      allow: { person: ["read"], consent_config: ["read"] },
    });
    expect(visibleSettingsPages(management).map((page) => page.id)).toContain(
      "privacy",
    );
    // The same reader without the consent grant is a rep, and stays out.
    expect(visibleIds(readsOnly("person"))).not.toContain("privacy");
    // And the consent grant alone does not do it either: the purposes list is
    // read through `person`, so a holder without that read would open a page
    // whose only card is withheld.
    expect(visibleIds(readsOnly("consent_config"))).not.toContain("privacy");
  });

  it("withholds privacy from a rep holding person, and opens it to a retention reader", () => {
    expect(visibleIds(readsOnly("person"))).not.toContain("privacy");
    expect(visibleIds(readsOnly("retention_policy"))).toContain("privacy");
    expect(visibleIds(readsOnly("privacy_request"))).toContain("privacy");
  });

  // Management and manager read `automation` — they see what ran, on the
  // records it touched. Neither may change one, and the page that DEFINES
  // automations is therefore not theirs. This is the deliberate half of the
  // narrowing: it is not only reps who lose a page here.
  it("withholds automations from management, which reads automation but cannot write", () => {
    expect(
      visibleIds(
        meFixture({ roles: ["management"], allow: { automation: ["read"] } }),
      ),
    ).not.toContain("automations");
  });
});

describe("requirements that are not permissions", () => {
  it("withholds the company page when the installation lacks the surface", () => {
    // organization.update alone. The company profile ANDs its grant with a
    // deployment flag, so a reader holding only that grant sees nothing when
    // the flag is off — the surface may genuinely not exist here.
    const holder = meFixture({
      allow: { organization: ["read", "update"] },
      settingsAvailability: { company_context: false },
    });
    expect(visibleSettingsPages(holder).map((p) => p.id)).not.toContain(
      "company",
    );
  });

  it("shows it once the installation has it", () => {
    const holder = meFixture({
      allow: { organization: ["read", "update"] },
      settingsAvailability: { company_context: true },
    });
    expect(visibleSettingsPages(holder).map((p) => p.id)).toContain("company");
  });

  it("withholds it when /me carries no availability at all", () => {
    // A server older than the field, or a snapshot cached before it shipped.
    // Absent is not permission: it has to read as "no such surface here".
    const holder = meFixture({
      allow: { organization: ["read", "update"] },
      settingsAvailability: null,
    });
    expect(visibleSettingsPages(holder).map((p) => p.id)).not.toContain(
      "company",
    );
  });

  it("still shows it to a reader whose OTHER grant carries the page", () => {
    // The flag gates one card, not the page: installation_settings.update opens
    // the company page regardless, and treating the flag as a page-level
    // condition would hide a surface the reader may use.
    const admin = meFixture({
      allow: { installation_settings: ["read", "update"] },
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
    // Nor on `person`, which every seeded role holds: the purposes card reads
    // through it, but a page that opened on it was the whole workspace's
    // governance page. The arms that DO open it are the two objects nobody
    // below admin and ops holds at all.
    expect(
      opens("privacy", { roles: ["rep"], allow: { person: ["read"] } }),
    ).toBe(false);
    expect(
      opens("privacy", {
        roles: ["custom"],
        allow: { retention_policy: ["read"] },
      }),
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

  it("opens integrations to whoever may CONNECT one, and not to the readers", () => {
    // Spelled out rather than looped over a computed key: a computed key widens
    // `allow` to a string index and loses the object/action checking that makes
    // a misspelling here a compile error rather than a silently denied grant.
    //
    // The read is the same everyone-holds-it grant as `integrations` above —
    // every seeded role reads both, because "is capture working?" shows up on
    // the records they already open. Connecting an overlay is admin and ops work.
    expect(
      visibleSettingsPages(
        meFixture({ allow: { overlay_connection: ["read"] } }),
      ).some((p) => p.id === "integrations"),
    ).toBe(false);
    expect(
      visibleSettingsPages(
        meFixture({ allow: { overlay_connection: ["read", "update"] } }),
      ).some((p) => p.id === "integrations"),
    ).toBe(true);
    expect(
      visibleSettingsPages(
        meFixture({ allow: { webhook_subscription: ["read"] } }),
      ).some((p) => p.id === "integrations"),
    ).toBe(false);
    expect(
      visibleSettingsPages(
        meFixture({ allow: { webhook_subscription: ["read", "create"] } }),
      ).some((p) => p.id === "integrations"),
    ).toBe(true);
  });
});

// What a page's `changes` buys a real reader: which pages reach the rail, and
// which stay reachable from the settings home and search without cluttering it.
//
// Built on the SEEDED rep's grants rather than a hand-picked few. A partial
// fixture would answer a question about a fixture — the point of this case is
// what the product actually shows the role Lars ships, so a policy change that
// widens a rep shows up here as a failing list rather than silently.
describe("what the rail carries and what it leaves behind", () => {
  const seededRep = meFixture({
    roles: ["rep"],
    allow: {
      activity: ["create", "read", "update"],
      automation: ["read"],
      capture_settings: ["create", "read"],
      channel_connection: ["read"],
      commission: ["read"],
      computed_field: ["read"],
      contract: ["create", "read", "update"],
      custom_field: ["read"],
      deal: ["create", "read", "update"],
      deal_room: ["create", "read", "update"],
      finance: ["read"],
      forecast: ["read"],
      installation_settings: ["read"],
      integrations: ["read"],
      introduction: ["create", "read", "update"],
      knowledge_corpus: ["read"],
      knowledge_document: ["read"],
      lead: ["create", "read", "update", "delete"],
      list: ["create", "read", "update"],
      offer: ["create", "read", "update"],
      offer_template: ["create", "read", "update"],
      organization: ["create", "read", "update"],
      overlay_connection: ["read"],
      partner: ["read"],
      person: ["create", "read", "update"],
      pipeline: ["read"],
      product: ["create", "read", "update"],
      project: ["create", "read", "update"],
      relationship: ["create", "read", "update"],
      saved_view: ["create", "read", "update", "delete"],
      signal: ["create", "read", "update"],
      tag: ["read"],
      voice_profile: ["create", "read", "update"],
      webhook_subscription: ["read"],
      weekly_plan: ["create", "read", "update"],
    },
  });

  it("gives a rep the pages they work in, and only those", () => {
    const reach = settingsReach(seededRep);
    expect(reach.acts.map((page) => page.id)).toEqual([
      // Their own five, minus Voice — a rep holds voice_profile create and
      // update, so Voice IS theirs; it is here for that reason and not because
      // the page sits under their own heading.
      "account",
      "voice",
      "agents",
      "connections",
      "capture-activity",
      // The company profile the AI reads: a rep holds `organization` create and
      // update, which is what CompanyContextCard asks. Existing behaviour that
      // the rail is only now reporting — the card was always editable by them.
      "company",
      // Products and offer templates: a rep authors both.
      "products",
      // Capture rules, because a rep holds `organization:update` and
      // BlockedDomainsCard writes it. If a rep editing the company's blocked
      // domains is not wanted, that CARD's grant is the thing to change — the
      // rail is only reporting what the card already allows.
      "capture",
    ]);
  });

  it("leaves the pages a rep can only read out of the rail, not out of reach", () => {
    const reach = settingsReach(seededRep);
    expect(reach.looksUp.map((page) => page.id)).toEqual([
      "pipelines",
      // Stage automation MOVED here when the page grew its switches. A rep
      // holds pipeline read and not update, so the report is still theirs to
      // consult and every control on it is refused — which is what looksUp
      // means. It sat in the acted-on half while the page was read-only.
      "stageautomation",
      "leads",
      "fields",
      "tags",
      "knowledge",
    ]);
    // Integrations and Automations are absent from BOTH halves, and that is
    // #4650's doing rather than this change's: their `requires` asks the write,
    // so a rep never opens them at all. A page has to be visible before the
    // partition has anything to say about it.
    // Still openable, every one of them: the partition decides prominence, and
    // a page in `looksUp` answers its address, appears in search and is listed
    // on the settings home. Losing that distinction would turn a tidier rail
    // into a permission change nobody asked for.
    const openable = visibleSettingsPages(seededRep).map((page) => page.id);
    for (const page of reach.looksUp) {
      expect(openable).toContain(page.id);
    }
  });

  // The partition is exhaustive and disjoint — every visible page in exactly
  // one half. Without this a page could fall out of both and vanish from the
  // rail AND the home while still answering its address, which is the one
  // shape of this bug nobody would report.
  it("puts every page a reader may open in exactly one half", () => {
    for (const snapshot of [seededRep, meFixture({ roles: ["admin"] })]) {
      const reach = settingsReach(snapshot);
      const partitioned = [...reach.acts, ...reach.looksUp]
        .map((page) => page.id)
        .sort();
      expect(partitioned).toEqual(
        visibleSettingsPages(snapshot)
          .map((page) => page.id)
          .sort(),
      );
      expect(new Set(partitioned).size).toBe(partitioned.length);
    }
  });
});

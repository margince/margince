import { describe, expect, it } from "vitest";
import { SETTINGS_PAGES, type SettingsPageId } from "./settingscatalog";
import {
  SETTINGS_SCREEN,
  settingsHref,
  settingsRouteTarget,
} from "./settingsrouting";

describe("every page round-trips through its own address", () => {
  it.each(SETTINGS_PAGES.map((page) => page.id))("%s", (id) => {
    // The property the whole module exists for: what a link mints is what an
    // address resolves to. A page that failed this would be reachable from the
    // rail and dead from a bookmark, or the reverse.
    const target = settingsRouteTarget(settingsHref(id));
    expect(target).toEqual({ kind: "page", page: id, legacy: false });
  });

  it("mints a flat address with no group segment", () => {
    // The old shape put `admin/` in front of half the pages, which made a
    // page's address depend on which group it sat in — so moving a page
    // between groups broke every bookmark under it.
    expect(settingsHref("audit")).toEqual({
      screen: SETTINGS_SCREEN,
      id: "audit",
    });
  });
});

describe("the ten ids that did not change are not aliases", () => {
  // The classification an earlier draft of this got backwards. These survived
  // the regroup as canonical page ids; resolving them as legacy would rewrite
  // the address bar on arrival for every one of them, for nothing.
  const unchanged = [
    "account",
    "voice",
    "agents",
    "connections",
    "capture-activity",
    "privacy",
    "capture",
    "integrations",
    "knowledge",
    "extensions",
  ] satisfies SettingsPageId[];

  it.each(unchanged)("%s resolves as current, not legacy", (id) => {
    expect(settingsRouteTarget({ screen: SETTINGS_SCREEN, id })).toEqual({
      kind: "page",
      page: id,
      legacy: false,
    });
  });
});

describe("the six ids that did change resolve and say so", () => {
  it.each([
    ["general", "company"],
    ["users", "members"],
    ["people", "members"],
    ["data-model", "fields"],
    ["ai", "models"],
    ["maintenance", "system-health"],
    ["license", "seats"],
  ] as const)("%s → %s", (old, now) => {
    expect(settingsRouteTarget({ screen: SETTINGS_SCREEN, id: old })).toEqual({
      kind: "page",
      page: now,
      legacy: true,
    });
  });
});

describe("the retired admin/ segment", () => {
  it("still answers an address the product actually minted", () => {
    // A bookmark, a pasted link, the handbook. Resolving it as legacy is what
    // rewrites the address bar to the current spelling rather than leaving two
    // in circulation.
    expect(
      settingsRouteTarget({
        screen: SETTINGS_SCREEN,
        id: "admin",
        id2: "privacy",
      }),
    ).toEqual({ kind: "page", page: "privacy", legacy: true });
  });

  it("carries a renamed id through to its new page", () => {
    expect(
      settingsRouteTarget({
        screen: SETTINGS_SCREEN,
        id: "admin",
        id2: "general",
      }),
    ).toEqual({ kind: "page", page: "company", legacy: true });
  });

  it("refuses an address the product never minted", () => {
    // `voice` was never an admin entry, so `#/settings/admin/voice` is
    // somebody's invention. Answering it would put a second spelling of a
    // personal page into circulation, which nothing would ever rewrite.
    expect(
      settingsRouteTarget({
        screen: SETTINGS_SCREEN,
        id: "admin",
        id2: "voice",
      }),
    ).toEqual({ kind: "unknown" });
  });

  it("refuses the bare segment", () => {
    expect(
      settingsRouteTarget({ screen: SETTINGS_SCREEN, id: "admin" }),
    ).toEqual({
      kind: "unknown",
    });
  });
});

describe("home and nowhere are different answers", () => {
  it("reads a bare settings address as home", () => {
    expect(settingsRouteTarget({ screen: SETTINGS_SCREEN })).toEqual({
      kind: "home",
    });
  });

  it("reads an id nothing answers as unknown, not as home", () => {
    // Distinct on purpose. An address nobody minted should say so rather than
    // quietly landing somewhere — which is how a typo in a shared link becomes
    // a page the sender never meant to send.
    expect(
      settingsRouteTarget({ screen: SETTINGS_SCREEN, id: "nonesuch" }),
    ).toEqual({ kind: "unknown" });
  });
});

describe("id2 is page-local state, not a sub-page", () => {
  it("resolves the OAuth callback to the connections page itself", () => {
    // `#/settings/connections/{outcome}` is what a mailbox consent redirect
    // comes back to, and connectors.tsx reads the outcome off id2. If this
    // resolved to `unknown` — or rewrote the address as legacy and dropped the
    // segment — a reader would finish an OAuth flow and land nowhere, or land
    // on a page that no longer knows whether their mailbox connected.
    for (const outcome of ["ok", "denied", "error"]) {
      expect(
        settingsRouteTarget({
          screen: SETTINGS_SCREEN,
          id: "connections",
          id2: outcome,
        }),
      ).toEqual({ kind: "page", page: "connections", legacy: false });
    }
  });

  it("does not treat id2 as deepening any other page", () => {
    // The same rule everywhere, so no page grows a second address shape by
    // accident: a trailing segment never changes WHICH page an address names.
    expect(
      settingsRouteTarget({
        screen: SETTINGS_SCREEN,
        id: "audit",
        id2: "anything",
      }),
    ).toEqual({ kind: "page", page: "audit", legacy: false });
  });
});

describe("every address the product used to mint still resolves", () => {
  // The census this file could get wrong by falling SHORT: an old admin id
  // missing from the legacy list is a bookmark that resolves to `unknown`, and
  // nothing else in this suite would notice — every other case names an id it
  // already knows about.
  //
  // Written out from the shipped register rather than derived, because the
  // register is what this change deletes: deriving it from the new catalog
  // would compare the change against itself.
  const everyOldAdminId = [
    "general",
    "users",
    "people",
    "integrations",
    "extensions",
    "capture",
    "data-model",
    "ai",
    "knowledge",
    "privacy",
    "license",
    "maintenance",
  ];

  it.each(everyOldAdminId)("#/settings/admin/%s lands somewhere", (id) => {
    const target = settingsRouteTarget({
      screen: SETTINGS_SCREEN,
      id: "admin",
      id2: id,
    });
    expect(target.kind).toBe("page");
    // Always legacy under this segment: the segment itself is the old spelling,
    // so arriving through it rewrites the address bar even when the page id
    // after it never changed.
    expect(target).toMatchObject({ legacy: true });
  });

  it.each(everyOldAdminId)(
    "the bare #/settings/%s lands somewhere too",
    (id) => {
      // Both shapes were in circulation — the register minted the admin one, but
      // a reader could type either, and the shallow form is what the six renamed
      // ids answer under.
      expect(settingsRouteTarget({ screen: SETTINGS_SCREEN, id }).kind).toBe(
        "page",
      );
    },
  );
});

describe("an address off the bar is attacker-typed, not a key", () => {
  // The alias map is looked up with a string straight off the address bar. On
  // an ordinary object `constructor` and `toString` are INHERITED properties,
  // so those lookups return a function rather than undefined and the address
  // resolves as a renamed page whose `page` is that function — a crash or a
  // malformed redirect wherever the target is consumed.
  //
  // None of the seventy-nine cases above could see it: every one names an id
  // that either is a page or plainly is not.
  it.each([
    "constructor",
    "toString",
    "__proto__",
    "valueOf",
    "hasOwnProperty",
  ])("%s resolves to unknown", (id) => {
    expect(settingsRouteTarget({ screen: SETTINGS_SCREEN, id })).toEqual({
      kind: "unknown",
    });
  });

  it("refuses them under the legacy admin segment too", () => {
    // The other lookup, which takes the same input a segment deeper.
    expect(
      settingsRouteTarget({
        screen: SETTINGS_SCREEN,
        id: "admin",
        id2: "constructor",
      }),
    ).toEqual({ kind: "unknown" });
  });
});

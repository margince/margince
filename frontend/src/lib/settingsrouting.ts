// Settings addresses: what a link mints, and what an address resolves to.
//
// Flat — `#/settings/<page>`, with `#/settings` the home. The old shape put an
// `admin/` segment in front of half the pages, which encoded the two-audience
// model in the URL itself: the segment was a property of who a page was FOR,
// so renaming a group would have moved every bookmark under it.
//
// Pure, like the catalog it reads. The rail, the palette, the redirect and the
// page all mint links through here, so a spelling nothing mints cannot get into
// circulation.

import type { Route } from "./router";
import {
  SETTINGS_PAGES,
  type SettingsPageId,
  type SettingsPageRow,
} from "./settingscatalog";

/** The screen this module addresses, named once. */
export const SETTINGS_SCREEN = "settings";

/** Where an old address now lands: a page, and the tab that holds its part. */
type RenamedPage = Readonly<{ page: SettingsPageId; tab?: string }>;

/**
 * Old ids that are GENUINELY renamed or merged, and where each now lands.
 *
 * Most old ids are still CANONICAL page ids (`account`, `privacy`, `capture`
 * and the rest). They are not aliases and must not resolve as legacy, or each
 * would rewrite the address bar on arrival for no reason.
 *
 * The rest lost their names. A page merged into another lands on the tab that
 * holds what it was; a split one lands on the part its id was named for.
 */
const RENAMED_PAGES: Readonly<Record<string, RenamedPage>> = Object.assign(
  // Prototype-free, because the lookups below take an id straight off the
  // address bar: on an ordinary object `#/settings/constructor` would resolve
  // to an inherited function rather than to undefined.
  Object.create(null) as Record<string, RenamedPage>,
  {
    general: { page: "company" },
    users: { page: "members" },
    contacts: { page: "members" },
    "data-model": { page: "records" },
    ai: { page: "models" },
    maintenance: { page: "system-health" },
    license: { page: "seats" },
    "capture-activity": { page: "connections", tab: "activity" },
    pipelines: { page: "deals" },
    acquisition: { page: "deals", tab: "sources" },
    lossreasons: { page: "deals", tab: "loss" },
    reviewtemplates: { page: "deals", tab: "reviews" },
    fields: { page: "records" },
    tags: { page: "records", tab: "tags" },
    recordroles: { page: "records", tab: "roles" },
  },
);

/**
 * The segment the old two-audience shape put in front of the admin pages.
 *
 * Kept only so addresses the product actually minted keep working. A reader
 * following `#/settings/admin/privacy` from a bookmark lands on the privacy
 * page and the address bar is rewritten; an invented `#/settings/admin/voice`
 * stays unknown, because the product never minted it and answering it would put
 * a second spelling of a personal page into circulation.
 */
const LEGACY_ADMIN_SEGMENT = "admin";

/** The ids that were reachable under `admin/`, so an invented one stays unknown. */
const LEGACY_ADMIN_IDS: readonly string[] = [
  "general",
  "users",
  "contacts",
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

export const IMPORT_RUN_SUBPAGE = "run";
export const IMPORT_TEMPLATE_SUBPAGE = "template";

// A page listed here answers only these segments under it, one level deep; any
// other is unknown. A page with tabs answers its tab ids the same way.
const SUBPAGES: Readonly<Partial<Record<SettingsPageId, readonly string[]>>> = {
  import: [IMPORT_RUN_SUBPAGE, IMPORT_TEMPLATE_SUBPAGE],
};

// The OAuth return lands on `#/settings/<page>/{outcome}/{provider}`, so these
// two pages also answer segments that are not tabs, on their first tab.
const OAUTH_LANDINGS: readonly SettingsPageId[] = ["connections", "meetings"];

function tabIds(page: SettingsPageId): readonly string[] {
  const row: SettingsPageRow | undefined = SETTINGS_PAGES.find(
    (each) => each.id === page,
  );
  return row?.tabs?.map((tab) => tab.id) ?? [];
}

/** The tab a route names, `undefined` for the page's first, or `null` for none. */
function routeTab(
  page: SettingsPageId,
  route: Route,
): string | undefined | null {
  if (route.id2 === undefined) return undefined;
  const tabs = tabIds(page);
  if (route.id3 === undefined && tabs.includes(route.id2)) return route.id2;
  if (OAUTH_LANDINGS.includes(page)) return undefined;
  const subpages = SUBPAGES[page];
  if (subpages !== undefined) {
    return route.id3 === undefined && subpages.includes(route.id2)
      ? undefined
      : null;
  }
  return tabs.length > 0 ? null : undefined;
}

/** What an address resolves to. */
export type SettingsTarget =
  | { readonly kind: "home" }
  | {
      readonly kind: "page";
      readonly page: SettingsPageId;
      /** The tab the address names; absent means the page's first. */
      readonly tab?: string;
      /** True when the address the reader used is not the current spelling. */
      readonly legacy: boolean;
    }
  | { readonly kind: "unknown" };

function pageFor(id: string | undefined): SettingsPageId | undefined {
  return SETTINGS_PAGES.find((page) => page.id === id)?.id;
}

/**
 * Resolve one route to a settings destination.
 *
 * `unknown` is a real answer and distinct from `home`: an address nobody minted
 * should say so rather than quietly landing somewhere, which is how a typo in a
 * shared link becomes a page the sender never meant to send.
 */
export function settingsRouteTarget(route: Route): SettingsTarget {
  if (route.id === undefined) {
    return { kind: "home" };
  }

  if (route.id === LEGACY_ADMIN_SEGMENT) {
    if (route.id2 === undefined || !LEGACY_ADMIN_IDS.includes(route.id2)) {
      return { kind: "unknown" };
    }
    const renamed = RENAMED_PAGES[route.id2];
    if (renamed !== undefined) {
      return { kind: "page", ...renamed, legacy: true };
    }
    const current = pageFor(route.id2);
    // Every id in LEGACY_ADMIN_IDS is either renamed above or still a page, so
    // this cannot miss; resolving it explicitly keeps the two lists independent.
    return current === undefined
      ? { kind: "unknown" }
      : { kind: "page", page: current, legacy: true };
  }

  const renamed = RENAMED_PAGES[route.id];
  if (renamed !== undefined) {
    return { kind: "page", ...renamed, legacy: true };
  }

  const current = pageFor(route.id);
  if (current === undefined) return { kind: "unknown" };
  const tab = routeTab(current, route);
  if (tab === null) return { kind: "unknown" };
  return tab === undefined
    ? { kind: "page", page: current, legacy: false }
    : { kind: "page", page: current, tab, legacy: false };
}

/**
 * The address a settings page lives at.
 *
 * Every caller that mints a settings link goes through this. Flat, so a page's
 * address does not depend on which group it sits in — moving a page between
 * groups is then a catalog edit and not a broken bookmark.
 */
export function settingsHref(page?: SettingsPageId, tab?: string): Route {
  return tab === undefined
    ? { screen: SETTINGS_SCREEN, id: page }
    : { screen: SETTINGS_SCREEN, id: page, id2: tab };
}

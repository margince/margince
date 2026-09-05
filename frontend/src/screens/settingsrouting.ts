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

import type { Route } from "../app/router";
import { SETTINGS_PAGES, type SettingsPageId } from "./settingscatalog";

/** The screen this module addresses, named once. */
export const SETTINGS_SCREEN = "settings";

/**
 * Old ids that are GENUINELY renamed, and the page each now names.
 *
 * The distinction this map turns on is easy to get wrong in the other
 * direction, so it is worth stating: ten of the sixteen old ids are still
 * CANONICAL page ids — `account`, `voice`, `agents`, `connections`,
 * `capture-activity`, `privacy`, `capture`, `integrations`, `knowledge`,
 * `extensions`. They are not aliases and must not resolve as legacy, or every
 * one of them would rewrite the address bar on arrival for no reason.
 *
 * Only these six lost their names. Each points at the page carrying the part a
 * reader following an old link was most likely after — `general` had the
 * company profile and the sign-in policy, and the profile is what the id was
 * named for.
 */
const RENAMED_PAGES: Readonly<Record<string, SettingsPageId>> = Object.assign(
  // Prototype-free, because the lookups below take an id straight off the
  // address bar. On an ordinary object `#/settings/constructor` and
  // `#/settings/toString` resolve to an INHERITED function rather than to
  // undefined, so the address would be reported as a renamed page whose `page`
  // is a function — a crash or a malformed redirect wherever this is consumed.
  Object.create(null) as Record<string, SettingsPageId>,
  {
    general: "company",
    users: "members",
    people: "members",
    "data-model": "fields",
    ai: "models",
    maintenance: "system-health",
    license: "seats",
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

/** What an address resolves to. */
export type SettingsTarget =
  | { readonly kind: "home" }
  | {
      readonly kind: "page";
      readonly page: SettingsPageId;
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
      return { kind: "page", page: renamed, legacy: true };
    }
    const current = pageFor(route.id2);
    // Every id in LEGACY_ADMIN_IDS is either renamed above or still a page, so
    // this cannot miss — but resolving it explicitly keeps the list and the
    // catalog independent, so adding an id to one without the other is an
    // unknown address rather than a crash.
    return current === undefined
      ? { kind: "unknown" }
      : { kind: "page", page: current, legacy: true };
  }

  const renamed = RENAMED_PAGES[route.id];
  if (renamed !== undefined) {
    return { kind: "page", page: renamed, legacy: true };
  }

  const current = pageFor(route.id);
  return current === undefined
    ? { kind: "unknown" }
    : { kind: "page", page: current, legacy: false };
}

/**
 * The address a settings page lives at.
 *
 * Every caller that mints a settings link goes through this. Flat, so a page's
 * address does not depend on which group it sits in — moving a page between
 * groups is then a catalog edit and not a broken bookmark.
 */
export function settingsHref(page?: SettingsPageId): Route {
  return { screen: SETTINGS_SCREEN, id: page };
}

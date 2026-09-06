// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";
import { settingsHref } from "../screens/settingsrouting";
import { ENTITY, type EntityKind, isEntityKind } from "./entity";
import type { Route } from "./router";

// What a search hit IS to the two surfaces that draw one — the ⌘K palette and
// the results screen — spelled once.
//
// Both used to answer this for themselves, and the answers had already drifted:
// each carried its own set of "which types are linkable", each special-cased a
// tag, and the results screen's group list quietly omitted `project` so a hit
// the server returned was dropped on the floor. Two readings of one vocabulary
// is one reading too many, and the second one is always the stale one.
export type SearchHitType = NonNullable<
  components["schemas"]["SearchResult"]["type"]
>;

// The display order the results screen groups by, and the tie-break the palette
// falls back on. Records first, most-asked-for first; a tag last because it is a
// WORD, and somebody who typed a name is usually after the records rather than
// the label they were filed under.
export const SEARCH_HIT_ORDER = [
  "person",
  "organization",
  "deal",
  "project",
  "product",
  "offer_template",
  "activity",
  "lead",
  "tag",
] as const satisfies readonly SearchHitType[];

// The heading a group of these hits carries. One key per member of the contract
// enum, so a type the server learns to return cannot reach the screen without a
// name to file it under — the failure that dropped project hits.
export const SEARCH_HIT_GROUP_KEY: Readonly<Record<SearchHitType, MessageKey>> =
  {
    person: "search.group.person",
    organization: "search.group.organization",
    deal: "search.group.deal",
    project: "search.group.project",
    product: "search.group.product",
    offer_template: "search.group.offerTemplate",
    activity: "search.group.activity",
    lead: "search.group.lead",
    tag: "search.group.tag",
  };

// The SINGULAR name of the kind, for the line under one hit's title. The group
// headings above are plural because they head a set; a row says what that one
// row is.
//
// It exists because the palette used to print `hit.type` — the raw wire word —
// straight onto the row, so a German reader saw an English word and the first
// hyphenated type to arrive would have rendered as "offer_template".
//
// Each singular matches the plural heading above it and takes no side on which
// noun this product uses for a record type: whether `person` reads as Contacts
// or People, and `organization` as Company or Organization, is one open
// decision across both surfaces, and a key added here is not the place to
// settle it by half.
export const SEARCH_HIT_KIND_KEY: Readonly<Record<SearchHitType, MessageKey>> =
  {
    person: "search.kind.person",
    organization: "search.kind.organization",
    deal: "search.kind.deal",
    project: "search.kind.project",
    product: "search.kind.product",
    offer_template: "search.kind.offerTemplate",
    activity: "search.kind.activity",
    lead: "search.kind.lead",
    tag: "search.kind.tag",
  };

/**
 * Where a hit of this type goes, or null when it has no page to open.
 *
 * Three families, and the split is about what the thing IS rather than about
 * which code was easiest:
 *
 *   - a RECORD has a 360, and the app-wide `ENTITY` registry already says where
 *   - a TAG is not a record and has no 360, but its own page is the point of
 *     finding one: the word is the way to the records carrying it
 *   - a CATALOG row lives on the data-model settings page rather than at an
 *     address of its own, so that page is where the reader is taken. It is the
 *     honest destination and not the ideal one; a per-record address for a
 *     product is worth having and is not this change.
 *
 * An ACTIVITY returns null: it is a link rather than a thing links hang off,
 * and the results screen draws it as the canonical email row instead. Where
 * that activity IS a message, searchEmailRoute below is its destination — a
 * question about the hit rather than about its type, which is why it is not
 * an arm of this one.
 */
export function searchHitRoute(type: SearchHitType, id: string): Route | null {
  if (isEntityKind(type)) {
    return ENTITY[type as EntityKind].route(id);
  }
  if (type === "tag") {
    return { screen: "tags", id };
  }
  if (type === "product" || type === "offer_template") {
    // Through settingsHref rather than a path spelled here, and naming a page
    // the SCREEN renders today. The catalog already splits this entry into
    // fields/tags/products, but the screen still renders the combined one, so
    // minting `products` would send a reader to a page that falls back to
    // Account. It moves to `products` in the same change that splits the cards.
    return settingsHref("fields");
  }
  return null;
}

/**
 * Where an EMAIL hit goes: the results screen, with that message open.
 *
 * A message has no page of its own — it opens a drawer owned by the page it is
 * on, and that ownership is deliberate: it is what makes a record page reset
 * the drawer when the record changes. So the destination is the one page that
 * already owns this drawer and is already about finding things.
 *
 * The query rides in `id` because the screen is the search results and they
 * have to be the results for something; the message rides in `id2`, which
 * `Route` has carried since the share and contact-tab routes needed a second
 * slot. No routing change, and no app-level owner of "which email is open".
 */
export function searchEmailRoute(query: string, activityId: string): Route {
  return { screen: "search", id: query, id2: activityId };
}

/**
 * Where this HIT goes, as against where its TYPE lives.
 *
 * The difference is the whole of #3850: an `activity` has no page, so
 * searchHitRoute answers null for the type — but an activity that carries an
 * `email_summary` is a message, and a message has a destination. Asking about
 * the hit rather than about the type is what lets the palette offer one.
 *
 * Here rather than in the palette because the palette is one of two surfaces
 * that draw a hit, and the last time each answered this for itself the two
 * drifted until the results screen was dropping project hits on the floor.
 */
export function searchHitDestination(
  hit: Readonly<{
    type: SearchHitType;
    id: string;
    email_summary?: Readonly<{ activity_id: string }> | null;
  }>,
  query: string,
): Route | null {
  if (hit.email_summary) {
    return searchEmailRoute(query, hit.email_summary.activity_id);
  }
  return searchHitRoute(hit.type, hit.id);
}

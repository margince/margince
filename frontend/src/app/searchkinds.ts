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

// What a reader calls a hit, which is not always its wire type. An activity
// that is a message is an EMAIL to anybody scanning the results, and filing it
// under "Activities" beside calls and notes is how a search for an account read
// as a list of mail threads with nothing saying what they were.
export type SearchHitGroup = SearchHitType | "email";

// The order both surfaces draw their groups in. Records first, most-asked-for
// first; then what was SAID about them, because a thread that names an account
// ten times is still not the account; a tag last because it is a WORD, and
// somebody who typed a name is usually after the records rather than the label
// they were filed under.
export const SEARCH_GROUP_ORDER = [
  "contact",
  "company",
  "deal",
  "lead",
  "project",
  "product",
  "offer_template",
  "email",
  "activity",
  "tag",
] as const satisfies readonly SearchHitGroup[];

// The types a reader can narrow to, in the same order. An email narrows as the
// activity it is, so it has no pill of its own.
export const SEARCH_HIT_ORDER: readonly SearchHitType[] =
  SEARCH_GROUP_ORDER.filter(
    (group): group is SearchHitType => group !== "email",
  );

/** Which group a hit is drawn in: its type, unless it is a message. */
export function searchHitGroup(
  hit: Readonly<{ type: SearchHitType; email_summary?: unknown }>,
): SearchHitGroup {
  return hit.email_summary ? "email" : hit.type;
}

/** The type a group narrows to, and the type the server counts it under. */
export function searchGroupType(group: SearchHitGroup): SearchHitType {
  return group === "email" ? "activity" : group;
}

/**
 * The kinds drawn with the record's own mark — a contact and a company, the
 * two records a chip stands for — on the results page and in the palette.
 */
export type SearchRecordCardType = "contact" | "company";
export function searchHitHasCard(
  type: SearchHitType,
): type is SearchRecordCardType {
  return type === "contact" || type === "company";
}

/**
 * A page of hits, grouped and in SEARCH_GROUP_ORDER, each group in the order
 * the server ranked it. Empty groups are absent.
 */
export function groupSearchHits<
  Hit extends Readonly<{ type: SearchHitType; email_summary?: unknown }>,
>(hits: readonly Hit[]): { group: SearchHitGroup; hits: Hit[] }[] {
  return SEARCH_GROUP_ORDER.map((group) => ({
    group,
    hits: hits.filter((hit) => searchHitGroup(hit) === group),
  })).filter(({ hits: members }) => members.length > 0);
}

// The heading a group of these hits carries. One key per member of the contract
// enum, so a type the server learns to return cannot reach the screen without a
// name to file it under — the failure that dropped project hits.
export const SEARCH_HIT_GROUP_KEY: Readonly<Record<SearchHitType, MessageKey>> =
  {
    contact: "search.group.contact",
    company: "search.group.company",
    deal: "search.group.deal",
    project: "search.group.project",
    product: "search.group.product",
    offer_template: "search.group.offerTemplate",
    activity: "search.group.activity",
    lead: "search.group.lead",
    tag: "search.group.tag",
  };

// The heading of each group a hit can be drawn in.
export const SEARCH_GROUP_KEY: Readonly<Record<SearchHitGroup, MessageKey>> = {
  ...SEARCH_HIT_GROUP_KEY,
  email: "search.group.email",
};

// The label of each pill that narrows to a type. The activity pill narrows to
// emails AND the calls, notes and meetings beside them, and says so; every
// other pill reads as its group's heading.
export const SEARCH_FILTER_KEY: Readonly<Record<SearchHitType, MessageKey>> = {
  ...SEARCH_HIT_GROUP_KEY,
  activity: "search.filter.activity",
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
// noun this product uses for a record type: whether `contact` reads as Contacts
// or Contacts, and `company` as Company or Company, is one open
// decision across both surfaces, and a key added here is not the place to
// settle it by half.
export const SEARCH_HIT_KIND_KEY: Readonly<Record<SearchHitType, MessageKey>> =
  {
    contact: "search.kind.contact",
    company: "search.kind.company",
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

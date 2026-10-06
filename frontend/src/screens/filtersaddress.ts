// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What a Filters and views address opens, and every correspondence between the
// words three endpoint families use for one record type. Light on purpose: the
// shell, the trail and every filter page read it, so it imports no screen.

import type { components } from "../api/schema";
import type { PluralBase } from "../i18n";
import type { MessageKey } from "../i18n/en";

type FilterResource = components["schemas"]["FilterVocabulary"]["resource"];
type ListRecordType = components["schemas"]["List"]["entity_type"];
type SavedViewResource = components["schemas"]["SavedViewResource"];

/**
 * The resources whose lists offer saved views, as the contract spells them:
 * plural here and singular on `/filters/*`, because the two endpoint families
 * spell their enums differently. `VIEW_OF` below is where the two meet.
 */
export type ViewResource =
  | "contacts"
  | "companies"
  | "deals"
  | "leads"
  | "projects";

/**
 * The object tabs, and the record type each reads.
 *
 * The tab says "Contacts" and the vocabulary says "contact": the wire's word and
 * the product's word differ, and this is the one place that correspondence is
 * written down rather than assumed at each call site.
 */
export const OBJECT_TABS = ["contacts", "companies", "deals", "leads"] as const;
export type ObjectTab = (typeof OBJECT_TABS)[number];

export function isObjectTab(id: string | undefined): id is ObjectTab {
  return OBJECT_TABS.some((tab) => tab === id);
}

/** The address segment below `filters` that opens a Live List's filter. */
export const EDIT_LIST_SEGMENT = "list";

export const RESOURCE_OF: Record<ObjectTab, FilterResource> = {
  contacts: "contact",
  companies: "company",
  deals: "deal",
  leads: "lead",
};

/**
 * The same objects again, as `/views` spells them.
 *
 * A third spelling, and it is not a mistake to fix here: `/filters/*` takes
 * `contact` and `/views` takes `contacts`, both enumerated in the contract. So
 * the correspondence is written down once, beside `RESOURCE_OF`, rather than
 * derived at each call site by adding an "s".
 */
export const VIEW_OF: Record<ObjectTab, ViewResource> = {
  contacts: "contacts",
  companies: "companies",
  deals: "deals",
  leads: "leads",
};

export const TAB_LABEL: Record<ObjectTab, MessageKey> = {
  contacts: "filters.tab.contacts",
  companies: "filters.tab.companies",
  deals: "filters.tab.deals",
  leads: "filters.tab.leads",
};

export const MATCH_LABEL: Record<ObjectTab, PluralBase> = {
  contacts: "filters.matchContacts",
  companies: "filters.matchCompanies",
  deals: "filters.matchDeals",
  leads: "filters.matchLeads",
};

/** The plural noun the results table counts and names its empty state by. */
export const UNIT_LABEL: Record<ObjectTab, MessageKey> = {
  contacts: "unit.contacts",
  companies: "unit.companies",
  deals: "unit.deals",
  leads: "unit.leads",
};

/** What the one primary verb says once a record type is chosen. */
export const NEW_FILTER_LABEL: Record<ObjectTab, MessageKey> = {
  contacts: "filters.new.contacts",
  companies: "filters.new.companies",
  deals: "filters.new.deals",
  leads: "filters.new.leads",
};

/** A count of a list's records, in the noun its record type is counted in. */
export const RECORDS_COUNT_LABEL: Record<ListRecordType, PluralBase> = {
  contact: "filters.library.records.contact",
  company: "filters.library.records.company",
  deal: "filters.library.records.deal",
  lead: "filters.library.records.lead",
  project: "filters.library.records.project",
};

/**
 * The record types the library cuts by: the four a filter is built on, and
 * projects, which a list can hold but no builder edits.
 */
export const LIBRARY_TYPES = [...OBJECT_TABS, "projects"] as const;
export type LibraryType = (typeof LIBRARY_TYPES)[number];

const LIBRARY_TYPE_OF_LIST: Record<ListRecordType, LibraryType> = {
  contact: "contacts",
  company: "companies",
  deal: "deals",
  lead: "leads",
  project: "projects",
};

export function libraryTypeOfList(type: ListRecordType): LibraryType {
  return LIBRARY_TYPE_OF_LIST[type];
}

/** What a pill with no rows says, in that record type's own noun. */
export const NO_TYPE_HITS_LABEL: Record<LibraryType, MessageKey> = {
  contacts: "filters.library.noTypeHits.contacts",
  companies: "filters.library.noTypeHits.companies",
  deals: "filters.library.noTypeHits.deals",
  leads: "filters.library.noTypeHits.leads",
  projects: "filters.library.noTypeHits.projects",
};

/** The builder tab a record type's Live List opens on; projects have none. */
export function tabOfListType(type: ListRecordType): ObjectTab | undefined {
  const libraryType = LIBRARY_TYPE_OF_LIST[type];
  return isObjectTab(libraryType) ? libraryType : undefined;
}

/**
 * The tab a saved view's filter opens on. Views over activities, partners or
 * projects hold list state no builder reads, so they have none.
 */
export function tabOfViewResource(
  resource: SavedViewResource,
): ObjectTab | undefined {
  return OBJECT_TABS.find((tab) => VIEW_OF[tab] === resource);
}

/** The group `#/filters/views` or `#/filters/lists` lands on. */
export type LibraryAnchor = "views" | "lists";

export type FiltersAddress =
  | Readonly<{ kind: "library"; anchor?: LibraryAnchor }>
  | Readonly<{ kind: "new"; tab: ObjectTab }>
  | Readonly<{ kind: "view"; tab: ObjectTab; viewId: string }>
  | Readonly<{ kind: "listFilter"; listId: string }>;

/**
 * What `#/filters/<id>/<id2>` opens. Anything this does not recognise is the
 * library, so a stale or mistyped link lands somewhere a reader can start from
 * rather than on a builder for a record type they never chose.
 */
export function filtersAddressOf({
  id,
  id2,
}: Readonly<{ id?: string; id2?: string }>): FiltersAddress {
  if (id === EDIT_LIST_SEGMENT) {
    return id2 ? { kind: "listFilter", listId: id2 } : { kind: "library" };
  }
  if (isObjectTab(id)) {
    return id2
      ? { kind: "view", tab: id, viewId: id2 }
      : { kind: "new", tab: id };
  }
  if (id === "views" || id === "lists") {
    return { kind: "library", anchor: id };
  }
  return { kind: "library" };
}

/**
 * Whether the page at this address prints its own heading: a filter page, or
 * one list. Structural rather than the router's `Route`, so the shell can ask
 * without this module reaching into the app.
 */
export function opensAFocusedFiltersPage(
  route: Readonly<{ screen: string; id?: string; id2?: string }>,
): boolean {
  if (route.screen === "lists") {
    return Boolean(route.id);
  }
  return (
    route.screen === "filters" && filtersAddressOf(route).kind !== "library"
  );
}

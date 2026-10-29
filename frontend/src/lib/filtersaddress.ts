// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What a Filters and views address opens, and every correspondence between the
// words three endpoint families use for one record type. Light on purpose: the
// shell, the trail and every filter page read it, so it imports no screen.

import type { components } from "../api/schema";
import { type PluralBase, type Translator, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { type ExtensionEntityKind, isExtensionEntityKind } from "./entity";
import { recordKindLabel, useRecordKindDirectory } from "./recordkinds";

type FilterResource = components["schemas"]["FilterResource"];
import type { ListRecordType } from "./lists.queries";

/**
 * The core resources whose lists offer saved views, as the contract spells them.
 * They are plural here and singular on `/filters/*`, because the two endpoint
 * families spell them differently. `viewOf` below is where the two meet.
 */
export type ViewResource =
  | "contacts"
  | "companies"
  | "deals"
  | "leads"
  | "projects"
  | ExtensionEntityKind;

/**
 * The core object tabs, and the record type each reads.
 *
 * The tab says "Contacts" and the vocabulary says "contact": the wire's word and
 * the product's word differ, and this is the one place that correspondence is
 * written down rather than assumed at each call site.
 */
export const OBJECT_TABS = ["contacts", "companies", "deals", "leads"] as const;
type CoreObjectTab = (typeof OBJECT_TABS)[number];

/** A core tab, or a record kind an extension declared, which names itself. */
export type ObjectTab = CoreObjectTab | ExtensionEntityKind;

export function isObjectTab(id: string | undefined): id is ObjectTab {
  return (
    id !== undefined &&
    (OBJECT_TABS.some((tab) => tab === id) || isExtensionEntityKind(id))
  );
}

/** The address segment below `filters` that opens a Live List's filter. */
export const EDIT_LIST_SEGMENT = "list";

const CORE_RESOURCE_OF = {
  contacts: "contact",
  companies: "company",
  deals: "deal",
  leads: "lead",
} as const satisfies Record<CoreObjectTab, FilterResource>;

/** The record type `/filters/*` is asked about for this tab. */
export function resourceOf(tab: ObjectTab): FilterResource {
  return isExtensionEntityKind(tab) ? tab : CORE_RESOURCE_OF[tab];
}

/**
 * The record type a Live List is made over, absent for an extension kind. The
 * list screens draw core types only, so the filter is saved as a view instead.
 */
export function listEntityTypeOf(tab: ObjectTab): ListRecordType | undefined {
  return isExtensionEntityKind(tab) ? undefined : CORE_RESOURCE_OF[tab];
}

/**
 * The same objects again, as `/views` spells them.
 *
 * A third spelling, and it is not a mistake to fix here: `/filters/*` takes
 * `contact` and `/views` takes `contacts`, both enumerated in the contract. So
 * the correspondence is written down once, beside `resourceOf`, rather than
 * derived at each call site by adding an "s".
 */
export function viewOf(tab: ObjectTab): ViewResource {
  return tab;
}

const TAB_LABEL: Record<CoreObjectTab, MessageKey> = {
  contacts: "filters.tab.contacts",
  companies: "filters.tab.companies",
  deals: "filters.tab.deals",
  leads: "filters.tab.leads",
};

const MATCH_LABEL: Record<CoreObjectTab, PluralBase> = {
  contacts: "filters.matchContacts",
  companies: "filters.matchCompanies",
  deals: "filters.matchDeals",
  leads: "filters.matchLeads",
};

const UNIT_LABEL: Record<CoreObjectTab, MessageKey> = {
  contacts: "unit.contacts",
  companies: "unit.companies",
  deals: "unit.deals",
  leads: "unit.leads",
};

const NEW_FILTER_LABEL: Record<CoreObjectTab, MessageKey> = {
  contacts: "filters.new.contacts",
  companies: "filters.new.companies",
  deals: "filters.new.deals",
  leads: "filters.new.leads",
};

const SWITCH_CLEARS_LABEL: Record<CoreObjectTab, MessageKey> = {
  contacts: "filters.switch.body.contacts",
  companies: "filters.switch.body.companies",
  deals: "filters.switch.body.deals",
  leads: "filters.switch.body.leads",
};

/** What the directory says of an extension's kinds, as far as words need it. */
type KindDirectory = readonly { name: string; display_name: string }[];

/** Every word a filter page says about the record type it is built on. */
export type TabWords = Readonly<{
  /** The tab's name on the segmented control and the library's pills. */
  tab: string;
  /** The plural noun the results table counts and names its empty state by. */
  unit: string;
  /** What the one primary verb says once a record type is chosen. */
  newFilter: string;
  /** Why switching away from this record type clears the conditions. */
  switchClears: string;
  /** The count's plural base, and the values it interpolates besides `count`. */
  match: PluralBase;
  matchParams: Readonly<Record<string, string>>;
}>;

/** The words for a tab: the catalogue's for a core one, the directory's name for an extension's. */
export function tabWords(
  tab: ObjectTab,
  t: Translator,
  directory: KindDirectory,
): TabWords {
  if (isExtensionEntityKind(tab)) {
    const kind = recordKindLabel(tab, directory);
    return {
      tab: kind,
      unit: t("unit.extension", { kind }),
      newFilter: t("filters.new.extension", { kind }),
      switchClears: t("filters.switch.body.extension"),
      match: "filters.matchExtension",
      matchParams: { kind },
    };
  }
  return {
    tab: t(TAB_LABEL[tab]),
    unit: t(UNIT_LABEL[tab]),
    newFilter: t(NEW_FILTER_LABEL[tab]),
    switchClears: t(SWITCH_CLEARS_LABEL[tab]),
    match: MATCH_LABEL[tab],
    matchParams: {},
  };
}

/** The words for a tab, with the kind directory read once per session. */
export function useTabWords(): (tab: ObjectTab) => TabWords {
  const t = useT();
  const directory = useRecordKindDirectory().data?.data ?? [];
  return (tab) => tabWords(tab, t, directory);
}

/** The tabs the builder offers: the four core ones, then the directory's kinds. */
export function useObjectTabs(): readonly ObjectTab[] {
  const directory = useRecordKindDirectory().data?.data ?? [];
  return [
    ...OBJECT_TABS,
    ...directory.map((entry) => entry.name).filter(isExtensionEntityKind),
  ];
}

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
export type CoreLibraryType = (typeof LIBRARY_TYPES)[number];

/** A row's record type: a pill's, or an extension's kind, which has no pill. */
export type LibraryType = CoreLibraryType | ExtensionEntityKind;

const LIBRARY_TYPE_OF_LIST: Record<ListRecordType, CoreLibraryType> = {
  contact: "contacts",
  company: "companies",
  deal: "deals",
  lead: "leads",
  project: "projects",
};

export function libraryTypeOfList(
  type: ListRecordType | ExtensionEntityKind,
): LibraryType {
  return isExtensionEntityKind(type) ? type : LIBRARY_TYPE_OF_LIST[type];
}

/** What a pill with no rows says, in that record type's own noun. */
export const NO_TYPE_HITS_LABEL: Record<CoreLibraryType, MessageKey> = {
  contacts: "filters.library.noTypeHits.contacts",
  companies: "filters.library.noTypeHits.companies",
  deals: "filters.library.noTypeHits.deals",
  leads: "filters.library.noTypeHits.leads",
  projects: "filters.library.noTypeHits.projects",
};

/** The builder tab a record type's Live List opens on; projects have none. */
export function tabOfListType(
  type: ListRecordType | ExtensionEntityKind,
): ObjectTab | undefined {
  const libraryType = libraryTypeOfList(type);
  return isObjectTab(libraryType) ? libraryType : undefined;
}

/**
 * The tab a saved view's filter opens on. Views over activities, partners or
 * projects hold list state no builder reads, so they have none.
 */
export function tabOfViewResource(resource: string): ObjectTab | undefined {
  if (isExtensionEntityKind(resource)) {
    return resource;
  }
  return OBJECT_TABS.find((tab) => viewOf(tab) === resource);
}

/** The group `#/filters/views` or `#/filters/lists` lands on. */
export type LibraryAnchor = "views" | "lists";

export type FiltersAddress =
  | Readonly<{ kind: "library"; anchor?: LibraryAnchor }>
  | Readonly<{ kind: "new"; tab: ObjectTab }>
  | Readonly<{ kind: "view"; tab: ObjectTab; viewId: string }>
  | Readonly<{ kind: "listFilter"; listId: string }>;

/** An address that opens a focused page rather than the library. */
export type FocusedFiltersAddress = Exclude<
  FiltersAddress,
  Readonly<{ kind: "library" }>
>;

/**
 * The name a focused page wears until it has read its own — a view's or a
 * list's name — so it is never without a heading while it waits.
 */
export function fallbackTitleOf(
  address: FocusedFiltersAddress,
  t: Translator,
  directory: KindDirectory,
): string {
  switch (address.kind) {
    case "new":
      return tabWords(address.tab, t, directory).newFilter;
    case "view":
      return t("filters.library.kindView");
    case "listFilter":
      return t("lists.editFilter");
  }
}

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

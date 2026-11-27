// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The Filters and views library as data. Pure, so the rules the page draws by
// are proved here without a page.

import { listOpening } from "../../components/listquery.options";
import { paramsFromListQuery } from "../../components/listqueryparams";
import {
  forReader,
  holdsEveryWord,
  searchWords,
  stable,
} from "../../format/collate";
import type { Locale } from "../../i18n";
import { VIEW_PARAM } from "../../lib/deals.queries";
import { isExtensionEntityKind } from "../../lib/entity";
import type { FilterResource } from "../../lib/filterdata";
import {
  type CoreLibraryType,
  LIBRARY_TYPES,
  type LibraryType,
  libraryTypeOfList,
  type ObjectTab,
  resourceOf as resourceOfTab,
  tabOfViewResource,
} from "../../lib/filtersaddress";
import type { List } from "../../lib/lists.queries";
import { routeHash } from "../../lib/router";
import { filterTreeOf, type SavedView } from "../../lib/savedviews.queries";
import { decode, type Node, rootGroup } from "../../lib/segmentpredicate";
import { hashWithParams } from "../../lib/urlstate";
import { listStateOf, type SavedListState } from "./savedviews";

/** A view saved on a list screen, which opens on that list. */
export type ListViewItem = Readonly<{
  kind: "listView";
  view: SavedView;
  state: SavedListState;
  type: CoreLibraryType;
}>;

export type LibraryItem =
  | Readonly<{ kind: "view"; view: SavedView; tree: Node; tab: ObjectTab }>
  | ListViewItem
  | Readonly<{ kind: "list"; list: List }>;

/** `mine`: what the reader owns. `shared`: what someone else or nobody owns. */
export type LibraryGroup = "mine" | "shared";

/** The cut the address holds: the search, the pressed pill, the archived toggle. */
export type LibraryCut = Readonly<{
  q: string;
  type: CoreLibraryType | "all";
  archived: boolean;
}>;

/**
 * Saved views and lists as one set of rows. A view with a filter the builder
 * reads opens in the builder, even when it also keeps a list's dials. One
 * with only a list's dials opens on that list. Any other view is left out: a
 * row that opens onto nothing is worse than no row. A stored single clause is
 * wrapped in a group, the shape every editor of it expects.
 */
export function libraryItems(
  views: readonly SavedView[],
  lists: readonly List[],
): LibraryItem[] {
  const fromViews = views.flatMap((view): LibraryItem[] => {
    const tab = tabOfViewResource(view.resource);
    const tree = filterTreeOf(view);
    if (tab && tree) {
      return [{ kind: "view", view, tree: rootGroup(tree), tab }];
    }
    const type = LIBRARY_TYPES.find((known) => known === view.resource);
    const state = listStateOf(view);
    return type && state ? [{ kind: "listView", view, state, type }] : [];
  });
  const fromLists = lists.map((list): LibraryItem => ({ kind: "list", list }));
  return [...fromViews, ...fromLists];
}

/**
 * A capped read's lists, and what a server search found past the cap. The
 * server never matches a caption, so a held row it missed may still be wanted.
 */
export function withFound(
  capped: readonly List[],
  found: readonly List[],
): List[] {
  const held = new Set(capped.map((list) => list.id));
  return [...capped, ...found.filter((list) => !held.has(list.id))];
}

export function nameOf(item: LibraryItem): string {
  return item.kind === "list" ? item.list.name : item.view.name;
}

/** Unique across both kinds, since a view and a list may share an id. */
export function keyOf(item: LibraryItem): string {
  return item.kind === "list" ? `l:${item.list.id}` : `v:${item.view.id}`;
}

export function typeOf(item: LibraryItem): LibraryType {
  switch (item.kind) {
    case "view":
      return item.tab;
    case "listView":
      return item.type;
    case "list":
      return libraryTypeOfList(item.list.entity_type);
  }
}

/** The record type whose vocabulary names this row's fields. */
export function resourceOf(item: LibraryItem): FilterResource {
  switch (item.kind) {
    case "view":
      return resourceOfTab(item.tab);
    case "listView":
      return item.type === "projects" ? "project" : resourceOfTab(item.type);
    case "list":
      return item.list.entity_type;
  }
}

/** The filter a row selects by, or null for a Shortlist, a list's view or an unreadable one. */
export function treeOf(item: LibraryItem): Node | null {
  switch (item.kind) {
    case "view":
      return item.tree;
    case "listView":
      return null;
    case "list":
      return item.list.list_type === "dynamic"
        ? decode(item.list.definition)
        : null;
  }
}

/**
 * Where a view saved on a list opens: that list, its dials in the address the
 * list writes itself. The view's tab then lights there. `openingFilters` are
 * the filters that list opens on for this reader.
 */
export function listViewHref(
  item: ListViewItem,
  openingFilters: Readonly<Record<string, string>>,
): string {
  const opening = listOpening(undefined, openingFilters);
  const dials = new Map(
    paramsFromListQuery(
      { ...item.state, perPage: item.state.perPage ?? opening.perPage },
      opening,
    ),
  );
  if (item.type === "deals") {
    // Deals saves a view from its table alone, so the view opens there.
    dials.set(VIEW_PARAM, "table");
  }
  return hashWithParams(routeHash({ screen: item.type }), dials);
}

export function isArchived(item: LibraryItem): boolean {
  return item.kind === "list" && Boolean(item.list.archived_at);
}

export function ownedByReader(
  list: Pick<List, "owner_id">,
  readerId: string | undefined,
): boolean {
  // Not a bare `===`: an ownerless list would match an unknown reader.
  return readerId !== undefined && list.owner_id === readerId;
}

/**
 * A saved view is private in V1, so it is always the reader's own. A list
 * goes by its owner, not its audience, so a list the reader shares stays theirs.
 */
export function groupOf(
  item: LibraryItem,
  readerId: string | undefined,
): LibraryGroup {
  if (item.kind !== "list") {
    return "mine";
  }
  return ownedByReader(item.list, readerId) ? "mine" : "shared";
}

export function listsLanding(
  shown: readonly LibraryItem[],
  readerId: string | undefined,
): LibraryGroup {
  return shown.some(
    (item) => item.kind === "list" && groupOf(item, readerId) === "mine",
  )
    ? "mine"
    : "shared";
}

/**
 * By name, the way this reader reads an alphabet. Both servers sort by bytes,
 * where "Ärzte" lands after "Zahnärzte", and two merged reads would interleave
 * by neither order. The key breaks a tie so a re-read never reshuffles.
 */
export function sortForReader(
  items: readonly LibraryItem[],
  locale: Locale,
): LibraryItem[] {
  return [...items].sort(
    (one, other) =>
      forReader(nameOf(one), nameOf(other), locale) ||
      stable(keyOf(one), keyOf(other)),
  );
}

/** Whether the reader has narrowed the library at all. */
export function isCut(cut: LibraryCut): boolean {
  return cut.type !== "all" || cut.q.trim() !== "";
}

/**
 * The rows the cut leaves. The search matches the name or the caption, in any
 * case, because the caption is what a reader remembers a filter by.
 */
export function cutLibrary(
  items: readonly LibraryItem[],
  cut: LibraryCut,
  captionOf: (item: LibraryItem) => string,
): LibraryItem[] {
  const words = searchWords(cut.q);
  return items.filter(
    (item) =>
      (cut.archived || !isArchived(item)) &&
      (cut.type === "all" || typeOf(item) === cut.type) &&
      holdsEveryWord(words, [nameOf(item), captionOf(item)]),
  );
}

export type PillCounts = Readonly<Record<CoreLibraryType | "all", number>>;

/**
 * How many rows each pill holds, ignoring the search so a pill keeps saying
 * what pressing it would show. Absent while `partial`, a read still out,
 * refused or stopped at its cap: the number would be a floor printed as a count.
 */
export function pillCounts(
  items: readonly LibraryItem[],
  archived: boolean,
  partial: boolean,
): PillCounts | undefined {
  if (partial) {
    return undefined;
  }
  const counts = {
    all: 0,
    contacts: 0,
    companies: 0,
    deals: 0,
    leads: 0,
    projects: 0,
  };
  for (const item of items) {
    if (archived || !isArchived(item)) {
      counts.all += 1;
      // An extension's kind has no pill, so it counts under All alone.
      const type = typeOf(item);
      if (!isExtensionEntityKind(type)) {
        counts[type] += 1;
      }
    }
  }
  return counts;
}

/** Nothing saved yet: no live view or list, archived ones aside. */
export function isFirstRun(items: readonly LibraryItem[]): boolean {
  return items.every(isArchived);
}

/**
 * The groups worth drawing. A group with rows always is. With no cut, an
 * empty group still is, to say what would go there; under a cut it is left
 * out, so the reader sees only where their search landed. A group whose read
 * is still out or failed is drawn to say so.
 */
export function visibleGroups(
  groups: readonly LibraryGroup[],
  shown: readonly LibraryItem[],
  readerId: string | undefined,
  cut: boolean,
  unsettled: (group: LibraryGroup) => boolean,
): LibraryGroup[] {
  return groups.filter(
    (group) =>
      unsettled(group) ||
      shown.some((item) => groupOf(item, readerId) === group) ||
      (!cut && shown.length > 0),
  );
}

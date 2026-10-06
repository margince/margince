// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The Filters and views library as data: every saved view and list the reader
// can use, merged into one set of rows, grouped by who can find them and cut
// by what the reader typed and pressed. Pure, so the rules the page draws by
// are proved here without a page.

import { forReader, stable } from "../format/collate";
import type { Locale } from "../i18n";
import type { FilterResource } from "./filterdata";
import {
  type LibraryType,
  libraryTypeOfList,
  type ObjectTab,
  RESOURCE_OF,
  tabOfViewResource,
} from "./filtersaddress";
import type { List } from "./lists.queries";
import { filterTreeOf, type SavedView } from "./savedviews.queries";
import { decode, type Node, rootGroup } from "./segmentpredicate";

export type LibraryItem =
  | Readonly<{ kind: "view"; view: SavedView; tree: Node; tab: ObjectTab }>
  | Readonly<{ kind: "list"; list: List }>;

/** Only me holds what the reader alone can find; Shared, what others can. */
export type LibraryGroup = "mine" | "shared";

/** The cut the address holds: the search, the pressed pill, the archived toggle. */
export type LibraryCut = Readonly<{
  q: string;
  type: LibraryType | "all";
  archived: boolean;
}>;

/**
 * Saved views and lists as one set of rows. A view whose filter this build
 * cannot read, or that keeps a list's dials rather than a filter, is left out:
 * a row that opens onto nothing is worse than no row. A stored single clause
 * is wrapped in a group, the shape every editor of it expects.
 */
export function libraryItems(
  views: readonly SavedView[],
  lists: readonly List[],
): LibraryItem[] {
  const fromViews = views.flatMap((view): LibraryItem[] => {
    const tab = tabOfViewResource(view.resource);
    const tree = filterTreeOf(view);
    return tab && tree
      ? [{ kind: "view", view, tree: rootGroup(tree), tab }]
      : [];
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
  return item.kind === "view" ? item.view.name : item.list.name;
}

/** Unique across both kinds, since a view and a list may share an id. */
export function keyOf(item: LibraryItem): string {
  return item.kind === "view" ? `v:${item.view.id}` : `l:${item.list.id}`;
}

export function typeOf(item: LibraryItem): LibraryType {
  return item.kind === "view"
    ? item.tab
    : libraryTypeOfList(item.list.entity_type);
}

/** The record type whose vocabulary names this row's fields. */
export function resourceOf(item: LibraryItem): FilterResource {
  return item.kind === "view" ? RESOURCE_OF[item.tab] : item.list.entity_type;
}

/** The filter a row selects by, or null for a Shortlist or an unreadable one. */
export function treeOf(item: LibraryItem): Node | null {
  if (item.kind === "view") {
    return item.tree;
  }
  return item.list.list_type === "dynamic"
    ? decode(item.list.definition)
    : null;
}

export function isArchived(item: LibraryItem): boolean {
  return item.kind === "list" && Boolean(item.list.archived_at);
}

/** A saved view is private in V1, so it is always the reader's own. */
export function groupOf(item: LibraryItem): LibraryGroup {
  return item.kind === "list" && item.list.sharing !== "private"
    ? "shared"
    : "mine";
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
  const q = cut.q.trim().toLowerCase();
  return items.filter(
    (item) =>
      (cut.archived || !isArchived(item)) &&
      (cut.type === "all" || typeOf(item) === cut.type) &&
      (q === "" ||
        `${nameOf(item)} ${captionOf(item)}`.toLowerCase().includes(q)),
  );
}

export type PillCounts = Readonly<Record<LibraryType | "all", number>>;

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
      counts[typeOf(item)] += 1;
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
  cut: boolean,
  unsettled: (group: LibraryGroup) => boolean,
): LibraryGroup[] {
  return groups.filter(
    (group) =>
      unsettled(group) ||
      shown.some((item) => groupOf(item) === group) ||
      (!cut && shown.length > 0),
  );
}

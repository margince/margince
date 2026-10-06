// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What the Filters and views library reads, and where its cut is held: one
// read of every saved view and one of every list, the caption under each row,
// and the search, pill and archived toggle in the address.

import { useEffect, useMemo, useState } from "react";
import type { UrlParams } from "../app/urlstate";
import { SEARCH_DEBOUNCE_MS } from "../design-system/debouncedsearch";
import { formatNumber } from "../format/format";
import { useLocale } from "../i18n";
import { type FilterResource, useFilterVocabulary } from "./filterdata";
import { LIBRARY_TYPES } from "./filtersaddress";
import { filterSentence, useSentenceWords } from "./filtersentence";
import {
  type LibraryCut,
  type LibraryGroup,
  type LibraryItem,
  libraryItems,
  resourceOf,
  sortForReader,
  treeOf,
  withFound,
} from "./library";
import { useLists } from "./lists.queries";
import { useAllSavedViews } from "./savedviews.queries";

export const SEARCH_PARAM = "q";
export const TYPE_PARAM = "type";
export const ARCHIVED_PARAM = "archived";

/** The cut the address holds; an unknown type reads as no cut at all. */
export function cutOf(params: UrlParams): LibraryCut {
  const type = params.get(TYPE_PARAM);
  return {
    q: params.get(SEARCH_PARAM) ?? "",
    type: LIBRARY_TYPES.find((known) => known === type) ?? "all",
    archived: params.get(ARCHIVED_PARAM) === "1",
  };
}

export type LibraryReads = Readonly<{
  items: readonly LibraryItem[];
  settled: boolean;
  failed: boolean;
  viewsPending: boolean;
  viewsFailed: boolean;
  listsPending: boolean;
  listsFailed: boolean;
  /** The list rows on screen answer the previous archived toggle, not this one. */
  listsHeld: boolean;
  unsettled: (group: LibraryGroup) => boolean;
  truncated: boolean;
  /** Every read answered in full, so a count is the whole figure. */
  countable: boolean;
  /** How many rows a truncated read stopped at, formatted. */
  limit: string;
  retryViews: () => void;
  retryLists: () => void;
}>;

/**
 * One read of every saved view and one of every list, merged. While the lists
 * read is cut short, the search goes to the server too, once the reader pauses,
 * because the rows it would find may be past the cap; what it finds joins the
 * rows already held, which stay on screen before that answer lands and after.
 */
export function useLibraryReads(
  cut: LibraryCut,
  listsOn: boolean,
): LibraryReads {
  const { locale } = useLocale();
  const views = useAllSavedViews();
  const lists = useLists({ includeArchived: cut.archived }, listsOn, true);
  const listsTruncated = lists.data?.page.has_more === true;
  const typed = cut.q.trim();
  const q = usePaused(typed);
  const searching = listsOn && listsTruncated && q !== "" && typed !== "";
  const searched = useLists(
    { q, includeArchived: cut.archived },
    searching,
    true,
  );
  const listRead = searching && !searched.isPending ? searched : lists;
  const viewRows = views.data?.views;
  const cappedRows = lists.data?.data;
  const listRows = listRead.data?.data;
  const items = useMemo(
    () =>
      sortForReader(
        libraryItems(
          viewRows ?? [],
          listsOn ? withFound(cappedRows ?? [], listRows ?? []) : [],
        ),
        locale,
      ),
    [viewRows, cappedRows, listRows, listsOn, locale],
  );
  const viewsPending = views.isPending;
  const viewsFailed = views.isError;
  const listsHeld = listsOn && lists.isPlaceholderData;
  // Held rows stand in for the answer; with none held, the read is still out.
  const listsPending =
    listsOn && (listRead.isPending || (listsHeld && cappedRows?.length === 0));
  const listsFailed = listsOn && listRead.isError;
  // Taken from the uncut reads: a search the server answered in full still
  // leaves the library itself past its cap, and a count over it would be short.
  const truncatedRows = [
    views.data?.truncated ? (viewRows?.length ?? 0) : 0,
    listsTruncated ? (cappedRows?.length ?? 0) : 0,
  ];
  const settled = !viewsPending && !listsPending;
  const failed = viewsFailed || listsFailed;
  const truncated = truncatedRows.some((rows) => rows > 0);
  return {
    items,
    settled,
    failed,
    viewsPending,
    viewsFailed,
    listsPending,
    listsFailed,
    listsHeld,
    unsettled: (group) =>
      listsPending ||
      listsFailed ||
      (group === "mine" && (viewsPending || viewsFailed)),
    truncated,
    countable: settled && !failed && !truncated && !listsHeld,
    limit: formatNumber(Math.max(...truncatedRows), locale),
    retryViews: () => void views.refetch(),
    retryLists: () => void listRead.refetch(),
  };
}

/** `value` once it has held still as long as the list search waits. */
function usePaused(value: string): string {
  const [paused, setPaused] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => setPaused(value), SEARCH_DEBOUNCE_MS);
    return () => clearTimeout(timer);
  }, [value]);
  return paused;
}

/**
 * What each row says under its name: a list's purpose, else the filter as a
 * sentence. A vocabulary is read only for a record type some row needs words
 * for, and until it answers the filter is counted rather than named.
 */
export function useCaptions(
  items: readonly LibraryItem[],
): (item: LibraryItem) => string {
  const words = useSentenceWords();
  const wanted = new Set<FilterResource>(
    items
      .filter((item) => treeOf(item) !== null && !purposeOf(item))
      .map(resourceOf),
  );
  const vocabularies: Readonly<Record<FilterResource, VocabularyRead>> = {
    contact: useFilterVocabulary("contact", wanted.has("contact")),
    company: useFilterVocabulary("company", wanted.has("company")),
    deal: useFilterVocabulary("deal", wanted.has("deal")),
    lead: useFilterVocabulary("lead", wanted.has("lead")),
    project: useFilterVocabulary("project", wanted.has("project")),
  };
  return (item) => {
    const purpose = purposeOf(item);
    if (purpose) {
      return purpose;
    }
    const tree = treeOf(item);
    return tree
      ? filterSentence(tree, vocabularies[resourceOf(item)].data?.fields, words)
      : "";
  };
}

type VocabularyRead = ReturnType<typeof useFilterVocabulary>;

function purposeOf(item: LibraryItem): string {
  return item.kind === "list" ? (item.list.purpose ?? "") : "";
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// `#/filters`: every saved view and list the reader can use, in one library,
// and the one place a new filter starts. It shows no builder at all; a filter
// is built on its own page, which "New filter" opens.

import { Plus } from "lucide-react";
import { type RefObject, useEffect, useRef, useState } from "react";
import { navigate } from "../app/router";
import { replaceDial, useUrlParams } from "../app/urlstate";
import { useFoldedViewport } from "../app/viewport";
import { ActionRow } from "../design-system/actionrow";
import {
  Button,
  EmptyState,
  OverflowMenu,
  PendingBody,
  SearchField,
} from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { type FilterPill, FilterPills } from "../design-system/filterpills";
import { Panel, PanelBody } from "../design-system/panel";
import { Popover } from "../design-system/popover";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  type LibraryAnchor,
  type LibraryType,
  NEW_FILTER_LABEL,
  NO_TYPE_HITS_LABEL,
  OBJECT_TABS,
  type ObjectTab,
  TAB_LABEL,
} from "./filtersaddress";
import {
  ARCHIVED_PARAM,
  cutOf,
  type LibraryReads,
  SEARCH_PARAM,
  TYPE_PARAM,
  useCaptions,
  useLibraryReads,
} from "./filterslibrary.reads";
import {
  cutLibrary,
  groupOf,
  isCut,
  isFirstRun,
  type LibraryCut,
  type LibraryItem,
  pillCounts,
  typeOf,
  visibleGroups,
} from "./library";
import "./library.css";
import {
  LibraryTable,
  type LibraryTableGroup,
  NewShortlistAction,
} from "./listlibrary";
import { useListsAvailable } from "./lists.queries";
import { DEFAULT_AUDIENCE } from "./listsharing";

export function FiltersLibrary({
  anchor,
}: Readonly<{ anchor?: LibraryAnchor }>) {
  const t = useT();
  const folded = useFoldedViewport();
  const listsOn = useListsAvailable();
  const [params] = useUrlParams();
  const cut = cutOf(params);
  const reads = useLibraryReads(cut, listsOn);
  const captionOf = useCaptions(reads.items);
  const shown = cutLibrary(reads.items, cut, captionOf);
  const hasItems =
    !isFirstRun(reads.items) || (cut.archived && reads.items.length > 0);
  const firstRun = reads.settled && !reads.failed && !hasItems;
  const narrowed = isCut(cut);
  const groups = firstRun
    ? []
    : visibleGroups(
        listsOn ? ["mine", "shared"] : ["mine"],
        shown,
        narrowed,
        reads.unsettled,
      );
  const anchors = useAnchorFocus(anchor, listsOn, reads.settled);
  // Nothing matched only once every read has answered: until then, the rows
  // that would have matched may be the ones still out or refused.
  const noHits =
    reads.settled &&
    !reads.failed &&
    narrowed &&
    hasItems &&
    shown.length === 0;
  return (
    <div className="wrap library-page">
      <LibraryToolbar
        cut={cut}
        items={reads.items}
        countable={reads.countable}
        searchable={hasItems}
        folded={folded}
        listsOn={listsOn}
      />
      {noHits && <NoHits cut={cut} />}
      {/* The first-run plate takes the saved-views group's place, so the group
          a deleted last view handed focus to stays mounted and keeps it. */}
      {(firstRun ? (["mine"] as const) : groups).map((group) => (
        <div
          key={group}
          ref={group === "mine" ? anchors.mine : anchors.shared}
          className="library-anchor"
          tabIndex={-1}
        >
          {firstRun ? (
            <EmptyState
              plate
              title={t(
                listsOn
                  ? "filters.library.firstRunTitle"
                  : "filters.library.viewsEmptyTitle",
              )}
            >
              {t(
                listsOn
                  ? "filters.library.firstRunBody"
                  : "filters.library.viewsEmptyBody",
              )}
            </EmptyState>
          ) : (
            <LibraryGroupPanel
              group={listsOn ? group : "views"}
              rows={shown.filter((item) => groupOf(item) === group)}
              reads={reads}
              narrowed={narrowed}
              captionOf={captionOf}
              folded={folded}
              returnFocusTo={() => anchors.mine.current}
            />
          )}
        </div>
      ))}
      {reads.truncated && (
        <p className="t-caption">
          {t("filters.library.truncated", { limit: reads.limit })}
        </p>
      )}
      {listsOn && (
        <div>
          <Button
            variant="link"
            onClick={() =>
              replaceDial(ARCHIVED_PARAM, cut.archived ? undefined : "1")
            }
          >
            {t(
              cut.archived
                ? "filters.library.hideArchived"
                : "filters.library.showArchived",
            )}
          </Button>
        </div>
      )}
    </div>
  );
}

function LibraryToolbar({
  cut,
  items,
  countable,
  searchable,
  folded,
  listsOn,
}: Readonly<{
  cut: LibraryCut;
  items: readonly LibraryItem[];
  countable: boolean;
  searchable: boolean;
  folded: boolean;
  listsOn: boolean;
}>) {
  const t = useT();
  const newFilter = <NewFilterAction type={cut.type} />;
  const shortlist = listsOn ? (
    <NewShortlistAction defaultAudience={DEFAULT_AUDIENCE} />
  ) : null;
  const finders = searchable ? (
    <>
      <SearchField
        value={cut.q}
        onChange={(event) => replaceDial(SEARCH_PARAM, event.target.value)}
        placeholder={t("filters.library.search")}
        aria-label={t("filters.library.search")}
      />
      <TypePills cut={cut} items={items} countable={countable} />
    </>
  ) : null;
  if (!folded) {
    return (
      <ActionRow
        primary={
          <>
            {shortlist}
            {newFilter}
          </>
        }
      >
        {finders}
      </ActionRow>
    );
  }
  // A phone puts the verb first and full width, the Shortlist one press
  // behind it, and the search and pills each on their own line under them.
  return (
    <div className="library-toolbar">
      <div className="library-toolbar-head">
        {newFilter}
        {shortlist && (
          <OverflowMenu label={t("shell.more")}>{shortlist}</OverflowMenu>
        )}
      </div>
      {finders}
    </div>
  );
}

function TypePills({
  cut,
  items,
  countable,
}: Readonly<{
  cut: LibraryCut;
  items: readonly LibraryItem[];
  countable: boolean;
}>) {
  const t = useT();
  const counts = pillCounts(items, cut.archived, !countable);
  const projects =
    cut.type === "projects" ||
    items.some((item) => typeOf(item) === "projects");
  const pills: FilterPill<LibraryType | "all">[] = [
    { value: "all", label: t("filters.library.all"), count: counts?.all },
    ...OBJECT_TABS.map((tab) => ({
      value: tab,
      label: t(TAB_LABEL[tab]),
      count: counts?.[tab],
    })),
    ...(projects
      ? [
          {
            value: "projects" as const,
            label: t("lists.type.project"),
            count: counts?.projects,
          },
        ]
      : []),
  ];
  return (
    <FilterPills
      pills={pills}
      value={cut.type}
      onChange={(next) =>
        replaceDial(TYPE_PARAM, next === "all" ? undefined : next)
      }
      label={t("filters.objectLabel")}
    />
  );
}

/**
 * The one primary verb. With a record type pressed it starts that type's
 * filter; otherwise it asks which records first, because a filter is built
 * over the fields of one type and there is no neutral one to start on.
 */
function NewFilterAction({ type }: Readonly<{ type: LibraryType | "all" }>) {
  const t = useT();
  const open = (tab: ObjectTab) => navigate({ screen: "filters", id: tab });
  if (type !== "all" && type !== "projects") {
    return (
      <Button
        variant="primary"
        className="library-new"
        onClick={() => open(type)}
      >
        <Plus aria-hidden />
        {t(NEW_FILTER_LABEL[type])}
      </Button>
    );
  }
  return (
    <Popover
      variant="primary"
      className="library-new"
      label={
        <>
          <Plus aria-hidden />
          {t("filters.library.newFilter")}
        </>
      }
    >
      <p className="t-caption">{t("filters.library.whichRecords")}</p>
      <div className="library-types">
        {OBJECT_TABS.map((tab) => (
          <Button key={tab} onClick={() => open(tab)}>
            {t(TAB_LABEL[tab])}
          </Button>
        ))}
      </div>
    </Popover>
  );
}

/** Nothing left under the cut, and the one way to undo the part that hid it. */
function NoHits({ cut }: Readonly<{ cut: LibraryCut }>) {
  const t = useT();
  const q = cut.q.trim();
  const searched = q !== "";
  let said: string;
  if (searched) {
    said = t("filters.library.noHits", { q });
  } else if (cut.type !== "all") {
    said = t(NO_TYPE_HITS_LABEL[cut.type]);
  } else {
    return null;
  }
  return (
    <EmptyState>
      <p>{said}</p>
      <Button
        variant="link"
        onClick={() =>
          replaceDial(searched ? SEARCH_PARAM : TYPE_PARAM, undefined)
        }
      >
        {t(searched ? "filters.library.clearSearch" : "list.showAll")}
      </Button>
    </EmptyState>
  );
}

const GROUP_TITLE: Record<LibraryTableGroup, MessageKey> = {
  mine: "filters.library.mine",
  shared: "filters.library.shared",
  views: "filters.library.views",
};

/**
 * One group: its rows, and what its reads have not answered yet. A deleted
 * view hands focus to the group holding every saved view.
 */
function LibraryGroupPanel({
  group,
  rows,
  reads,
  narrowed,
  captionOf,
  folded,
  returnFocusTo,
}: Readonly<{
  group: LibraryTableGroup;
  rows: readonly LibraryItem[];
  reads: LibraryReads;
  narrowed: boolean;
  captionOf: (item: LibraryItem) => string;
  folded: boolean;
  returnFocusTo: () => HTMLElement | null;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const title = t(GROUP_TITLE[group]);
  const viewsHere = group !== "shared";
  const listsHere = group !== "views";
  const pending =
    (viewsHere && reads.viewsPending) || (listsHere && reads.listsPending);
  const settled =
    !pending &&
    !(viewsHere && reads.viewsFailed) &&
    !(listsHere && reads.listsFailed);
  const counted =
    settled && !reads.truncated && !(listsHere && reads.listsHeld) ? (
      <span className="t-num">{formatNumber(rows.length, locale)}</span>
    ) : undefined;
  return (
    <Panel title={title} titleAction={counted}>
      <PanelBody>
        {viewsHere && reads.viewsFailed && (
          <ErrorLine actions={<RetryButton onRetry={reads.retryViews} />}>
            {t("filters.library.viewsFailed")}
          </ErrorLine>
        )}
        {listsHere && reads.listsFailed && (
          <ErrorLine actions={<RetryButton onRetry={reads.retryLists} />}>
            {t("filters.library.listsFailed")}
          </ErrorLine>
        )}
        {rows.length > 0 && (
          <LibraryTable
            label={title}
            items={rows}
            group={group}
            captionOf={captionOf}
            folded={folded}
            returnFocusTo={returnFocusTo}
          />
        )}
        {pending && (
          <PendingBody label={t("filters.library.loading")} lines={6} />
        )}
        {settled && rows.length === 0 && !narrowed && (
          <EmptyState>
            {t(
              group === "shared"
                ? "filters.library.sharedEmpty"
                : "filters.library.mineEmpty",
            )}
          </EmptyState>
        )}
      </PanelBody>
    </Panel>
  );
}

function RetryButton({ onRetry }: Readonly<{ onRetry: () => void }>) {
  const t = useT();
  return (
    <Button variant="link" onClick={onRetry}>
      {t("common.retry")}
    </Button>
  );
}

/**
 * `#/filters/views` and `#/filters/lists` land on their group. Focus moves one
 * commit after the rows have drawn, so the shell's scroll memory has already
 * put the new page at its top and the move is not undone under the reader.
 */
function useAnchorFocus(
  anchor: LibraryAnchor | undefined,
  listsOn: boolean,
  settled: boolean,
): Readonly<{
  mine: RefObject<HTMLDivElement | null>;
  shared: RefObject<HTMLDivElement | null>;
}> {
  const mine = useRef<HTMLDivElement | null>(null);
  const shared = useRef<HTMLDivElement | null>(null);
  const [ready, setReady] = useState(false);
  const landed = useRef(false);
  useEffect(() => {
    if (settled) {
      setReady(true);
    }
  }, [settled]);
  useEffect(() => {
    if (!ready || anchor === undefined || landed.current) {
      return;
    }
    landed.current = true;
    const target = anchor === "lists" && listsOn ? shared : mine;
    target.current?.focus();
  }, [ready, anchor, listsOn]);
  return { mine, shared };
}

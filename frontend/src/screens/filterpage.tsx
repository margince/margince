// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A focused filter page (AC-filters-and-views-1/3/4/5): a new filter on one
// record type, or a saved view or a Live List's filter opened for editing.
//
// It starts calm — the page's name, the record type, and two ways in — and
// grows only as the reader builds. The count and the rows arrive with the
// first complete condition; before then there is nothing to count, and the
// line under the editor says what would make there be.

import { useState } from "react";
import { useGuardedLeave } from "../app/unsaved";
import { SegmentedControl } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { Panel, PanelBody } from "../design-system/panel";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import {
  PREVIEW_PAGE,
  useFilterPreview,
  useFilterVocabulary,
} from "./filterdata";
import { useFilterDraft } from "./filterdraft";
import { FilterEditor, vocabularyState } from "./filtereditor";
import { useFilterExport } from "./filterexport";
import { FilterFoot } from "./filterfoot";
import { FocusedHead, FocusedPending } from "./filterhead";
import {
  EditingListNotice,
  SaveToListAction,
  useOpenListFromAddress,
} from "./filterlistedit";
import { FilterMatches, useResultsShown } from "./filtermatches";
import {
  type FocusedFiltersAddress,
  fallbackTitleOf,
  OBJECT_TABS,
  type ObjectTab,
  RESOURCE_OF,
  SWITCH_CLEARS_LABEL,
  TAB_LABEL,
  tabOfListType,
  UNIT_LABEL,
  VIEW_OF,
} from "./filtersaddress";
import { type SavedKind, SaveFilterModal } from "./filtersave";
import { useList } from "./lists.queries";
import { filterTreeOf, useSavedViews } from "./savedviews.queries";
import {
  fieldsNamed,
  isComplete,
  type Node,
  newGroup,
  rootGroup,
} from "./segmentpredicate";
import "./filters.css";

/** What the page is editing, beyond the record type it reads. */
type Editing =
  | Readonly<{ kind: "new" }>
  | Readonly<{ kind: "view" }>
  /** `listId` is absent when the list cannot be edited on any tab. */
  | Readonly<{ kind: "list"; listId?: string }>;

export function FilterPage({
  address,
}: Readonly<{ address: FocusedFiltersAddress }>) {
  const t = useT();
  switch (address.kind) {
    case "listFilter":
      return <ListFilterLoader address={address} />;
    case "view":
      return <OpenedViewLoader address={address} />;
    default:
      // Keyed by record type: conditions name one type's fields, so another
      // type starts a page of its own rather than inheriting them.
      return (
        <FilterBuildPage
          key={address.tab}
          tab={address.tab}
          title={t(fallbackTitleOf(address))}
          editing={{ kind: "new" }}
        />
      );
  }
}

/**
 * `#/filters/<tab>/<viewId>`: the saved view's filter, once it has been read.
 * The views are read afresh, because a cached list may predate the view; one
 * that is gone or unreadable opens an empty editor once that read has settled.
 * The page waits until then, so nothing the reader types can be replaced by
 * the view arriving late.
 *
 * It waits for the FIRST read only. Every later one — a save here refreshing
 * the views, a refetch on focus — happens under a page the reader is typing
 * in, and waiting on it would unmount the page and its draft with it.
 */
function OpenedViewLoader({
  address,
}: Readonly<{ address: Extract<FocusedFiltersAddress, { kind: "view" }> }>) {
  const t = useT();
  const views = useSavedViews(VIEW_OF[address.tab], true);
  const view = views.data?.find((row) => row.id === address.viewId);
  const tree = view ? filterTreeOf(view) : null;
  const fallback = t(fallbackTitleOf(address));
  if (tree === null && !views.isFetchedAfterMount) {
    return <FocusedPending title={fallback} label={t("common.loading")} />;
  }
  return (
    <FilterBuildPage
      tab={address.tab}
      title={view?.name ?? fallback}
      editing={{ kind: "view" }}
      initial={tree}
    />
  );
}

/**
 * `#/filters/list/<id>`: the list's filter, on the tab of its record type
 * once the list has been read.
 */
function ListFilterLoader({
  address,
}: Readonly<{
  address: Extract<FocusedFiltersAddress, { kind: "listFilter" }>;
}>) {
  const t = useT();
  const list = useList(address.listId);
  const fallback = t(fallbackTitleOf(address));
  if (list.isPending) {
    return <FocusedPending title={fallback} label={t("lists.loading")} />;
  }
  const tab = list.data ? tabOfListType(list.data.entity_type) : undefined;
  return (
    <FilterBuildPage
      tab={tab ?? "contacts"}
      title={list.data?.name ?? fallback}
      editing={{ kind: "list", listId: tab ? address.listId : undefined }}
    />
  );
}

function FilterBuildPage({
  tab,
  title,
  editing,
  initial = null,
}: Readonly<{
  tab: ObjectTab;
  /** The page's h1: the new filter's type, or the name of what was opened. */
  title: string;
  editing: Editing;
  initial?: Node | null;
}>) {
  const t = useT();
  const toast = useToast();
  const [draft, dispatch] = useFilterDraft(() =>
    initial ? rootGroup(initial) : newGroup("and"),
  );
  // The request's page size, which the table's own size dial sets.
  const [limit, setLimit] = useState(PREVIEW_PAGE);
  const [switchTo, setSwitchTo] = useState<ObjectTab | null>(null);
  const [saving, setSaving] = useState(false);
  const { edited, opening } = useOpenListFromAddress(
    editing.kind === "list" ? editing.listId : undefined,
    tab,
    (tree) => dispatch({ type: "reset", tree }),
  );
  const resource = RESOURCE_OF[tab];
  const vocabulary = useFilterVocabulary(resource);
  const preview = useFilterPreview(resource, draft.tree, limit);
  const shown = useResultsShown(draft.tree, preview);
  const exportRun = useFilterExport();
  const complete = isComplete(draft.tree);
  // A half-built condition is nothing to lose; a complete one is a filter the
  // reader could have kept. Every way off the page that already asked, or
  // kept the filter, leaves through here so the guard does not ask again.
  const leave = useGuardedLeave(editing.kind === "new" && complete);

  if (opening) {
    return <FocusedPending title={title} label={t("lists.loading")} />;
  }

  const empty = draft.tree.children.length === 0;
  const records = t(UNIT_LABEL[tab]);
  const switchTab = (next: ObjectTab) => {
    if (next === tab) {
      return;
    }
    // A PUSH: the record type is what the reader came here for, so Back
    // returns to the last one they looked at. Conditions name one type's
    // fields, so leaving a type with some on screen asks first.
    if (empty) {
      leave({ screen: "filters", id: next });
      return;
    }
    setSwitchTo(next);
  };
  const saved = (kind: SavedKind, id: string, name: string) => {
    setSaving(false);
    if (kind === "view") {
      toast.show(t("filters.viewSaved"));
      leave({ screen: "filters", id: tab, id2: id });
      return;
    }
    toast.show(t("filters.listCreated", { name }));
    leave({ screen: "lists", id });
  };

  return (
    <div className="wrap filters-screen">
      <FocusedHead
        title={title}
        control={
          editing.kind === "new" && (
            <SegmentedControl
              options={OBJECT_TABS}
              value={tab}
              onChange={switchTab}
              labels={{
                contacts: t(TAB_LABEL.contacts),
                companies: t(TAB_LABEL.companies),
                deals: t(TAB_LABEL.deals),
                leads: t(TAB_LABEL.leads),
              }}
              label={t("filters.objectLabel")}
            />
          )
        }
      />
      {edited && <EditingListNotice edited={edited} />}
      <Panel
        title={
          editing.kind === "new"
            ? t("filters.find", { records })
            : t("filters.builderTitle")
        }
        // Once there is a filter to keep. Every verb here sends the tree,
        // and an unfinished one is a tree the engine refuses.
        footer={
          complete ? (
            <FilterFoot
              resource={resource}
              tree={draft.tree}
              onSave={() => setSaving(true)}
              exportRun={exportRun}
              saveTo={
                edited ? (
                  <SaveToListAction edited={edited} tree={draft.tree} />
                ) : undefined
              }
            />
          ) : undefined
        }
      >
        <PanelBody>
          <FilterEditor
            resource={resource}
            draft={draft}
            dispatch={dispatch}
            vocabulary={vocabulary}
          />
        </PanelBody>
      </Panel>
      {/* Only beside an editor the reader can use: while the fields are
          read, or could not be, there is no condition for it to ask for. */}
      {vocabularyState(vocabulary) === "ready" && (
        <WaitingLine
          shown={shown}
          empty={empty}
          complete={complete}
          records={records}
        />
      )}
      {shown && (
        <FilterMatches
          preview={preview}
          tab={tab}
          fields={vocabulary.data?.fields ?? []}
          named={fieldsNamed(draft.tree)}
          limit={limit}
          onLimit={setLimit}
        />
      )}
      <SaveFilterModal
        open={saving}
        onClose={() => setSaving(false)}
        tab={tab}
        tree={draft.tree}
        onSaved={saved}
      />
      <ConfirmModal
        open={switchTo !== null}
        onClose={() => setSwitchTo(null)}
        title={t("filters.switch.title", {
          records: switchTo ? t(UNIT_LABEL[switchTo]) : "",
        })}
        confirmLabel={t("filters.switch.confirm")}
        onConfirm={() => {
          if (switchTo) {
            leave({ screen: "filters", id: switchTo });
          }
          setSwitchTo(null);
        }}
      >
        <p>{t(SWITCH_CLEARS_LABEL[tab])}</p>
      </ConfirmModal>
    </div>
  );
}

/**
 * The line under the editor while the results are not the whole story: what
 * would bring a count, or that the count on screen waits for the condition
 * the reader is still writing.
 */
function WaitingLine({
  shown,
  empty,
  complete,
  records,
}: Readonly<{
  shown: boolean;
  empty: boolean;
  complete: boolean;
  records: string;
}>) {
  const t = useT();
  if (shown && complete) {
    return null;
  }
  return (
    <p className="t-sub filters-hint">
      {shown
        ? t("filters.hint.update")
        : t(empty ? "filters.hint.start" : "filters.hint.finish", { records })}
    </p>
  );
}

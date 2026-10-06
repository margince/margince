// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The Filters & views destination. An address opens the library, one list,
// or the builder a human authors a dynamic filter on (AC-filters-and-views-1/3/4).
//
// The builder hosts three things and owns none of them. The object control picks which
// record type's vocabulary to read; the builder draws the tree; the count comes
// back from the preview. What this file adds is the wiring and one judgement — how
// to report a count that is one edit behind, which is the honest state of any live
// recount over a moving table.

import { useEffect, useState } from "react";
import { navigate, navigateReplacing } from "../app/router";
import { Badge, PendingBody, SegmentedControl } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { type SectionState, SurfaceState } from "../design-system/surfacestate";
import { useT } from "../i18n";
import { QueryStates, useMe } from "./common";
import { FilterBuilder } from "./filterbuilder";
import {
  useFilterPreview,
  useFilterVocabulary,
  type VocabularyField,
} from "./filterdata";
import { canExportFilter, ExportFilterMenu } from "./filterexport";
import { SaveFilterListAction } from "./filterlist";
import {
  EditingListNotice,
  SaveToListAction,
  useOpenListFromAddress,
} from "./filterlistedit";
import { MatchCount } from "./filtermatchcount";
import {
  PlainWordsFilter,
  type UnusedPhrase,
  UnusedPhrases,
} from "./filterpropose";
import { FilterResults } from "./filterresults";
import {
  filtersAddressOf,
  MATCH_LABEL,
  OBJECT_TABS,
  type ObjectTab,
  RESOURCE_OF,
  TAB_LABEL,
  tabOfListType,
  UNIT_LABEL,
  VIEW_OF,
  type ViewResource,
} from "./filtersaddress";
import { FiltersLibrary } from "./filterslibrary";
import { ListScreen } from "./listpage";
import { useList } from "./lists.queries";
import "./filters.css";
import { LoadFilterViewMenu, SaveFilterViewAction } from "./savedviews";
import { filterTreeOf, useSavedViews } from "./savedviews.queries";
import { fieldsNamed, type Node, newGroup } from "./segmentpredicate";

export function FiltersScreen({
  id,
  list,
  view,
}: Readonly<{ id?: string; list?: string; view?: string }>) {
  const t = useT();
  const me = useMe();
  // `#/lists` names no list, and the lists a reader can find live in the
  // library's Shared group. A redirect, so Back never lands here again.
  useEffect(() => {
    if (list === "") {
      navigateReplacing({ screen: "filters", id: "lists" });
    }
  }, [list]);
  if (list === "") {
    return null;
  }
  // Nothing draws until the session says whether lists are on: the lists-off
  // library and the builder would each flash before the page they are not.
  if (me.isPending) {
    return (
      <div className="wrap">
        <PendingBody label={t("filters.library.loading")} lines={6} />
      </div>
    );
  }
  // One opened list. It loads with the library that opens it, as one chunk.
  if (list !== undefined) {
    return <ListScreen listID={list} />;
  }
  const address = filtersAddressOf({ id, id2: view });
  switch (address.kind) {
    case "library":
      return <FiltersLibrary anchor={address.anchor} />;
    case "listFilter":
      return (
        <div className="wrap">
          <ListFilterBuild listID={address.listId} />
        </div>
      );
    case "view":
      return (
        <div className="wrap">
          <FilterBuildScreen tab={address.tab} view={address.viewId} />
        </div>
      );
    default:
      return (
        <div className="wrap">
          <FilterBuildScreen tab={address.tab} />
        </div>
      );
  }
}

/**
 * `#/filters/list/<id>`: the builder on that Live List's filter, on the tab of
 * its record type once the list has been read.
 */
function ListFilterBuild({ listID }: Readonly<{ listID: string }>) {
  const list = useList(listID);
  if (list.isPending) {
    return null;
  }
  const tab = list.data ? tabOfListType(list.data.entity_type) : undefined;
  return (
    <FilterBuildScreen
      tab={tab ?? "contacts"}
      editList={tab ? listID : undefined}
    />
  );
}

function FilterBuildScreen({
  tab,
  view,
  editList,
}: Readonly<{ tab: ObjectTab; view?: string; editList?: string }>) {
  const t = useT();
  // A fresh tree per object, because a clause naming a contact's field means
  // nothing on a deal — carrying the tree across would offer the human a filter
  // the new vocabulary refuses.
  const [tree, setTree] = useState<Node>(() => newGroup("and"));
  // What the last plain-words proposal could not use, kept until the next one
  // or until the reader puts it away.
  const [unused, setUnused] = useState<readonly UnusedPhrase[]>([]);
  const openingView = useOpenViewFromAddress(VIEW_OF[tab], view, setTree);
  const { edited, opening: openingList } = useOpenListFromAddress(
    editList,
    tab,
    setTree,
  );
  const opening = openingView || openingList;

  const resource = RESOURCE_OF[tab];
  const vocabulary = useFilterVocabulary(resource);
  const preview = useFilterPreview(resource, tree);

  const switchTab = (next: ObjectTab) => {
    // A PUSH: the object being filtered is what the reader came here for, so
    // Back returns to the last one they looked at. The tree resets because the
    // address changed, not beside it — `id` is what this screen renders from.
    navigate({ screen: "filters", id: next });
    setTree(newGroup("and"));
    setUnused([]);
  };

  return (
    <div className="filters-screen">
      {/* The object control alone. The page's name and its subtitle belong to
          the shell's page head, which names every rail destination — a screen
          that printed them again would put two page titles in one document, and
          a reader navigating by heading could not tell which was the page. */}
      <div className="filters-object-row">
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
      </div>

      {edited && <EditingListNotice edited={edited} />}
      <Panel
        title={t("filters.builderTitle")}
        // Below the builder, not beside the count: the export takes the filter
        // as its argument, so it belongs after the thing it reads — and a
        // refusal is a sentence, which no header row has width for. It also
        // takes the FILTER vocabulary's word for the object rather than the
        // view rail's, because `/exports` enumerates `contact`, the same as the
        // preview it has to agree with.
        //
        // The slot is filled only when the menu will draw something. Panel
        // draws its actions band for any element it is handed, and a React
        // element that renders `null` is still an element — so an unfinished
        // filter, which has nothing to export, left an empty band ruled under
        // the builder. `canExportFilter` is the menu's own condition, asked
        // here rather than restated.
        actions={
          canExportFilter(tree) ? (
            <ExportFilterMenu resource={resource} tree={tree} />
          ) : undefined
        }
      >
        {/* A toolbar row under the head, not inside it: the band is one line
            that never wraps, and this cluster — a count, a badge and two view
            menus — measured 614px at a 390px viewport. */}
        <PanelBody className="filters-count-row">
          <MatchCount
            label={MATCH_LABEL[tab]}
            count={preview.data?.match_count}
            stale={preview.isFetching}
            failed={preview.isError}
          />
          {/* AC-1's live badge: what this filter IS, not what it is doing. A
              dynamic list recomputes on every event, and that is the property
              a reader needs before trusting a count at all. */}
          <Badge tone="accent">{t("filters.dynamic")}</Badge>
          <LoadFilterViewMenu resource={VIEW_OF[tab]} onLoad={setTree} />
          <SaveFilterViewAction resource={VIEW_OF[tab]} tree={tree} />
          {edited && <SaveToListAction edited={edited} tree={tree} />}
          <SaveFilterListAction resource={resource} tree={tree} />
        </PanelBody>
        <PanelBody>
          {/* Keyed by object so a sentence typed for contacts is not offered
              to deals, whose vocabulary it was never read against. */}
          <PlainWordsFilter
            key={resource}
            resource={resource}
            tree={tree}
            onApply={(next, phrases) => {
              if (next !== null) {
                setTree(next);
              }
              setUnused(phrases);
            }}
          />
        </PanelBody>
        <PanelBody>
          <SurfaceState
            state={
              opening
                ? "loading"
                : vocabularyState(vocabulary.isPending, vocabulary.isError)
            }
            emptyLabel={t("filters.noFields")}
            loadingLabel={t("filters.loadingVocabulary")}
            // The builder that lands here is a condition row plus its verbs.
            loadingLines={4}
          >
            <FilterBuilder
              tree={tree}
              onChange={setTree}
              fields={vocabulary.data?.fields ?? []}
            />
          </SurfaceState>
        </PanelBody>
        {unused.length > 0 && (
          <PanelBody>
            <UnusedPhrases
              unused={unused}
              fields={vocabulary.data?.fields ?? []}
              onDismiss={() => setUnused([])}
            />
          </PanelBody>
        )}
      </Panel>

      <PreviewSection
        preview={preview}
        tab={tab}
        fields={vocabulary.data?.fields ?? []}
        named={fieldsNamed(tree)}
      />
    </div>
  );
}

/**
 * Loads the saved view the address names into the builder, and answers true
 * while it is still being read. The builder stays a skeleton until then, so
 * nothing the reader types can be replaced by the view arriving late. The
 * views are read afresh, because a cached list may predate the view; one that
 * is gone or unreadable leaves an empty builder once that read has settled.
 * The screen remounts per address, so this loads at most once.
 */
function useOpenViewFromAddress(
  resource: ViewResource,
  viewId: string | undefined,
  load: (tree: Node) => void,
): boolean {
  const views = useSavedViews(resource, viewId !== undefined);
  const [opened, setOpened] = useState(false);
  if (viewId === undefined || opened) {
    return false;
  }
  const found = views.data?.find((row) => row.id === viewId);
  const tree = found ? filterTreeOf(found) : null;
  if (tree) {
    setOpened(true);
    load(tree);
    return false;
  }
  const settled = !views.isFetching && (views.isSuccess || views.isError);
  if (settled) {
    setOpened(true);
  }
  return !settled;
}

/**
 * The rows behind the count, the reason there are none, or nothing at all.
 *
 * Three outcomes and they are three different statements, which is why the
 * absent one cannot stand in for the other two. Nothing at all, while no
 * complete clause has been written: an empty table there would say "no records
 * match this filter" about a filter nobody wrote. The rows, once the server has
 * answered. And a failure with its reason and a retry, when the preview was
 * refused — a read seat is refused every `POST /filters/preview` before RBAC is
 * consulted, so a reader who may read the vocabulary and build a clause can
 * still never get a count, and the reason is the only thing that tells them so.
 * The header row beside the count has no width for a sentence, so it lands here.
 */
function PreviewSection({
  preview,
  tab,
  fields,
  named,
}: Readonly<{
  preview: ReturnType<typeof useFilterPreview>;
  tab: ObjectTab;
  fields: readonly VocabularyField[];
  named: readonly string[];
}>) {
  const t = useT();
  if (preview.isError) {
    return (
      <Panel title={t("filters.resultsTitle")}>
        {/* The house spelling of a failed read: the headline and the server's
            own cause in one live region, with the retry beside it. */}
        <PanelBody>
          <QueryStates query={preview} pendingLabel={t("filters.resultsTitle")}>
            {null}
          </QueryStates>
        </PanelBody>
      </Panel>
    );
  }
  if (preview.data === undefined) {
    return null;
  }
  return (
    <Panel title={t("filters.resultsTitle")}>
      <PanelBody>
        <FilterResults
          preview={preview.data}
          fields={fields}
          named={named}
          unit={t(UNIT_LABEL[tab])}
          // Per object, so switching tabs does not hand a deal's table the
          // widths a reader dragged for a contact's columns.
          widthsKey={`filter-preview-${tab}`}
          pending={preview.isFetching}
        />
      </PanelBody>
    </Panel>
  );
}

/**
 * The vocabulary read's three outcomes as a surface state.
 *
 * An error is `unavailable` rather than `empty`: an empty field list would tell a
 * reader this record type has nothing to filter on, which is a different and false
 * statement — and the endpoint 404s rather than answering an empty set, so a
 * genuine empty cannot arrive.
 */
function vocabularyState(pending: boolean, failed: boolean): SectionState {
  if (pending) {
    return "loading";
  }
  return failed ? "unavailable" : "ready";
}

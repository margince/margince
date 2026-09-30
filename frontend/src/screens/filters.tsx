// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The Filters & views screen (AC-filters-and-views-1/3/4): the surface a
// human authors a dynamic filter on, which until now existed only through the API.
//
// It hosts three things and owns none of them. The object control picks which
// record type's vocabulary to read; the builder draws the tree; the count comes
// back from the preview. What this file adds is the wiring and one judgement — how
// to report a count that is one edit behind, which is the honest state of any live
// recount over a moving table.

import { useState } from "react";
import { navigate } from "../app/router";
import { Badge, SegmentedControl } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { Panel, PanelBody } from "../design-system/panel";
import { type SectionState, SurfaceState } from "../design-system/surfacestate";
import { formatNumber } from "../format/format";
import { type PluralBase, useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { QueryStates } from "./common";
import { FilterBuilder } from "./filterbuilder";
import {
  type FilterResource,
  useFilterPreview,
  useFilterVocabulary,
  type VocabularyField,
} from "./filterdata";
import { canExportFilter, ExportFilterMenu } from "./filterexport";
import { SaveFilterListAction } from "./filterlist";
import {
  PlainWordsFilter,
  type UnusedPhrase,
  UnusedPhrases,
} from "./filterpropose";
import { FilterResults } from "./filterresults";
import { ListLibrary } from "./listlibrary";
import { ListScreen } from "./listpage";
import { useListsAvailable } from "./lists.queries";
import { MyViews } from "./myviews";
import "./filters.css";
import {
  filterTreeOf,
  LoadFilterViewMenu,
  SaveFilterViewAction,
  useSavedViews,
  type ViewResource,
} from "./savedviews";
import { fieldsNamed, type Node, newGroup } from "./segmentpredicate";

/**
 * The object tabs, and the record type each reads.
 *
 * The tab says "Contacts" and the vocabulary says "contact": the wire's word and
 * the product's word differ, and this is the one place that correspondence is
 * written down rather than assumed at each call site.
 */
const OBJECT_TABS = ["contacts", "companies", "deals", "leads"] as const;
type ObjectTab = (typeof OBJECT_TABS)[number];

const RESOURCE_OF: Record<ObjectTab, FilterResource> = {
  contacts: "contact",
  companies: "company",
  deals: "deal",
  leads: "lead",
};

const TAB_LABEL: Record<ObjectTab, MessageKey> = {
  contacts: "filters.tab.contacts",
  companies: "filters.tab.companies",
  deals: "filters.tab.deals",
  leads: "filters.tab.leads",
};

const MATCH_LABEL: Record<ObjectTab, PluralBase> = {
  contacts: "filters.matchContacts",
  companies: "filters.matchCompanies",
  deals: "filters.matchDeals",
  leads: "filters.matchLeads",
};

/** The plural noun the results table counts and names its empty state by. */
const UNIT_LABEL: Record<ObjectTab, MessageKey> = {
  contacts: "unit.contacts",
  companies: "unit.companies",
  deals: "unit.deals",
  leads: "unit.leads",
};

/**
 * The same three objects again, as `/views` spells them.
 *
 * A third spelling, and it is not a mistake to fix here: `/filters/*` takes
 * `contact` and `/views` takes `contacts`, both enumerated in the contract. So this
 * screen is where the two vocabularies meet, and the correspondence is written
 * down once — beside `RESOURCE_OF`, so a reader sees both mappings together —
 * rather than derived at each call site by adding an "s".
 */
const VIEW_OF: Record<ObjectTab, ViewResource> = {
  contacts: "contacts",
  companies: "companies",
  deals: "deals",
  leads: "leads",
};

/** A resource this screen can address, or the default when the route names none. */
function tabFromRoute(id: string | undefined): ObjectTab {
  return OBJECT_TABS.find((tab) => tab === id) ?? "contacts";
}

/**
 * The library's three sections, while lists are switched on: the reader's own
 * views, the shared views, and the builder a Live List is made in. Each is an
 * address — `#/filters/lists`, `#/filters/views`, or an object tab for Build —
 * so Back returns to the section the reader left.
 */
const SECTIONS = ["views", "lists", "build"] as const;
type Section = (typeof SECTIONS)[number];

function sectionFromRoute(id: string | undefined): Section {
  return id === "lists" || id === "views" ? id : "build";
}

export function FiltersScreen({
  id,
  list,
  view,
}: Readonly<{ id?: string; list?: string; view?: string }>) {
  const t = useT();
  const listsOn = useListsAvailable();
  // One opened list. It loads with the library that opens it, as one chunk.
  if (list !== undefined) {
    return <ListScreen listID={list} />;
  }
  if (!listsOn) {
    return (
      <div className="wrap">
        <FilterBuildScreen id={id} view={view} />
      </div>
    );
  }
  const section = sectionFromRoute(id);
  return (
    <div className="wrap filters-screen">
      <div className="filters-object-row">
        <SegmentedControl
          options={SECTIONS}
          value={section}
          onChange={(next) =>
            navigate({
              screen: "filters",
              id: next === "build" ? undefined : next,
            })
          }
          labels={{
            views: t("lists.section.views"),
            lists: t("lists.section.lists"),
            build: t("lists.section.build"),
          }}
          label={t("lists.section.label")}
        />
      </div>
      {section === "lists" && <ListLibrary />}
      {section === "views" && <MyViews />}
      {section === "build" && <FilterBuildScreen id={id} view={view} />}
    </div>
  );
}

function FilterBuildScreen({
  id,
  view,
}: Readonly<{ id?: string; view?: string }>) {
  const t = useT();
  // The ADDRESS is which object is being filtered. It was read once, on mount,
  // and never written back — so pressing a tab moved the screen and left the
  // URL naming the object the reader had left, which a reload or a Back press
  // then restored over them.
  const tab = tabFromRoute(id);
  // A fresh tree per object, because a clause naming a contact's field means
  // nothing on a deal — carrying the tree across would offer the human a filter
  // the new vocabulary refuses.
  const [tree, setTree] = useState<Node>(() => newGroup("and"));
  // What the last plain-words proposal could not use, kept until the next one
  // or until the reader puts it away.
  const [unused, setUnused] = useState<readonly UnusedPhrase[]>([]);
  const opening = useOpenViewFromAddress(VIEW_OF[tab], view, setTree);

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
            tab={tab}
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
 * The count, and whether it is behind.
 *
 * Four readings, and keeping them apart is the point. A count the server has
 * answered reads plainly. A count being recomputed reads as the LAST answer,
 * marked stale — not as a spinner, because a number that vanishes on every
 * keystroke is harder to read than one that lags a moment. A tree with no
 * complete clause has no count at all, which is different from a count of zero:
 * zero means "nothing matches", and this means "you have not asked yet". And a
 * count the server was asked for and refused says exactly that.
 *
 * The refusal outranks the other three. It is read first because the previous
 * answer survives a failed refetch, so a stale number would otherwise be
 * presented as current, and because "you have not asked yet" over a finished
 * clause blames the reader for the server's refusal.
 */
function MatchCount({
  tab,
  count,
  stale,
  failed,
}: Readonly<{
  tab: ObjectTab;
  count: number | undefined;
  stale: boolean;
  failed: boolean;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  if (failed) {
    // Silent: the results card below carries the reason in an assertive live
    // region, and announcing the same failure twice fragments it.
    return (
      <span className="filters-count">
        <ErrorLine inline standing>
          {t("filters.countUnavailable")}
        </ErrorLine>
      </span>
    );
  }
  if (count === undefined) {
    return <span className="filters-count">{t("filters.noFilterYet")}</span>;
  }
  return (
    <span
      className="filters-count"
      // Spoken, because the count changing is the feedback for every edit — a
      // sighted reader sees the number move and a screen-reader user would
      // otherwise get nothing back from adding a clause.
      role="status"
      aria-busy={stale}
      data-stale={stale ? "true" : undefined}
    >
      {plural(MATCH_LABEL[tab], count, { count: formatNumber(count, locale) })}
    </span>
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

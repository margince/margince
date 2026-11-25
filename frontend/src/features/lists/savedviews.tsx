// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Bookmark } from "lucide-react";
import { useContext, useState } from "react";
import {
  isNarrowed,
  type ListOpening,
  type ListQuery,
  type SavedViewTab,
} from "../../components/listquery";
import {
  matchesView,
  OpenedViewContext,
} from "../../components/listqueryviews";
import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { NamePrompt } from "../../design-system/nameprompt";
import { useToast } from "../../design-system/toast";
import { useT } from "../../i18n";
import type { ViewResource } from "../../lib/filtersaddress";
import { problemMessageOf } from "../../lib/problem";
import {
  type SavedView,
  useSavedViews,
  useSaveView,
  viewNameTaken,
} from "../../lib/savedviews.queries";
import { ManageViewsButton } from "./savedviews.manage";

// A saved view is the reader's own list state, by name: the search, the sort,
// the filters, the archived toggle and the page size they were looking at.
//
// Pressing its tab restores four of those five. The SEARCH is stored and read
// back but not written to the box, so a view naming one is a view whose tab
// lights only while that search is actually typed: the tab is a claim about
// what the list is showing, and it makes no claim it cannot keep.
//
// It is per-user and private (the server stamps owner_id from the caller and
// writes shared_scope 'private'), so nothing here asks who may see it — the
// answer is always "only you", and a picker that implied otherwise would be
// promising a sharing model V1 does not have.

/**
 * The list state a view restores.
 *
 * Deliberately NOT written under the `query.filter` key: the server validates
 * that one as a segment filter TREE (field/op/value, compiled against the
 * resource's engine), and a list's filters are flat `param=value` pairs the
 * list endpoints take directly. Storing them there would be refused as "not a
 * valid filter tree" — and storing a tree here would be a second filter dialect
 * the lists cannot read. So list state lives under its own key and the tree key
 * stays free for the segment builder that owns it.
 */
const LIST_STATE_KEY = "list";

type SavedListState = Pick<
  ListQuery,
  "q" | "sort" | "includeArchived" | "filters"
> & {
  /**
   * The page size the view claims, or absent when the stored blob makes no
   * claim about one.
   *
   * Optional rather than defaulted, because those are different facts and the
   * rail acts on the difference: a view that stored 25 asks for 25, and a view
   * that stored nothing leaves the reader on whatever they had. Substituting a
   * number here made the two indistinguishable, and a view written through
   * `POST /views` with a `list` blob carrying no page size then dropped a
   * reader from 100 rows to the default with no dial moving.
   */
  perPage?: number;
};

/** The saved state of one view, or null when the row predates this shape. */
export function listStateOf(view: SavedView): SavedListState | null {
  // `query` is required by the contract, but a stub, a hand-written row, or a
  // future shape can still hand this an object without it — and a list screen
  // that throws while reading its own tab rail takes the whole screen with it.
  const stored = view.query as Record<string, unknown> | undefined;
  const raw = stored?.[LIST_STATE_KEY];
  if (!raw || typeof raw !== "object") {
    return null;
  }
  const state = raw as Partial<SavedListState>;
  // Every field is checked rather than trusted: a view is stored as an open
  // JSON object, so a row written by an older build (or by hand) can carry any
  // shape at all. The four dials that describe WHICH rows the list holds take a
  // neutral value when the blob omits them, because the list has to send the
  // server something; the page size does not, and stays absent so the rail can
  // tell "asked for the default" from "asked for nothing".
  return {
    q: typeof state.q === "string" ? state.q : "",
    sort: typeof state.sort === "string" ? state.sort : "",
    includeArchived: state.includeArchived === true,
    filters:
      state.filters && typeof state.filters === "object"
        ? Object.fromEntries(
            Object.entries(state.filters).filter(
              ([, value]) => typeof value === "string",
            ),
          )
        : {},
    perPage: typeof state.perPage === "number" ? state.perPage : undefined,
  };
}

/** What a view saves, given the list the reader is looking at now. */
function listStateFrom(query: ListQuery): Record<string, unknown> {
  return {
    [LIST_STATE_KEY]: {
      q: query.q,
      sort: query.sort,
      includeArchived: query.includeArchived,
      filters: query.filters,
      perPage: query.perPage,
    },
  };
}

/**
 * The saved views of one resource, as view tabs the list can render beside its
 * built-in presets.
 *
 * A view whose stored state cannot be read is dropped rather than shown: a tab
 * that lights up and restores nothing is worse than a tab that is not there.
 *
 * Everything the view claims travels with the tab, not just the sort and the
 * filters: the search, the archived toggle and the page size were saved too, and
 * a tab that drops one of them claims a list it is not describing. The id
 * travels for a different reason — it is what still identifies the view after
 * `/views` has re-ordered the rail around a rename.
 *
 * A failed read answers with no tabs, which is indistinguishable from a reader
 * who has saved none. `SavedViewsFailure` says which it was, because a hook can
 * only answer with tabs.
 */
export function useSavedViewTabs(resource: ViewResource): SavedViewTab[] {
  const views = useSavedViews(resource);
  return (views.data ?? []).flatMap((view) => {
    const state = listStateOf(view);
    return state
      ? [
          {
            id: view.id,
            label: view.name,
            q: state.q,
            sort: state.sort,
            filters: state.filters,
            includeArchived: state.includeArchived,
            perPage: state.perPage,
          },
        ]
      : [];
  });
}

/**
 * "Save this view", named through the catalog's one name-and-save dialog
 * (`NamePrompt`) and never one of its own, so every surface asks this question
 * the same way.
 *
 * The list's dials are read at the press: it is the committed render's state
 * that the mutation is given, never a closure the observer might still hold
 * from an earlier one.
 */
function SaveViewButton({
  resource,
  query,
  views,
  asNew,
}: Readonly<{
  resource: ViewResource;
  query: ListQuery;
  views: readonly SavedView[];
  /** Offered beside "Update": the trigger then says the view will be a new one. */
  asNew: boolean;
}>) {
  const t = useT();
  const toast = useToast();
  const { create } = useSaveView();
  const [nameTaken, setNameTaken] = useState(false);

  return (
    <NamePrompt
      trigger={t(asNew ? "filters.saveAsNew" : "views.save")}
      icon={<Bookmark strokeWidth={1.5} aria-hidden="true" />}
      title={t("views.saveTitle")}
      label={t("views.name")}
      confirmLabel={t("views.saveConfirm")}
      pending={create.isPending}
      // `problemMessageOf`, not `problemMessage`: what a failed mutation carries
      // is a ProblemError, and handing that to the body reader yields the
      // generic "request failed" instead of the reason the server gave.
      problem={create.isError ? problemMessageOf(create.error, t) : undefined}
      nameProblem={nameTaken ? t("views.nameTaken") : null}
      onSave={(name, done) => {
        const taken = viewNameTaken(views, name);
        setNameTaken(taken);
        if (taken) {
          return;
        }
        // The promise, not the mutate callback: the saved view makes this
        // button leave, and a callback of an unmounted caller never runs.
        create
          .mutateAsync({ resource, name, query: listStateFrom(query) })
          .then(() => {
            done();
            toast.show(t("filters.viewSaved"));
          })
          // A refusal is drawn from the mutation's own error state above.
          .catch(() => undefined);
      }}
    />
  );
}

/**
 * "Update": writes the list's dials over the view the reader opened and then
 * changed, held to the version that view was read at. Keys the view stores
 * beside its list state are kept.
 */
function UpdateViewButton({
  view,
  query,
}: Readonly<{ view: SavedView; query: ListQuery }>) {
  const t = useT();
  const toast = useToast();
  const { saveQuery } = useSaveView();
  return (
    <Button
      pending={saveQuery.isPending}
      onClick={() =>
        saveQuery
          .mutateAsync({
            id: view.id,
            version: view.version,
            query: { ...view.query, ...listStateFrom(query) },
          })
          .then(
            () => toast.show(t("views.updated", { name: view.name })),
            (error: unknown) =>
              toast.show(problemMessageOf(error, t), {
                tone: "danger",
                sticky: true,
              }),
          )
      }
    >
      {t("views.update", { name: view.name })}
    </Button>
  );
}

/** A failed views read, on the toolbar where it is seen without opening View:
 * an empty rail would claim the reader saved none. */
export function SavedViewsFailure({
  resource,
}: Readonly<{ resource: ViewResource }>) {
  const t = useT();
  // The same query the rail reads, so this costs no second request: one read of
  // /views answers both, and the rail cannot be shown as loaded here and failed
  // there.
  const views = useSavedViews(resource);
  if (!views.isError) {
    return null;
  }
  return (
    <ErrorLine
      actions={
        <Button
          variant="link"
          pending={views.isFetching}
          onClick={() => void views.refetch()}
        >
          {t("common.retry")}
        </Button>
      }
    >
      {t("views.loadFailed")}
    </ErrorLine>
  );
}

/**
 * Save view once the reader has narrowed the list from how it opens. It is not
 * offered while a saved view already holds that list, which would be a second
 * tab of the same rows. A changed view the reader opened can be updated in
 * place, or kept beside a new one.
 */
export function SaveViewAction({
  resource,
  state,
}: Readonly<{
  resource: ViewResource;
  state: ListOpening & Readonly<{ query: ListQuery }>;
}>) {
  const views = useSavedViews(resource);
  const openedId = useContext(OpenedViewContext);
  const saved = views.data ?? [];
  const holdsThisList = (view: SavedView) => {
    const stored = listStateOf(view);
    return stored !== null && matchesView(stored, state.query);
  };
  const unsaved = isNarrowed(state.query, state) && !saved.some(holdsThisList);
  const opened = saved.find(
    (view) => view.id === openedId && listStateOf(view) !== null,
  );
  return (
    <>
      {unsaved && opened && (
        <UpdateViewButton view={opened} query={state.query} />
      )}
      {unsaved && (
        <SaveViewButton
          resource={resource}
          query={state.query}
          views={saved}
          asNew={opened !== undefined}
        />
      )}
      {/* Beside Save, because it is the same set of the reader's own views:
          a tab can be pressed but not named or removed, so this is the only
          place a view that has served its purpose can go. */}
      {views.data && <ManageViewsButton views={views.data} />}
    </>
  );
}

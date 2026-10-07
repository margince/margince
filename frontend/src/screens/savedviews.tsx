// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Bookmark } from "lucide-react";
import { NamePrompt } from "../design-system/nameprompt";
import { SurfaceState } from "../design-system/surfacestate";
import { useT } from "../i18n";
import { problemMessageOf } from "./common";
import type { ViewResource } from "./filtersaddress";
import type { ListQuery, SavedViewTab } from "./listquery";
import { ManageViewsButton } from "./savedviews.manage";
import {
  type SavedView,
  useSavedViews,
  useSaveView,
} from "./savedviews.queries";

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
 * who has saved none. `SaveViewAction` says which it was, because a hook can
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
}: Readonly<{ resource: ViewResource; query: ListQuery }>) {
  const t = useT();
  const { create } = useSaveView();

  return (
    <NamePrompt
      trigger={t("views.save")}
      icon={<Bookmark strokeWidth={1.5} aria-hidden="true" />}
      title={t("views.saveTitle")}
      label={t("views.name")}
      confirmLabel={t("views.saveConfirm")}
      pending={create.isPending}
      // `problemMessageOf`, not `problemMessage`: what a failed mutation carries
      // is a ProblemError, and handing that to the body reader yields the
      // generic "request failed" instead of the reason the server gave.
      problem={create.isError ? problemMessageOf(create.error, t) : undefined}
      onSave={(name, done) =>
        create.mutate(
          { resource, name, query: listStateFrom(query) },
          { onSuccess: done },
        )
      }
    />
  );
}

/**
 * "Save this view" beside a list's own tools, and the one place that says the
 * saved-view rail failed to load.
 *
 * Saving is offered only when the list is actually narrowed. Saving the
 * unfiltered default would create a tab that does what the All tab already
 * does, and a rail of those is how a useful feature becomes clutter.
 *
 * The failure is reported here rather than on the rail because the rail is a
 * row of tabs: with no tabs there is nothing to hang the state on, and an empty
 * rail says "you have saved none" — a claim the screen cannot make when the
 * read failed. This is the control that manages saved views, so it is where a
 * reader looks, and it is `failed` rather than `unavailable` because a retry is
 * offered with it. It is NAMED, because it lands in the list's tools slot beside
 * Columns and Compact: "this section did not load" under a toolbar covering
 * three controls says which one only if the part is named.
 */
export function SaveViewAction({
  resource,
  query,
}: Readonly<{ resource: ViewResource; query: ListQuery }>) {
  const t = useT();
  // The same query the rail reads, so this costs no second request: one read of
  // /views answers both, and the rail cannot be shown as loaded here and failed
  // there.
  const views = useSavedViews(resource);
  const narrowed =
    Boolean(query.q) ||
    Boolean(query.sort) ||
    query.includeArchived ||
    Object.values(query.filters).some(Boolean);
  return (
    <>
      {views.isError && (
        <SurfaceState
          loadingLabel={t("views.rail")}
          state="failed"
          label={t("views.rail")}
          // Read by the `empty` arm alone, which this call never reaches: what
          // there is none of is not a question a failed read has an answer to.
          emptyLabel={t("common.empty")}
          detail={{ onRetry: () => void views.refetch() }}
        >
          {null}
        </SurfaceState>
      )}
      {narrowed && <SaveViewButton resource={resource} query={query} />}
      {/* Beside Save, because it is the same set of the reader's own views:
          a tab can be pressed but not named or removed, so this is the only
          place a view that has served its purpose can go. */}
      {views.data && <ManageViewsButton views={views.data} />}
    </>
  );
}

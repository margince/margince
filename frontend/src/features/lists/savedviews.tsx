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
import { type ViewResource, viewPageHref } from "../../lib/filtersaddress";
import { problemMessageOf } from "../../lib/problem";
import {
  type SavedView,
  useSavedViews,
  useSaveView,
  viewNameTaken,
} from "../../lib/savedviews.queries";
import { ManageViewsButton } from "./savedviews.manage";
import { listStateFrom, viewOpening } from "./savedviews.state";
import { viewNameRefusal } from "./viewnamerefusal";

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
 * The saved views of one resource, as view tabs the list can render beside its
 * built-in presets.
 *
 * A view that opens in the builder (`viewOpening`) cannot be applied here, so
 * its tab links to its page in Filters and views. Any other unreadable view is
 * dropped: a tab that restores nothing is worse than none.
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
  const t = useT();
  const views = useSavedViews(resource);
  return (views.data ?? []).flatMap((view): SavedViewTab[] => {
    const opens = viewOpening(view);
    if (opens?.kind === "list") {
      const { state } = opens;
      return [
        {
          id: view.id,
          label: view.name,
          q: state.q,
          sort: state.sort,
          filters: state.filters,
          includeArchived: state.includeArchived,
          perPage: state.perPage,
          version: view.version,
        },
      ];
    }
    return opens?.kind === "builder"
      ? [
          {
            id: view.id,
            label: view.name,
            q: "",
            sort: "",
            filters: {},
            includeArchived: false,
            version: view.version,
            link: {
              href: viewPageHref(opens.tab, view.id),
              name: t("views.opensInFilters", { name: view.name }),
            },
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
  const refused = viewNameRefusal(create.error, t);

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
      problem={refused.problem}
      nameProblem={nameTaken ? t("views.nameTaken") : refused.nameProblem}
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
 * changed, held to the version they opened it at. A write made since, even one
 * a refetch has brought in, is then refused as a version skew rather than lost.
 * Keys the view stores beside its list state are kept.
 */
function UpdateViewButton({
  view,
  version,
  query,
}: Readonly<{ view: SavedView; version: number; query: ListQuery }>) {
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
            version,
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
  const openedView = useContext(OpenedViewContext);
  if (views.data === undefined) {
    // Until the reader's views are read, no name can be checked against them
    // and no view can be known to hold this list already.
    return null;
  }
  const saved = views.data;
  const holdsThisList = (view: SavedView) => {
    const opens = viewOpening(view);
    return opens?.kind === "list" && matchesView(opens.state, state.query);
  };
  const unsaved = isNarrowed(state.query, state) && !saved.some(holdsThisList);
  const opened = saved.find(
    (view) => view.id === openedView?.id && viewOpening(view)?.kind === "list",
  );
  // Only the version the reader opened may guard the write, never the cache's.
  const openedVersion = openedView?.version;
  return (
    <>
      {unsaved && opened && openedVersion !== undefined && (
        <UpdateViewButton
          view={opened}
          version={openedVersion}
          query={state.query}
        />
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
      <ManageViewsButton views={saved} />
    </>
  );
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// `#/filters/<tab>/<viewId>`: a saved view, opened. It reads first, the filter
// as one sentence above what it selects, and edits in place, saving back over
// the view held to the version it was read at.

import type { QueryObserverResult } from "@tanstack/react-query";
import { Pencil } from "lucide-react";
import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { navigateReplacing } from "../app/router";
import { useGuardedLeave } from "../app/unsaved";
import { Button, OverflowMenu } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { Panel, PanelBody } from "../design-system/panel";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { problemCodeOf, problemMessageOf } from "./common";
import { useFilterVocabulary } from "./filterdata";
import { useFilterDraft, useFirstAnswer, wireSignature } from "./filterdraft";
import {
  FilterEditor,
  useRowsFocus,
  type VocabularyRead,
} from "./filtereditor";
import {
  canExportFilter,
  ExportFilterItems,
  ExportProgress,
  useFilterExport,
} from "./filterexport";
import { FilterFoot } from "./filterfoot";
import {
  BackToLibrary,
  FocusedHead,
  FocusedPending,
  FocusedState,
} from "./filterhead";
import { FilterOutcome } from "./filtermatches";
import { usePlainWords } from "./filterpropose";
import {
  type ObjectTab,
  RESOURCE_OF,
  TAB_LABEL,
  tabOfViewResource,
  UNIT_LABEL,
} from "./filtersaddress";
import { type SavedKind, SaveFilterModal, useLandOnSaved } from "./filtersave";
import { filterSentence, useSentenceWords } from "./filtersentence";
import { useListsAvailable } from "./lists.queries";
import {
  filterStateFrom,
  filterTreeOf,
  openableTree,
  type SavedView,
  useSavedView,
  useSaveView,
} from "./savedviews.queries";
import { type Group, type Node, rootGroup } from "./segmentpredicate";
import { DeleteViewAction, RenameViewAction } from "./viewactions";
import "./filters.css";

type Reread = () => Promise<QueryObserverResult<SavedView>>;

export function OpenedViewPage({
  tab,
  viewId,
}: Readonly<{ tab: ObjectTab; viewId: string }>) {
  const t = useT();
  const read = useSavedView(viewId);
  const vocabulary = useFilterVocabulary(RESOURCE_OF[tab]);
  const opened = useFirstAnswer(read);
  const own = opened ? tabOfViewResource(opened.resource) : undefined;
  const elsewhere = own !== undefined && own !== tab;
  // A view under another record type's address is still that view: the address
  // is put right rather than the view refused, and Back skips the wrong one.
  useEffect(() => {
    if (elsewhere) {
      navigateReplacing({ screen: "filters", id: own, id2: viewId });
    }
  }, [elsewhere, own, viewId]);

  const tree = opened ? openableTree(opened) : null;
  if (read.isError && !opened) {
    return problemCodeOf(read.error) === "not_found" ? (
      <ViewGone />
    ) : (
      <ViewUnread
        error={read.error}
        pending={read.isFetching}
        retry={() => void read.refetch()}
      />
    );
  }
  if (opened && tree === null) {
    return <ViewGone />;
  }
  // The sentence names fields, so the page waits for both rather than reading
  // a view as wire names.
  if (!opened || tree === null || vocabulary.isPending || elsewhere) {
    return (
      <FocusedPending
        title={t("filters.library.kindView")}
        label={t("common.loading")}
      />
    );
  }
  return (
    <OpenedView
      tab={tab}
      first={opened}
      tree={tree}
      vocabulary={vocabulary}
      reread={read.refetch}
    />
  );
}

/** A view deleted, never there, or holding nothing this page can open. */
function ViewGone() {
  const t = useT();
  return (
    <FocusedState
      title={t("filters.library.kindView")}
      sentence={t("filters.view.gone")}
    />
  );
}

/**
 * A read that failed says nothing about whether the view is still there, so
 * the page says why and offers the read again.
 */
function ViewUnread({
  error,
  pending,
  retry,
}: Readonly<{ error: unknown; pending: boolean; retry: () => void }>) {
  const t = useT();
  return (
    <div className="wrap filters-screen">
      <FocusedHead title={t("filters.library.kindView")} />
      <ErrorLine
        error={error}
        actions={
          <Button variant="link" pending={pending} onClick={retry}>
            {t("filters.view.reload")}
          </Button>
        }
      />
      <BackToLibrary />
    </div>
  );
}

/**
 * The view as this page holds it, and the stored filter, in wire form, that
 * the draft is measured against.
 */
type Pin = Readonly<{ view: SavedView; baseline: string }>;

/** What "Save this filter" was opened for, and the tree on screen then. */
type Saving = Readonly<{ keep: SavedKind; tree: Group }>;

function OpenedView({
  tab,
  first,
  tree,
  vocabulary,
  reread,
}: Readonly<{
  tab: ObjectTab;
  first: SavedView;
  tree: Node;
  vocabulary: VocabularyRead;
  reread: Reread;
}>) {
  const t = useT();
  const toast = useToast();
  const sentenceWords = useSentenceWords();
  const listsOn = useListsAvailable();
  const { saveQuery } = useSaveView();
  // Advanced only by this page's own writes: a later read that answers a
  // colleague's version must not become the version a save is held to.
  const [pin, setPin] = useState<Pin>(() => ({
    view: first,
    baseline: wireSignature(tree),
  }));
  const [draft, dispatch] = useFilterDraft(() => rootGroup(tree));
  const [editing, setEditing] = useState(false);
  const [saving, setSaving] = useState<Saving | null>(null);
  const focus = useFoldFocus();
  const onScreen = useOnScreen(draft.tree);
  const resource = RESOURCE_OF[tab];
  const exportRun = useFilterExport();
  const plainWords = usePlainWords({
    resource,
    dispatch,
    onLanded: () => setEditing(true),
  });
  const reload = useReload(reread, (view, fresh) => {
    focus.toRows();
    saveQuery.reset();
    setPin({ view, baseline: wireSignature(fresh) });
    dispatch({ type: "reset", tree: fresh });
  });
  const gone = reload.lost || problemCodeOf(saveQuery.error) === "not_found";
  const changed = wireSignature(draft.tree) !== pin.baseline;
  // Every verb that sends the tree waits for one the engine would accept.
  const complete = canExportFilter(draft.tree);
  const leave = useGuardedLeave(changed && !gone);
  const land = useLandOnSaved(tab, leave);

  if (gone) {
    return <ViewGone />;
  }

  const fold = () => {
    focus.toEdit();
    setEditing(false);
    dispatch({ type: "setWordsOpen", open: false });
  };
  // The foot holds this back while a save is out: resetting that save drops
  // the answer that advances the version, so the next would meet its own write.
  const discard = () => {
    reload.clear();
    saveQuery.reset();
    dispatch({ type: "reset", tree: filterTreeOf(pin.view) ?? tree });
    fold();
  };
  const saveChanges = () => {
    const sent = wireSignature(draft.tree);
    reload.clear();
    saveQuery.mutate(
      {
        id: pin.view.id,
        version: pin.view.version,
        query: filterStateFrom(draft.tree, pin.view.query),
      },
      {
        onSuccess: (view) => {
          setPin({ view, baseline: sent });
          toast.show(t("filters.changesSaved"));
          // A proposal that landed while the save was out is not what was
          // saved: its rows stay open as unsaved rather than fold as kept.
          if (wireSignature(onScreen.current) === sent) {
            dispatch({ type: "keepAll" });
            fold();
          }
        },
      },
    );
  };

  return (
    <div className="wrap filters-screen">
      <FocusedHead
        title={pin.view.name}
        facts={t("filters.view.facts", { records: t(TAB_LABEL[tab]) })}
        actions={
          <OverflowMenu
            label={t("filters.library.rowMore", { name: pin.view.name })}
          >
            <RenameViewAction
              view={pin.view}
              onRenamed={(view) =>
                setPin((was) => ({
                  ...was,
                  view: { ...was.view, name: view.name, version: view.version },
                }))
              }
            />
            {listsOn && complete && (
              <Button
                onClick={() => setSaving({ keep: "list", tree: draft.tree })}
              >
                {t("filters.view.saveAsList")}
              </Button>
            )}
            {complete && (
              <ExportFilterItems
                run={exportRun}
                resource={resource}
                tree={draft.tree}
              />
            )}
            <DeleteViewAction
              view={pin.view}
              onDeleted={() => leave({ screen: "filters" })}
            />
          </OverflowMenu>
        }
      />
      <div className="filters-head-status">
        <ExportProgress run={exportRun} tree={draft.tree} />
      </div>
      <Panel
        title={t("filters.builderTitle")}
        titleAction={
          editing ? undefined : (
            <Button
              ref={focus.edit}
              onClick={() => {
                focus.toRows();
                setEditing(true);
              }}
            >
              <Pencil aria-hidden />
              {t("filters.editConditions")}
            </Button>
          )
        }
        footer={
          editing ? (
            <FilterFoot
              resource={resource}
              tree={draft.tree}
              mode={{
                kind: "view",
                changed,
                onDone: fold,
                onDiscard: discard,
                onSaveChanges: saveChanges,
                saving: saveQuery.isPending,
              }}
              onSave={() => setSaving({ keep: "view", tree: draft.tree })}
              exportRun={exportRun}
              problem={
                <SaveProblem failure={saveQuery.error} reload={reload} />
              }
            />
          ) : undefined
        }
      >
        <PanelBody>
          {editing ? (
            <div ref={focus.rows} inert={saveQuery.isPending}>
              <FilterEditor
                draft={draft}
                dispatch={dispatch}
                vocabulary={vocabulary}
                words={plainWords}
                records={t(UNIT_LABEL[tab])}
              />
            </div>
          ) : (
            <p className="filters-reading">
              {filterSentence(
                draft.tree,
                vocabulary.data?.fields,
                sentenceWords,
              )}
            </p>
          )}
        </PanelBody>
      </Panel>
      <FilterOutcome tab={tab} tree={draft.tree} vocabulary={vocabulary} />
      <SaveFilterModal
        open={saving !== null}
        onClose={() => setSaving(null)}
        tab={tab}
        tree={saving?.tree ?? draft.tree}
        initialKeep={saving?.keep ?? "view"}
        initialName={saving?.keep === "list" ? pin.view.name : ""}
        onSaved={(kind, id, name) => {
          setSaving(null);
          land(kind, id, name);
        }}
      />
    </div>
  );
}

/**
 * Why Save changes was refused. A colleague's change since opening is met
 * with the way to see it; a reload that itself failed says why, and offers
 * itself again.
 */
function SaveProblem({
  failure,
  reload,
}: Readonly<{ failure: unknown; reload: ReturnType<typeof useReload> }>) {
  const t = useT();
  const conflict = problemCodeOf(failure) === "version_skew";
  if (reload.failure !== null || conflict) {
    return (
      <ErrorLine
        inline
        actions={
          <Button
            variant="link"
            pending={reload.pending}
            onClick={reload.reload}
          >
            {t("filters.view.reload")}
          </Button>
        }
      >
        {reload.failure !== null
          ? problemMessageOf(reload.failure, t)
          : t("filters.view.conflict")}
      </ErrorLine>
    );
  }
  return <ErrorLine inline error={failure} />;
}

/**
 * "Reload view": read the view again and start over from what it says now, or
 * learn that it is gone. Its own answer, so a refetch nobody asked for moves
 * nothing on the page.
 */
function useReload(
  reread: Reread,
  onFresh: (view: SavedView, tree: Node) => void,
) {
  const [pending, setPending] = useState(false);
  const [failure, setFailure] = useState<unknown>(null);
  const [lost, setLost] = useState(false);
  const reload = () => {
    setPending(true);
    setFailure(null);
    void reread().then((answer) => {
      setPending(false);
      if (answer.isError && problemCodeOf(answer.error) !== "not_found") {
        setFailure(answer.error);
        return;
      }
      const view = answer.isError ? undefined : answer.data;
      const fresh = view ? openableTree(view) : null;
      if (!view || fresh === null) {
        setLost(true);
        return;
      }
      onFresh(view, fresh);
    });
  };
  return { reload, pending, failure, lost, clear: () => setFailure(null) };
}

/**
 * Focus follows the reader's own presses: into the rows "Edit conditions"
 * opened or "Reload view" redrew, and back to "Edit conditions" when Done,
 * Discard or a save folds them. Rows an answer opens move nothing here; the
 * editor's own arrival rule decides that.
 */
function useFoldFocus() {
  const { rows, focusRows } = useRowsFocus();
  const edit = useRef<HTMLButtonElement>(null);
  const folded = useRef(false);
  useLayoutEffect(() => {
    if (folded.current) {
      folded.current = false;
      edit.current?.focus();
    }
  });
  return {
    edit,
    rows,
    toRows: focusRows,
    toEdit: () => {
      folded.current = true;
    },
  };
}

/**
 * The tree on screen now, for an answer that arrives after the render that
 * asked: the save's own closure holds the tree it sent.
 */
function useOnScreen(tree: Group) {
  const current = useRef(tree);
  useLayoutEffect(() => {
    current.current = tree;
  });
  return current;
}

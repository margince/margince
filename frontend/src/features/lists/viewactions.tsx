// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Renaming and deleting one saved view, each its own dialog. These serve a
// row's ⋯ or a page head, where the view stands alone; ManageViewsList edits in
// place instead, because it is already a dialog listing every view.

import { useState } from "react";
import { Button } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { NameDialog } from "../../design-system/namedialog";
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
import { viewNameRefusal } from "./viewnamerefusal";

/**
 * "Rename": the view's name, prefilled, against the version it was read at.
 * A name another view of `resource` holds is refused, as Manage views refuses
 * it, and nothing is saved until those views are read.
 */
export function RenameViewAction({
  view,
  resource,
  onRenamed,
}: Readonly<{
  view: SavedView;
  resource: ViewResource;
  onRenamed?: (updated: SavedView) => void;
}>) {
  const t = useT();
  const toast = useToast();
  const { rename } = useSaveView();
  const siblings = useSavedViews(resource);
  const [open, setOpen] = useState(false);
  const [nameTaken, setNameTaken] = useState(false);
  // The view as the dialog opened on it: a refetch while the dialog is open
  // must not lend a stale name a newer If-Match.
  const [read, setRead] = useState(view);
  const refused = viewNameRefusal(rename.error, t);
  const unread = siblings.data === undefined;
  const readProblem = siblings.isError
    ? problemMessageOf(siblings.error, t)
    : null;
  return (
    <>
      <Button
        onClick={() => {
          setRead(view);
          rename.reset();
          setNameTaken(false);
          if (siblings.isError) {
            // refetch resolves with the error in state, which readProblem shows
            void siblings.refetch();
          }
          setOpen(true);
        }}
      >
        {t("views.rename")}
      </Button>
      <NameDialog
        open={open}
        onClose={() => setOpen(false)}
        title={t("views.renameTitle")}
        label={t("views.name")}
        initial={read.name}
        confirmLabel={t("views.rename")}
        pending={rename.isPending || (unread && siblings.isPending)}
        problem={rename.isError ? refused.problem : readProblem}
        nameProblem={nameTaken ? t("views.nameTaken") : refused.nameProblem}
        onSave={(name) => {
          // Names not read, or read before a failed refetch, would pass any name.
          if (siblings.data === undefined || siblings.isError) {
            return;
          }
          const taken = viewNameTaken(siblings.data, name, view.id);
          setNameTaken(taken);
          if (taken) {
            return;
          }
          rename.mutate(
            { id: view.id, name, version: read.version },
            {
              onSuccess: (updated) => {
                setOpen(false);
                toast.show(t("views.renamed"));
                onRenamed?.(updated);
              },
            },
          );
        }}
      />
    </>
  );
}

/**
 * "Delete view": asks first, naming the view, and says no record changes. A
 * caller whose row goes with the view names where focus lands instead.
 */
export function DeleteViewAction({
  view,
  onDeleted,
  returnFocusTo,
}: Readonly<{
  view: SavedView;
  onDeleted?: () => void;
  returnFocusTo?: () => HTMLElement | null;
}>) {
  const t = useT();
  const toast = useToast();
  const { remove } = useSaveView();
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button
        variant="danger"
        onClick={() => {
          remove.reset();
          setOpen(true);
        }}
      >
        {t("views.deleteConfirm")}
      </Button>
      <ConfirmModal
        open={open}
        onClose={() => setOpen(false)}
        title={t("views.deleteTitle")}
        confirmLabel={t("views.deleteConfirm")}
        confirmVariant="danger"
        returnFocusTo={returnFocusTo}
        pending={remove.isPending}
        error={remove.isError ? problemMessageOf(remove.error, t) : null}
        // mutateAsync: the refreshed read drops this row and its observer before
        // a per-call callback could run. The dialog shows a refusal from isError.
        onConfirm={() =>
          remove.mutateAsync(view.id).then(
            () => {
              setOpen(false);
              toast.show(t("views.deleted", { name: view.name }));
              onDeleted?.();
            },
            () => undefined,
          )
        }
      >
        <p>{t("views.deleteBody", { name: view.name })}</p>
      </ConfirmModal>
    </>
  );
}

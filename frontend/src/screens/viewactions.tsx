// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Renaming and deleting one saved view, each its own dialog. These serve a
// row's ⋯ or a page head, where the view stands alone; ManageViewsList edits in
// place instead, because it is already a dialog listing every view.

import { useState } from "react";
import { Button, Field, TextInput } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { problemMessageOf } from "./common";
import { type SavedView, useSaveView } from "./savedviews.queries";

/** "Rename": the view's name, prefilled, against the version it was read at. */
export function RenameViewAction({
  view,
  onRenamed,
}: Readonly<{ view: SavedView; onRenamed?: (updated: SavedView) => void }>) {
  const t = useT();
  const toast = useToast();
  const { rename } = useSaveView();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState(view.name);
  // The version the prefilled name came from: a refetch while the dialog is
  // open must not lend a stale name a newer If-Match.
  const [readAt, setReadAt] = useState(view.version);
  return (
    <>
      <Button
        onClick={() => {
          setName(view.name);
          setReadAt(view.version);
          rename.reset();
          setOpen(true);
        }}
      >
        {t("views.rename")}
      </Button>
      <ConfirmModal
        open={open}
        onClose={() => setOpen(false)}
        title={t("views.renameTitle")}
        confirmLabel={t("views.rename")}
        confirmDisabled={name.trim() === ""}
        pending={rename.isPending}
        error={rename.isError ? problemMessageOf(rename.error, t) : null}
        onConfirm={() =>
          rename.mutate(
            { id: view.id, name: name.trim(), version: readAt },
            {
              onSuccess: (updated) => {
                setOpen(false);
                toast.show(t("views.renamed"));
                onRenamed?.(updated);
              },
            },
          )
        }
      >
        <Field label={t("views.name")}>
          {(control) => (
            <TextInput
              {...control}
              value={name}
              onChange={(event) => setName(event.target.value)}
            />
          )}
        </Field>
      </ConfirmModal>
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

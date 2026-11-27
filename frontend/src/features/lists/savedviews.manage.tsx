// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId, useState } from "react";
import { Button, Field, Modal, TextInput } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { useSinglePress } from "../../design-system/presslatch";
import { useFieldFaults } from "../../design-system/savepage";
import { useLocale, useT } from "../../i18n";
import { blankRequired, problemMessageOf } from "../../lib/problem";
import {
  type SavedView,
  useSaveView,
  viewNameTaken,
} from "../../lib/savedviews.queries";
import { viewNameRefusal } from "./viewnamerefusal";
import "./savedviews.css";

// Renaming and deleting the reader's own saved views. One dialog listing every
// view of the resource, each row opening in place into the one question it is
// asked, so a rename or a delete never stacks a second dialog on the first.

// What one row is doing: showing the view, asking for its new name, or asking
// whether to delete it. One row at a time, so two half-answered questions can
// never stand open in the same list.
type RowMode =
  // The version the name was read at: a refetch while the row is open must not
  // lend that name a newer If-Match.
  | Readonly<{ kind: "rename"; id: string; name: string; version: number }>
  | Readonly<{ kind: "delete"; id: string }>
  | null;

export function ManageViewsButton({
  views,
}: Readonly<{ views: readonly SavedView[] }>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const headingId = useId();
  // Nothing to manage offers no button; but a dialog whose last view was
  // just deleted stays open to say so, rather than vanishing under the reader.
  if (views.length === 0 && !open) {
    return null;
  }
  return (
    <>
      <Button onClick={() => setOpen(true)}>{t("views.manage")}</Button>
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        labelledBy={headingId}
        intent="form"
      >
        <Heading size="large" id={headingId} className="t-h2 modal-title">
          {t("views.rail")}
        </Heading>
        {open && <ManageViewsList views={views} />}
      </Modal>
    </>
  );
}

function ManageViewsList({ views }: Readonly<{ views: readonly SavedView[] }>) {
  const t = useT();
  const { rename, remove } = useSaveView();
  const { locale } = useLocale();
  const singlePress = useSinglePress(rename.isPending);
  const [mode, setMode] = useState<RowMode>(null);
  const [faults, faultsRoot] = useFieldFaults();
  const nameLabel = t("views.name");
  // The failure belongs to the row that asked, and is cleared by the next
  // question: a refusal left under another row would name the wrong view.
  const settle = () => {
    rename.reset();
    remove.reset();
  };
  const ask = (next: RowMode) => {
    settle();
    faults.show(new Map());
    setMode(next);
  };
  if (views.length === 0) {
    return <p className="surfacestate-empty">{t("views.none")}</p>;
  }
  const renameRefused = viewNameRefusal(rename.error, t);
  const problem = renameRefused.nameProblem
    ? remove.error
    : (rename.error ?? remove.error);
  return (
    <ul className="savedviews-manage">
      {views.map((view) => (
        <li key={view.id} className="savedviews-manage-row">
          {mode?.id === view.id && mode.kind === "rename" ? (
            <form
              className="savedviews-manage-edit"
              ref={faultsRoot}
              noValidate
              onSubmit={singlePress((event) => {
                event.preventDefault();
                const name = mode.name.trim();
                // Another view's name is refused here, as Save view refuses it.
                const refused = viewNameTaken(views, name, view.id)
                  ? new Map([["name", t("views.nameTaken")]])
                  : blankRequired(locale, { name: [nameLabel, mode.name] });
                if (faults.show(refused)) {
                  return;
                }
                rename.mutate(
                  { id: view.id, name, version: mode.version },
                  {
                    onSuccess: () => setMode(null),
                    onError: (error) => {
                      const refused = viewNameRefusal(error, t);
                      if (refused.nameProblem)
                        faults.show(new Map([["name", refused.nameProblem]]));
                    },
                  },
                );
              })}
            >
              <Field label={nameLabel} required error={faults.of("name")}>
                {(control) => (
                  <TextInput
                    {...control}
                    value={mode.name}
                    onChange={(event) => {
                      setMode({ ...mode, name: event.target.value });
                      faults.edited("name");
                    }}
                  />
                )}
              </Field>
              <Button
                type="submit"
                variant="primary"
                pending={rename.isPending}
              >
                {t("views.saveConfirm")}
              </Button>
              <Button onClick={() => ask(null)} disabled={rename.isPending}>
                {t("create.cancel")}
              </Button>
            </form>
          ) : mode?.id === view.id && mode.kind === "delete" ? (
            <div className="savedviews-manage-edit">
              <p>{t("views.deleteAsk", { name: view.name })}</p>
              <Button
                variant="danger"
                pending={remove.isPending}
                onClick={() =>
                  remove.mutate(view.id, { onSuccess: () => setMode(null) })
                }
              >
                {t("views.deleteConfirm")}
              </Button>
              <Button onClick={() => ask(null)} disabled={remove.isPending}>
                {t("create.cancel")}
              </Button>
            </div>
          ) : (
            <>
              <span className="savedviews-manage-name">{view.name}</span>
              <Button
                variant="ghost"
                aria-label={t("views.renameNamed", { name: view.name })}
                onClick={() =>
                  ask({
                    kind: "rename",
                    id: view.id,
                    name: view.name,
                    version: view.version,
                  })
                }
              >
                {t("views.rename")}
              </Button>
              <Button
                variant="ghost"
                aria-label={t("views.deleteNamed", { name: view.name })}
                onClick={() => ask({ kind: "delete", id: view.id })}
              >
                {t("views.delete")}
              </Button>
            </>
          )}
          {mode?.id === view.id && problem && (
            <ErrorLine>{problemMessageOf(problem, t)}</ErrorLine>
          )}
        </li>
      ))}
    </ul>
  );
}

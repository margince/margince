// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch } from "../api/version";
import { Button, Field, Modal, TextInput } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import { useT } from "../i18n";
import { problemMessageOf, throwProblem } from "./common";
import type { ViewResource } from "./savedviews";
import "./savedviews.css";

// The writes a saved view takes, and the dialog that manages the reader's own.
// Beside savedviews.tsx rather than in it, which reads and restores them.

type SavedView = components["schemas"]["SavedView"];

/** The cache key every read of a resource's saved views shares. */
export function savedViewsKey(
  resource: ViewResource,
): readonly ["views", ViewResource] {
  return ["views", resource];
}

/**
 * Save the current list as a named view, rename one, and remove one that has
 * served its purpose.
 *
 * All three invalidate the resource's view list, so the tab rail is whatever
 * the server holds rather than a local copy that drifts from it.
 */
export function useSaveView(resource: ViewResource) {
  const client = useQueryClient();
  const invalidate = () =>
    client.invalidateQueries({ queryKey: savedViewsKey(resource) });

  // The blob is the caller's, not this hook's: a list saves its dials under one
  // key and the segment builder saves a tree under another, and both go through
  // ONE write so there is one place that stamps the resource and invalidates the
  // rail. A second mutation per shape is how the two would drift.
  const create = useMutation({
    mutationFn: async (
      input: Readonly<{ name: string; query: Record<string, unknown> }>,
    ) => {
      const { data, error } = await api.POST("/views", {
        body: {
          resource,
          name: input.name,
          query: input.query,
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: invalidate,
  });

  // The version the row was read at rides as If-Match, so a rename made in
  // another tab since this list was read is refused rather than overwritten.
  const rename = useMutation({
    mutationFn: async (
      input: Readonly<{ id: string; name: string; version: number }>,
    ) => {
      const { data, error } = await api.PATCH("/views/{id}", {
        params: { path: { id: input.id }, ...ifMatch(input.version) },
        body: { name: input.name },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: invalidate,
  });

  const remove = useMutation({
    mutationFn: async (id: string) => {
      const { error } = await api.DELETE("/views/{id}", {
        params: { path: { id } },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: invalidate,
  });

  return { create, rename, remove };
}

// Renaming and deleting the reader's own saved views. One dialog listing every
// view of the resource, each row opening in place into the one question it is
// asked, so a rename or a delete never stacks a second dialog on the first.

// What one row is doing: showing the view, asking for its new name, or asking
// whether to delete it. One row at a time, so two half-answered questions can
// never stand open in the same list.
type RowMode =
  | Readonly<{ kind: "rename"; id: string; name: string }>
  | Readonly<{ kind: "delete"; id: string }>
  | null;

export function ManageViewsButton({
  resource,
  views,
}: Readonly<{ resource: ViewResource; views: readonly SavedView[] }>) {
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
      <Modal open={open} onClose={() => setOpen(false)} labelledBy={headingId}>
        <Heading size="large" id={headingId} className="t-h2 modal-title">
          {t("views.rail")}
        </Heading>
        {open && <ManageViewsList resource={resource} views={views} />}
      </Modal>
    </>
  );
}

function ManageViewsList({
  resource,
  views,
}: Readonly<{ resource: ViewResource; views: readonly SavedView[] }>) {
  const t = useT();
  const { rename, remove } = useSaveView(resource);
  const [mode, setMode] = useState<RowMode>(null);
  // The failure belongs to the row that asked, and is cleared by the next
  // question: a refusal left under another row would name the wrong view.
  const settle = () => {
    rename.reset();
    remove.reset();
  };
  const ask = (next: RowMode) => {
    settle();
    setMode(next);
  };
  if (views.length === 0) {
    return <p className="surfacestate-empty">{t("views.none")}</p>;
  }
  const problem = rename.error ?? remove.error;
  return (
    <ul className="savedviews-manage">
      {views.map((view) => (
        <li key={view.id} className="savedviews-manage-row">
          {mode?.id === view.id && mode.kind === "rename" ? (
            <form
              className="savedviews-manage-edit"
              onSubmit={(event) => {
                event.preventDefault();
                const name = mode.name.trim();
                if (name === "") {
                  return;
                }
                rename.mutate(
                  { id: view.id, name, version: view.version },
                  { onSuccess: () => setMode(null) },
                );
              }}
            >
              <Field label={t("views.name")}>
                {(control) => (
                  <TextInput
                    {...control}
                    value={mode.name}
                    onChange={(event) =>
                      setMode({ ...mode, name: event.target.value })
                    }
                  />
                )}
              </Field>
              <Button
                type="submit"
                variant="primary"
                pending={rename.isPending}
                disabled={mode.name.trim() === ""}
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
                  ask({ kind: "rename", id: view.id, name: view.name })
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

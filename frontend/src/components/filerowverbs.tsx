// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type QueryKey,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { type ReactNode, useRef, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button, Field, OverflowMenu, TextInput } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { useFocusHandoff } from "../design-system/focushandoff";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { fileListKeys, homeOf } from "../lib/attachmentkeys";
import { attachmentTitle } from "../lib/attachmentlink";
import { labelFor } from "../lib/labelfor";
import { problemMessageOf, unwrap } from "../lib/problem";
import { CATEGORY_KEYS, UPLOADABLE_CATEGORIES } from "./adddocument.choices";

type Attachment = components["schemas"]["Attachment"];
type Patch = components["schemas"]["UpdateAttachmentMetadataRequest"];
type DocState = NonNullable<Attachment["doc_state"]>;
type Category = NonNullable<Patch["category"]>;

const STATES: Record<DocState, MessageKey> = {
  draft: "docs.state.draft",
  current: "docs.state.current",
  final: "docs.state.final",
  superseded: "docs.state.superseded",
};
const TITLE_MAX = 120;

/** A file that came with a message belongs to that message, so only its other uploads are edited or deleted here. */
export function isUploadedFile(file: Attachment): boolean {
  return file.entity_type !== "activity";
}

/**
 * The row menu of a file on a record's Documents list: Edit and Delete for an
 * upload, plus whatever the page adds (the deal's Hide). The caller draws it
 * only for a reader who may write the record.
 */
export function FileVerbs({
  file,
  focusLanding = () => null,
  extra,
}: Readonly<{
  file: Attachment;
  /** Takes focus when the row the menu stood in goes away. */
  focusLanding?: () => HTMLElement | null;
  /** Further menu items for a page's own verbs. */
  extra?: ReactNode;
}>) {
  const t = useT();
  const upload = isUploadedFile(file);
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const [editing, setEditing] = useState(false);
  const remove = useRemoveFile(file);
  const side = useRef<HTMLSpanElement | null>(null);
  // A ref, not state: the dialog reads it as it closes, before a re-render.
  const deleted = useRef(false);
  useFocusHandoff(side, focusLanding);
  if (!upload && !extra) return null;
  return (
    <span className="cell-actions" ref={side}>
      <OverflowMenu
        label={t("table.rowActions", { name: attachmentTitle(file) })}
      >
        {extra}
        {upload ? (
          <Button variant="ghost" onClick={() => setEditing(true)}>
            {t("files.edit")}
          </Button>
        ) : null}
        {upload ? (
          <Button
            variant="ghost"
            onClick={() => {
              deleted.current = false;
              setConfirmingDelete(true);
            }}
          >
            {t("files.delete")}
          </Button>
        ) : null}
      </OverflowMenu>
      {editing ? (
        <EditFileDialog file={file} onClose={() => setEditing(false)} />
      ) : null}
      <ConfirmModal
        open={confirmingDelete}
        onClose={() => setConfirmingDelete(false)}
        returnFocusTo={() => (deleted.current ? focusLanding() : null)}
        title={t("files.deleteTitle", { name: attachmentTitle(file) })}
        confirmLabel={t("files.delete")}
        confirmVariant="danger"
        pending={remove.isPending}
        error={remove.isError ? problemMessageOf(remove.error, t) : null}
        onConfirm={() =>
          remove.mutate(undefined, {
            onSuccess: () => {
              deleted.current = true;
              setConfirmingDelete(false);
            },
          })
        }
      >
        <p>
          {t(
            file.entity_type === "deal"
              ? "files.deleteBody"
              : "files.deleteBodyRecord",
          )}
        </p>
      </ConfirmModal>
    </span>
  );
}

// A removal and an edit leave the same lists stale, rooms included.
function useRefreshFile(file: Attachment) {
  const queryClient = useQueryClient();
  return () =>
    Promise.all(
      fileListKeys(homeOf(file), "removed").map((queryKey: QueryKey) =>
        queryClient.invalidateQueries({ queryKey }),
      ),
    );
}

function useRemoveFile(file: Attachment) {
  const t = useT();
  const refresh = useRefreshFile(file);
  return useMutation({
    mutationFn: async () => {
      unwrap(
        await api.DELETE("/attachments/{id}", {
          params: { path: { id: file.id } },
        }),
        t,
      );
    },
    onSuccess: refresh,
  });
}

// Only changed fields are sent. A cleared title is a null, which the server
// reads as "go back to the filename".
function changesOf(
  file: Attachment,
  draft: { title: string; category?: Category; state: DocState },
): Patch {
  const patch: Patch = {};
  const title = draft.title.trim();
  if (title !== (file.title ?? "")) patch.title = title === "" ? null : title;
  if (draft.category && draft.category !== (file.category ?? "other")) {
    patch.category = draft.category;
  }
  if (draft.state !== (file.doc_state ?? "current")) {
    patch.doc_state = draft.state;
  }
  return patch;
}

function EditFileDialog({
  file,
  onClose,
}: Readonly<{ file: Attachment; onClose: () => void }>) {
  const t = useT();
  const refresh = useRefreshFile(file);
  // An upload filed as Other arrives with no category, so absence is choosable.
  const choosable =
    !file.category ||
    (UPLOADABLE_CATEGORIES as readonly string[]).includes(file.category);
  const [title, setTitle] = useState(file.title ?? "");
  const [category, setCategory] = useState(
    (file.category as Category | undefined) ?? "other",
  );
  const [state, setState] = useState<DocState>(file.doc_state ?? "current");
  const save = useMutation({
    mutationFn: async (body: Patch) => {
      unwrap(
        await api.PATCH("/attachments/{id}/metadata", {
          params: { path: { id: file.id } },
          body,
        }),
        t,
      );
    },
    onSuccess: async () => {
      await refresh();
      onClose();
    },
  });
  const patch = changesOf(file, {
    title,
    category: choosable ? category : undefined,
    state,
  });
  return (
    <ConfirmModal
      open
      intent="form"
      onClose={onClose}
      title={t("files.editTitle", { name: attachmentTitle(file) })}
      confirmLabel={t("files.save")}
      pending={save.isPending}
      error={save.isError ? problemMessageOf(save.error, t) : null}
      onConfirm={() =>
        Object.keys(patch).length === 0 ? onClose() : save.mutate(patch)
      }
    >
      <Field label={t("docs.add.name")} hint={t("docs.add.nameHint")}>
        {(control) => (
          <TextInput
            {...control}
            value={title}
            maxLength={TITLE_MAX}
            onChange={(event) => setTitle(event.target.value)}
          />
        )}
      </Field>
      {choosable ? (
        <Field label={t("docs.add.category")}>
          {(control) => (
            <Select
              {...control}
              value={category}
              onChange={(picked) => setCategory(picked as Category)}
              options={UPLOADABLE_CATEGORIES.map((key) => ({
                value: key,
                label: t(labelFor(CATEGORY_KEYS, key)),
              }))}
            />
          )}
        </Field>
      ) : null}
      <Field label={t("files.editState")}>
        {(control) => (
          <Select
            {...control}
            value={state}
            onChange={(picked) => setState(picked as DocState)}
            options={(Object.keys(STATES) as DocState[]).map((key) => ({
              value: key,
              label: t(labelFor(STATES, key)),
            }))}
          />
        )}
      </Field>
    </ConfirmModal>
  );
}

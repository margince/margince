// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// "Add to Shortlist" on a record page: pick a Shortlist of this record's type
// — or name a new one — and say why. It goes through the Shortlist's one
// membership writer, the same one the list page and a bulk change use.

import { useState } from "react";
import { Button, Field, Textarea, TextInput } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import { problemMessageOf } from "./common";
import {
  type ListRecordType,
  useChangeMember,
  useCreateList,
  useLists,
  useListsAvailable,
} from "./lists.queries";

/** The Select value that means "a Shortlist named in the box below". */
const NEW_LIST = "__new__";

type AddInput = Readonly<{
  choice: string;
  newName: string;
  note: string;
}>;

/**
 * The verb, in the record page's overflow menu. Draws nothing while lists are
 * switched off, so the menu never offers a door that answers 404.
 */
export function AddToShortlistAction({
  entityType,
  entityId,
}: Readonly<{ entityType: ListRecordType; entityId: string }>) {
  const t = useT();
  const available = useListsAvailable();
  const [open, setOpen] = useState(false);
  if (!available) {
    return null;
  }
  return (
    <>
      <Button onClick={() => setOpen(true)}>{t("lists.addToShortlist")}</Button>
      {open && (
        <AddToShortlistDialog
          entityType={entityType}
          entityId={entityId}
          onClose={() => setOpen(false)}
        />
      )}
    </>
  );
}

function AddToShortlistDialog({
  entityType,
  entityId,
  onClose,
}: Readonly<{
  entityType: ListRecordType;
  entityId: string;
  onClose: () => void;
}>) {
  const t = useT();
  const shortlists = useLists({ entityType, listType: "static" });
  const create = useCreateList();
  const change = useChangeMember();
  const [choice, setChoice] = useState("");
  const [newName, setNewName] = useState("");
  const [note, setNote] = useState("");
  const editable = (shortlists.data?.data ?? []).filter(
    (list) => list.can_edit && !list.archived_at,
  );
  const failure = create.error ?? change.error;

  const add = async (input: AddInput) => {
    const listId =
      input.choice === NEW_LIST
        ? (
            await create.mutateAsync({
              name: input.newName.trim(),
              entityType,
              listType: "static",
            })
          ).id
        : input.choice;
    await change.mutateAsync({
      listId,
      entityType,
      entityId,
      note: input.note,
    });
    onClose();
  };

  return (
    <ConfirmModal
      open
      onClose={onClose}
      title={t("lists.addToShortlist")}
      confirmLabel={t("lists.add")}
      confirmDisabled={
        choice === "" || (choice === NEW_LIST && newName.trim() === "")
      }
      pending={create.isPending || change.isPending}
      error={failure ? problemMessageOf(failure, t) : null}
      onConfirm={() => {
        // A refusal is drawn from the mutations' own error state above; the
        // rejected promise has nothing left to say.
        add({ choice, newName, note }).catch(() => undefined);
      }}
    >
      <Field label={t("lists.shortlist")}>
        {(control) => (
          <Select
            {...control}
            value={choice}
            onChange={setChoice}
            placeholder={t("lists.pickShortlist")}
            options={[
              ...editable.map((list) => ({ value: list.id, label: list.name })),
              { value: NEW_LIST, label: t("lists.newShortlistOption") },
            ]}
          />
        )}
      </Field>
      {choice === NEW_LIST && (
        <Field label={t("lists.name")}>
          {(control) => (
            <TextInput
              {...control}
              value={newName}
              onChange={(event) => setNewName(event.target.value)}
            />
          )}
        </Field>
      )}
      <Field label={t("lists.note")} hint={t("lists.noteHint")}>
        {(control) => (
          <Textarea
            {...control}
            value={note}
            maxLength={500}
            onChange={(event) => setNote(event.target.value)}
          />
        )}
      </Field>
    </ConfirmModal>
  );
}

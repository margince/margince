// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// "Add to Shortlist" on a record page: pick a Shortlist of this record's type
// — or name a new one — and say why. It goes through the Shortlist's one
// membership writer, the same one the list page and a bulk change use.

import { useState } from "react";
import { ActionRow } from "../design-system/actionrow";
import { AppLink } from "../design-system/applink";
import { Button, Field, Textarea, TextInput } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import { useCanWrite } from "../lib/capability";
import {
  type ListRecordType,
  useChangeMember,
  useCreateList,
  useLists,
  useListsAvailable,
} from "../lib/lists.queries";
import { problemMessageOf } from "../lib/problem";
import { routeHash } from "../lib/router";

/** The Select value that means "a Shortlist named in the box below". */
const NEW_LIST = "__new__";

const ALL_LISTS_HREF = routeHash({ screen: "filters", id: "lists" });

type AddInput = Readonly<{
  choice: string;
  newName: string;
  note: string;
}>;

export function useMayAddToShortlist(): boolean {
  const available = useListsAvailable();
  const granted = useCanWrite("list", "update");
  return available && granted;
}

/**
 * The verb, in a record page's header and in `RecordListsWays`. Draws nothing
 * unless `useMayAddToShortlist`, so no page offers a door the server refuses.
 */
export function AddToShortlistAction({
  entityType,
  entityId,
  reasonId,
}: Readonly<{
  entityType: ListRecordType;
  entityId: string;
  // The page's sentence for a record that takes no changes, by id.
  reasonId?: string;
}>) {
  const t = useT();
  const mayAdd = useMayAddToShortlist();
  const [open, setOpen] = useState(false);
  if (!mayAdd) {
    return null;
  }
  return (
    <>
      <Button reasonId={reasonId} onClick={() => setOpen(true)}>
        {t("lists.addToShortlist")}
      </Button>
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

/**
 * The verbs of a record page's Lists section, wherever the page draws it: the
 * header's Add to Shortlist, and the way to every list the reader can find.
 */
export function RecordListsWays({
  entityType,
  entityId,
}: Readonly<{ entityType: ListRecordType; entityId: string }>) {
  const t = useT();
  return (
    <ActionRow>
      <AddToShortlistAction entityType={entityType} entityId={entityId} />
      <AppLink className="link-button" href={ALL_LISTS_HREF}>
        {t("lists.record.allLists")}
      </AppLink>
    </ActionRow>
  );
}

/**
 * The dialog itself. `newName` opens it on "A new Shortlist…" with that name
 * filled in, for a caller that already knows what the reader wants to call it.
 */
export function AddToShortlistDialog({
  entityType,
  entityId,
  onClose,
  newName: named = "",
}: Readonly<{
  entityType: ListRecordType;
  entityId: string;
  onClose: () => void;
  newName?: string;
}>) {
  const t = useT();
  const shortlists = useLists({ entityType, listType: "static" });
  const create = useCreateList();
  const change = useChangeMember();
  const [choice, setChoice] = useState(named === "" ? "" : NEW_LIST);
  const [newName, setNewName] = useState(named);
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
      intent="form"
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

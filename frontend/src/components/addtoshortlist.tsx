// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// "Add to Shortlist" on a record page: pick a Shortlist of this record's type
// — or name a new one — and say why. It goes through the Shortlist's one
// membership writer, the same one the list page and a bulk change use.

import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { ActionRow } from "../design-system/actionrow";
import { AppLink } from "../design-system/applink";
import { Button, Field, Textarea, TextInput } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { useFieldFaults } from "../design-system/savepage";
import { Select } from "../design-system/select";
import { useLocale, useT } from "../i18n";
import { recordListsKey } from "../lib/activitykeys";
import { useCanWrite } from "../lib/capability";
import {
  type ListRecordType,
  useChangeMember,
  useCreateList,
  useLists,
  useListsAvailable,
  type useRecordLists,
} from "../lib/lists.queries";
import { fieldCheckSentence, refusalSentences } from "../lib/problem";
import { routeHash } from "../lib/router";

/** The Select value that means "a Shortlist named in the box below". */
const NEW_LIST = "__new__";

// The dialog's fields, by the names its refusals carry.
const LIST = "list";
const NAME = "name";

const ALL_LISTS_HREF = routeHash({ screen: "filters", id: "lists" });

type RecordListsPage = NonNullable<ReturnType<typeof useRecordLists>["data"]>;

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
 * The verb, in a record page's header and in `RecordListsActions`. Draws
 * nothing unless lists are on and a full seat holds the `list:update` grant.
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

export function RecordListsActions({
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
  const { locale } = useLocale();
  const client = useQueryClient();
  const mayCreate = useCanWrite("list", "create");
  const shortlists = useLists({ entityType, listType: "static" });
  const create = useCreateList();
  const change = useChangeMember();
  const [choice, setChoice] = useState(named === "" ? "" : NEW_LIST);
  const [newName, setNewName] = useState(named);
  const [note, setNote] = useState("");
  const [faults, faultsRoot] = useFieldFaults<HTMLDivElement>();
  const editable = (shortlists.data?.data ?? []).filter(
    (list) => list.can_edit && !list.archived_at,
  );
  // Read from the record page's own lists block, never fetched for this mark.
  const onLists = new Set(
    (
      client.getQueryData<RecordListsPage>(recordListsKey(entityType, entityId))
        ?.data ?? []
    ).map((list) => list.id),
  );
  const listLabel = t("lists.shortlist");
  const nameLabel = t("lists.name");
  const memberRefusal = (error: unknown) =>
    refusalSentences(
      error,
      t,
      {},
      { conflict: { field: LIST, sentence: t("lists.alreadyOnShortlist") } },
    );
  const nameRefusal = (error: unknown) =>
    refusalSentences(error, t, { [NAME]: nameLabel });

  function checked(input: AddInput): ReadonlyMap<string, string> {
    const checks = new Map<string, string>();
    if (input.choice === "") {
      checks.set(LIST, fieldCheckSentence("required", locale, listLabel));
    }
    if (input.choice === NEW_LIST && input.newName.trim() === "") {
      checks.set(NAME, fieldCheckSentence("required", locale, nameLabel));
    }
    return checks;
  }

  const edited = (field: string) => {
    faults.edited(field);
    create.reset();
    change.reset();
  };

  const add = async (input: AddInput) => {
    const listId =
      input.choice === NEW_LIST
        ? (
            await create.mutateAsync(
              {
                name: input.newName.trim(),
                entityType,
                listType: "static",
              },
              {
                onError: (error) => faults.show(nameRefusal(error).byField),
              },
            )
          ).id
        : input.choice;
    await change.mutateAsync(
      { listId, entityType, entityId, note: input.note },
      { onError: (error) => faults.show(memberRefusal(error).byField) },
    );
    onClose();
  };

  return (
    <ConfirmModal
      open
      onClose={onClose}
      intent="form"
      title={t("lists.addToShortlist")}
      confirmLabel={t("lists.add")}
      pending={create.isPending || change.isPending}
      error={memberRefusal(change.error).rest ?? nameRefusal(create.error).rest}
      onConfirm={() => {
        const input = { choice, newName, note };
        if (faults.show(checked(input))) return;
        // A refusal is drawn from the mutations' own error state above; the
        // rejected promise has nothing left to say.
        add(input).catch(() => undefined);
      }}
    >
      <div className="form-stack" ref={faultsRoot}>
        <Field label={listLabel} error={faults.of(LIST)}>
          {(control) => (
            <Select
              {...control}
              value={choice}
              onChange={(next) => {
                setChoice(next);
                edited(LIST);
              }}
              placeholder={t("lists.pickShortlist")}
              options={[
                ...editable.map((list) => ({
                  value: list.id,
                  label: onLists.has(list.id)
                    ? t("lists.alreadyOnOption", { name: list.name })
                    : list.name,
                })),
                ...(mayCreate
                  ? [{ value: NEW_LIST, label: t("lists.newShortlistOption") }]
                  : []),
              ]}
            />
          )}
        </Field>
        {choice === NEW_LIST && (
          <Field label={nameLabel} error={faults.of(NAME)}>
            {(control) => (
              <TextInput
                {...control}
                value={newName}
                onChange={(event) => {
                  setNewName(event.target.value);
                  edited(NAME);
                }}
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
      </div>
    </ConfirmModal>
  );
}

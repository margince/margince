// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useRef, useState } from "react";
import {
  type ListOpener,
  ListPopover,
  type ListPopoverOption,
} from "../design-system/listpopover";
import { useT } from "../i18n";
import { problemMessageOf } from "../lib/problem";
import { useMe } from "../lib/session";
import { useRecordOwnerOptions } from "./assigneepicker";
import { useUpdateRecord } from "./edit";
import { RosterPartialNote, useRosterPartial } from "./roster";

/** A record as the list opened on it, which the write is pinned to. */
export type OpenedRecord = Readonly<
  Record<string, unknown> & { id: string; version?: number }
>;

type OwnedRecord = OpenedRecord & Readonly<{ owner_id?: string | null }>;

// The option id of "nobody": no seat id can be this word.
const UNASSIGNED = "unassigned";

type OwnerPickProps = Readonly<{
  /** Its owner is marked, and a pick of them writes nothing. */
  record: OwnedRecord;
  assign: (ownerId: string, opened: OpenedRecord) => Promise<unknown>;
  /** Given, the list opens on "Unassigned", which hands the record back to the queue. */
  unassign?: (opened: OpenedRecord) => Promise<unknown>;
  /** The list and record reads the write makes stale, as `useUpdateRecord` names them. */
  invalidate: string;
  recordKey: string;
  /** The trigger's face and the list's name. */
  label?: string;
  /** Another write on the record is out: every row refuses a pick until it lands. */
  pending?: boolean;
  /** Runs once the write has landed and the list has closed. */
  onAssigned?: () => void;
}> &
  (
    | Readonly<{
        opener?: undefined;
        open?: undefined;
        onClose?: undefined;
        reasonId?: string;
      }>
    | Readonly<{
        /** Opened by a control elsewhere, such as a section's Edit link. */
        opener: "caller";
        open: boolean;
        onClose: () => void;
        reasonId?: undefined;
      }>
  );

// The pick is the write: one more pick walks it back, so a confirm step would
// only add a press.
export function OwnerPick({
  record,
  assign,
  unassign,
  invalidate,
  recordKey,
  label,
  reasonId,
  pending = false,
  onAssigned,
  open: callerOpen,
  onClose,
}: OwnerPickProps) {
  const t = useT();
  const [ownOpen, setOwnOpen] = useState(false);
  const open = callerOpen ?? ownOpen;
  const [opened, setOpened] = useState(record);
  // A caller opens the list without a press here, so the record is pinned as it shows.
  const [shown, setShown] = useState(open);
  if (shown !== open) {
    setShown(open);
    if (open) {
      setOpened(record);
    }
  }
  const setOpen = (next: boolean) => {
    if (onClose === undefined) {
      setOwnOpen(next);
    } else if (!next) {
      onClose();
    }
  };
  // The shared roster rather than a search, so the list opens with names.
  const owners = useRecordOwnerOptions(open);
  const partial = useRosterPartial("user", open);
  const me = useMe(open);
  // The toast lands after the list closes, when the roster may be gone.
  const pickedName = useRef("");

  const mutation = useUpdateRecord({
    update: async (values, _rows, pinned) => {
      if (pinned === undefined) {
        throw new Error("an owner pick writes against the record it opened on");
      }
      if (values.owner_id === null && unassign) {
        await unassign(pinned);
        return { id: pinned.id, cleared: true };
      }
      await assign(String(values.owner_id), pinned);
      return { id: pinned.id, cleared: false };
    },
    invalidate,
    recordKey,
    recordId: record.id,
    // Named off the pick, never a re-read: the write sends only that option's id.
    savedMessage: (updated) =>
      updated.cleared
        ? t("ownerPick.cleared")
        : t("ownerPick.done", { name: pickedName.current }),
    onDone: () => {
      setOpen(false);
      onAssigned?.();
    },
  });

  const roster = owners.roster;
  // The viewer first: taking a record yourself is the common pick.
  const viewerId = me.data?.user?.id;
  const colleagues = roster.data
    ? [
        ...owners.options.filter((owner) => owner.value === viewerId),
        ...owners.options.filter((owner) => owner.value !== viewerId),
      ].map(
        (owner): ListPopoverOption => ({
          id: owner.value,
          name: owner.label,
          keywords: owner.keywords,
        }),
      )
    : undefined;
  const options =
    colleagues && unassign
      ? [{ id: UNASSIGNED, name: t("lead.unassigned") }, ...colleagues]
      : colleagues;
  const current = record.owner_id ?? (unassign ? UNASSIGNED : undefined);
  const title = label ?? t("ownerPick.change");

  const change = (next: boolean) => {
    // Held open while the write is out, so its refusal has somewhere to land.
    if (!next && mutation.isPending) {
      return;
    }
    setOpen(next);
    // A refusal belongs to the attempt it answered, not to the next opening.
    if (!mutation.isPending) {
      mutation.reset();
    }
  };
  const opening: ListOpener =
    onClose === undefined
      ? { label: title, reasonId, open, onOpenChange: change }
      : { opener: "caller", open, onClose: () => change(false) };

  return (
    <ListPopover
      {...opening}
      title={title}
      searchLabel={t("ownerPick.search")}
      options={roster.isError ? [] : options}
      empty={roster.isError ? problemMessageOf(roster.error, t) : undefined}
      selected={current}
      onPick={(option, done) => {
        if (option.id === current) {
          done();
          return;
        }
        pickedName.current = option.name;
        const owner = option.id === UNASSIGNED ? null : option.id;
        mutation.mutate({ values: { owner_id: owner }, rows: {}, opened });
      }}
      pending={mutation.isPending || pending}
      error={mutation.isError ? problemMessageOf(mutation.error, t) : undefined}
      footer={<RosterPartialNote partial={partial} />}
    />
  );
}

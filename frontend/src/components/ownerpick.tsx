// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useRef, useState } from "react";
import {
  ListPopover,
  type ListPopoverOption,
} from "../design-system/listpopover";
import { useT } from "../i18n";
import { problemMessageOf } from "../lib/problem";
import { useRecordOwnerOptions } from "./assigneepicker";
import { useUpdateRecord } from "./edit";
import { RosterPartialNote, useRosterPartial } from "./roster";

type OwnerPickProps = Readonly<{
  /** The colleague who holds the record now: marked, and a pick of them writes nothing. */
  ownerId: string | null | undefined;
  assign: (ownerId: string) => Promise<unknown>;
  /** The list and record reads the write makes stale, as `useUpdateRecord` names them. */
  invalidate: string;
  recordKey: string;
  recordId: string;
  /** The trigger's face and the list's name; "Change owner" by default. */
  label?: string;
  reasonId?: string;
  /** Runs once the write has landed and the list has closed. */
  onAssigned?: () => void;
}>;

// The pick is the write: one more pick walks it back, so a confirm step would
// only add a press.
export function OwnerPick({
  ownerId,
  assign,
  invalidate,
  recordKey,
  recordId,
  label,
  reasonId,
  onAssigned,
}: OwnerPickProps) {
  const t = useT();
  const [open, setOpen] = useState(false);
  // The shared roster rather than a search, so the list opens with names.
  const owners = useRecordOwnerOptions(open);
  const partial = useRosterPartial("user", open);
  // The toast lands after the list closes, when the roster may be gone.
  const pickedName = useRef("");

  const mutation = useUpdateRecord({
    update: async (values) => {
      await assign(String(values.owner_id));
      return { id: recordId };
    },
    invalidate,
    recordKey,
    recordId,
    // Named off the pick, never a re-read: the write sends only that option's id.
    savedMessage: () => t("ownerPick.done", { name: pickedName.current }),
    onDone: () => {
      setOpen(false);
      onAssigned?.();
    },
  });

  const roster = owners.roster;
  const options = roster.data
    ? owners.options.map(
        (owner): ListPopoverOption => ({
          id: owner.value,
          name: owner.label,
          keywords: owner.keywords,
        }),
      )
    : undefined;
  const title = label ?? t("ownerPick.change");

  return (
    <ListPopover
      label={title}
      title={title}
      searchLabel={t("ownerPick.search")}
      reasonId={reasonId}
      open={open}
      onOpenChange={(next) => {
        // Held open while the write is out, so its refusal has somewhere to land.
        if (!next && mutation.isPending) {
          return;
        }
        setOpen(next);
        // A refusal belongs to the attempt it answered, not to the next opening.
        if (!mutation.isPending) {
          mutation.reset();
        }
      }}
      options={roster.isError ? [] : options}
      empty={roster.isError ? problemMessageOf(roster.error, t) : undefined}
      selected={ownerId ?? undefined}
      onPick={(option, done) => {
        if (option.id === ownerId) {
          done();
          return;
        }
        pickedName.current = option.name;
        mutation.mutate({ values: { owner_id: option.id }, rows: {} });
      }}
      pending={mutation.isPending}
      error={mutation.isError ? problemMessageOf(mutation.error, t) : undefined}
      footer={<RosterPartialNote partial={partial} />}
    />
  );
}

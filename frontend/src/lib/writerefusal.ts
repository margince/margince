// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  type RequiredGrant,
  useCanMutate,
  useRecordWriteRefusal,
  useWriteBlock,
  useWriteRefusal,
  type WritableRecord,
  type WriteBlock,
} from "./capability";
import type { EntityKind } from "./entity";
import { useMe } from "./session";

// The sentences a refused write verb says, in one place. A read seat is told
// to ask for a full seat; a role that only reads, for another role; a missing
// grant, for the right. A record's own verbs say why it takes no change.

// What a refusal by the seat or by the role says; every hook here reads it.
export const WRITE_BLOCK_REFUSAL = {
  seat: "settings.seatRefused",
  role: "record.roleReadsOnly",
} as const satisfies Record<WriteBlock, MessageKey>;

/**
 * Why this seat may not take a write verb, or undefined when it may. A role
 * without `object`'s update grant is told it only reads. Nothing is claimed
 * while `/me` is on its way. Once it settles without a snapshot, the access is
 * unknown, and the verb is refused with a way to recover.
 */
export function useActionRefusal({
  object,
  action,
}: RequiredGrant): string | undefined {
  const t = useT();
  const block = useWriteBlock(object);
  const refusal = useWriteRefusal(object, action, {
    seat: t(WRITE_BLOCK_REFUSAL.seat),
    grant: t(
      block === "role" ? WRITE_BLOCK_REFUSAL.role : "record.actionRefused",
    ),
  });
  return refusalWhen(useAccessState(), refusal, t);
}

/**
 * Why this seat may not write at all, or undefined when it may: a read seat
 * is clamped before any role is asked. Read like `useActionRefusal`.
 */
export function useSeatRefusal(): string | undefined {
  const t = useT();
  const refusal = useCanMutate() ? undefined : t(WRITE_BLOCK_REFUSAL.seat);
  return refusalWhen(useAccessState(), refusal, t);
}

// Whether this seat's access is known: `/me` on its way, settled without a
// snapshot, or carrying one. A later snapshot moves it back to known.
type AccessState = "pending" | "unknown" | "known";

function useAccessState(): AccessState {
  const answered = useMe().data?.authorization !== undefined;
  const settled = useMeSettled();
  if (answered) {
    return "known";
  }
  return settled ? "unknown" : "pending";
}

function refusalWhen(
  state: AccessState,
  refusal: string | undefined,
  t: ReturnType<typeof useT>,
): string | undefined {
  if (state === "pending") {
    return undefined;
  }
  return state === "unknown" ? t("record.accessUnknown") : refusal;
}

/**
 * Whether `/me` has given its answer, a failure included. A write verb waits
 * for it. A failed read goes back to pending as each new reader mounts, so
 * having failed once counts as settled.
 */
export function useMeSettled(): boolean {
  const me = useMe();
  return !me.isPending || me.errorUpdateCount > 0;
}

// What each record kind's page says when the record takes no change.
const RECORD_REFUSALS = {
  contact: {
    archived: "contact.rail.archivedReadOnly",
    notYours: "contact.notYoursToChange",
  },
  company: {
    archived: "record.archivedReadOnly",
    notYours: "record.notYoursToChange",
  },
  deal: {
    archived: "deal.archivedReadOnly",
    notYours: "deal.notYoursToChange",
  },
  lead: {
    archived: "lead.terminalReadOnly",
    notYours: "lead.notYoursToChange",
  },
  project: {
    archived: "project.archivedReadOnly",
    notYours: "project.notYoursToChange",
  },
} as const satisfies Record<
  EntityKind,
  Record<"archived" | "notYours", MessageKey>
>;

/**
 * Why this reader may not change this record, in its kind's words, or
 * undefined when they may. See `useRecordWriteRefusal`.
 */
export function useKindWriteRefusal(
  kind: EntityKind,
  record: WritableRecord | undefined,
): string | undefined {
  const t = useT();
  const words = RECORD_REFUSALS[kind];
  return useRecordWriteRefusal(kind, record, {
    archived: t(words.archived),
    seat: t(WRITE_BLOCK_REFUSAL.seat),
    readOnlyRole: t(WRITE_BLOCK_REFUSAL.role),
    notYours: t(words.notYours),
  });
}

/**
 * What a refused Archive says on a record page. A read seat or a role that
 * only reads is told so on every kind. Otherwise a contact, company or deal is
 * its owner's or a manager's to archive; a project takes the delete grant alone.
 */
export function useArchiveRefusal(
  kind: "contact" | "company" | "deal" | "project",
): string {
  const t = useT();
  const block = useWriteBlock(kind);
  if (block) {
    return t(WRITE_BLOCK_REFUSAL[block]);
  }
  return t(
    kind === "project" ? "record.actionRefused" : "record.archiveRefused",
  );
}

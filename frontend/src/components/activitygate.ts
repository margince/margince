// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId } from "react";
import type { MessageKey } from "../i18n/en";
import { useGrantRefused } from "../lib/capability";
import { useMe } from "../lib/session";
import type { WritingMomentVerb } from "../lib/writeverbs";

/**
 * Whether this reader may write activities (log, task, book), asked once for
 * the page. The verbs point at `sentenceId`, and the page states the refusal
 * under that id outside any menu, which mounts its rows only once opened. A
 * grant `/me` has not answered yet refuses nothing; the verbs are only briefly
 * quiet (`pending`).
 */
export type ActivityGate = Readonly<{
  refusedId: string | undefined;
  pending: boolean;
  sentenceId: string;
  refusalKey?: MessageKey;
}>;

/**
 * The record the activity files onto. Filing onto a record changes it, so the
 * server answers per row. A contact carries `attachable`; any other record
 * goes through `writableTarget`. An absent answer refuses. Null is a record
 * still loading.
 */
type AttachTarget = {
  readonly attachable?: boolean;
  readonly refusal?: MessageKey;
};

/** A deal, company, lead or project as an attach target: its own write answer. */
export function writableTarget(
  record: { readonly writable?: boolean } | null | undefined,
): AttachTarget | null | undefined {
  return record
    ? { attachable: record.writable, refusal: "record.attachRefused" }
    : record;
}

/** What the gate says when the role or the seat may not write activities. */
export const ACTIVITY_ROLE_REFUSAL: MessageKey = "record.roleReadsOnly";

export function useActivityGate(record?: AttachTarget | null): ActivityGate {
  // Log, Add task and Book issue the same POST, and a read seat is refused
  // before RBAC is consulted. Either way only an administrator can lift it.
  const me = useMe();
  const roleRefused = useGrantRefused({ object: "activity", action: "create" });
  const sentenceId = useId();
  const known = me.data?.authorization !== undefined;
  const refusalKey = roleRefused
    ? ACTIVITY_ROLE_REFUSAL
    : attachRefusal(record);
  return {
    refusedId: refusalKey ? sentenceId : undefined,
    refusalKey,
    pending: !known || record === null,
    sentenceId,
  };
}

function attachRefusal(
  record: AttachTarget | null | undefined,
): MessageKey | undefined {
  if (!record || record.attachable === true) return undefined;
  return record.refusal ?? "contact.attachRefused";
}

/**
 * Whether a lone verb that writes an activity is offered; one refused is not
 * drawn. `contact` is the contact the write files under, where it names one.
 *
 * `_verb` is read only by the compiler: it must be a kind the server counts as
 * writing. The type import keeps the mirror table out of the Home chunk.
 */
export function useWritingVerbOffered(
  _verb: WritingMomentVerb,
  contact?: AttachTarget,
): boolean {
  return useActivityGate(contact).refusalKey === undefined;
}

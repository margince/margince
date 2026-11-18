// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId } from "react";
import type { MessageKey } from "../i18n/en";
import { useCanWrite } from "../lib/capability";
import { useMe } from "../lib/session";
import { type MomentVerbKind, momentVerbWrites } from "../lib/writeverbs";

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

export function useActivityGate(
  contact?: { readonly attachable?: boolean } | null,
  permissionRefusal: MessageKey = "record.logActivityRefused",
): ActivityGate {
  // useCanWrite, not useCan: Log and Add task issue the same POST, and a read
  // seat is refused before RBAC is consulted.
  const me = useMe();
  const canLog = useCanWrite("activity", "create");
  const sentenceId = useId();
  const known = me.data?.authorization !== undefined;
  const refusalKey =
    known && !canLog
      ? permissionRefusal
      : contact && contact.attachable !== true
        ? "contact.attachRefused"
        : undefined;
  return {
    refusedId: refusalKey ? sentenceId : undefined,
    refusalKey,
    pending: !known || contact === null,
    sentenceId,
  };
}

/**
 * Whether a lone verb is offered: one that writes an activity and is refused
 * is not drawn. The verb names its kind, so the server's classification of it
 * (`momentVerbWrites`) decides whether the grant matters. `contact` is the
 * contact the write files under, where it names one.
 */
export function useVerbOffered(
  kind: MomentVerbKind,
  contact?: { readonly attachable?: boolean },
): boolean {
  const refused = useActivityGate(contact).refusalKey !== undefined;
  return !(momentVerbWrites(kind) && refused);
}

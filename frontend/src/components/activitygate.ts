// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId } from "react";
import { useCanWrite } from "../lib/capability";
import { useMe } from "../lib/session";

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
}>;

export function useActivityGate(): ActivityGate {
  // useCanWrite, not useCan: Log and Add task issue the same POST, and a read
  // seat is refused before RBAC is consulted.
  const me = useMe();
  const canLog = useCanWrite("activity", "create");
  const sentenceId = useId();
  const known = me.data?.authorization !== undefined;
  return {
    refusedId: known && !canLog ? sentenceId : undefined,
    pending: !known,
    sentenceId,
  };
}

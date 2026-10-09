// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useCallback, useEffect, useRef } from "react";

// `pending` flips a task after the click, so a quick second press would start
// a second write. The latch closes at the press and opens when pending ends,
// on an error, on reopening, or a task later if no write showed itself. A
// caller without `pending` has nothing to wait for and runs on every press.
export function useSendOnce(
  pending: boolean | undefined,
  error?: string | null,
  open?: boolean,
): (send: () => void) => void {
  const sent = useRef(false);
  const pendingNow = useRef(pending);
  pendingNow.current = pending;
  // biome-ignore lint/correctness/useExhaustiveDependencies: error and open only trigger the reset
  useEffect(() => {
    if (!pending) sent.current = false;
  }, [pending, error, open]);
  return useCallback(
    (send) => {
      if (pending === undefined) {
        send();
        return;
      }
      if (sent.current || pending) return;
      sent.current = true;
      send();
      // Queued after the write's own timers, so a write in flight is drawn
      // pending before this runs.
      setTimeout(() => {
        if (!pendingNow.current) sent.current = false;
      }, 0);
    },
    [pending],
  );
}

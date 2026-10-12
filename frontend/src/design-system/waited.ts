// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useState } from "react";

/**
 * Whether a wait has lasted `delayMs`, so a pending state can stay hidden
 * through a wait too short to be worth showing. Unset, it has waited already.
 */
export function useWaited(delayMs: number | undefined): boolean {
  const [waited, setWaited] = useState(delayMs === undefined);
  useEffect(() => {
    if (delayMs === undefined) {
      // A caller that drops the delay wants the pending state now. Left where
      // the last delay put it, `waited` would stay false for good.
      setWaited(true);
      return;
    }
    // The clock runs per mount and per delay, not per read. A reader typing
    // through a slow search sees one bar, not one that blinks on every key.
    setWaited(false);
    const timer = setTimeout(() => setWaited(true), delayMs);
    return () => clearTimeout(timer);
  }, [delayMs]);
  return waited;
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { createContext, useContext, useLayoutEffect } from "react";

/**
 * The whole address the routed screen around a component was drawn for.
 * A screen stays up past a move while the next loads or a draft holds it.
 * Its reads must not see the address that replaced it. Null outside a routed
 * screen.
 */
export const ScreenAddress = createContext<string | null>(null);

/** How the routed screen tells the frame around it which address it shows. */
export const ShownAddressSink = createContext<
  ((address: string) => void) | null
>(null);

/** Tell the frame around the routed screen that `address` is on screen. */
export function useReportShownAddress(address: string): void {
  const report = useContext(ShownAddressSink);
  useLayoutEffect(() => {
    report?.(address);
  }, [report, address]);
}

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

/** The page on screen, and whether the page asked for is still loading. */
export type ShownScreen = Readonly<{ address: string; pending: boolean }>;

/** How the routed screen tells the frame around it what is on screen. */
export const ShownAddressSink = createContext<
  ((shown: ShownScreen | null) => void) | null
>(null);

/**
 * Tell the frame around the routed screen what is on screen. A screen that
 * leaves, or hides behind a loading body, takes its report back.
 */
export function useReportShownAddress(address: string, pending = false): void {
  const report = useContext(ShownAddressSink);
  useLayoutEffect(() => {
    report?.({ address, pending });
    return () => report?.(null);
  }, [report, address, pending]);
}

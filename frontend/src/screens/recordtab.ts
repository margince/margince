// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useRoute } from "../app/router";
import { navigateWithinRecord } from "./worklist.return";

/**
 * A record's tab, read off the address rather than held beside it, so it
 * survives a reload and belongs to the record it names: swapping records
 * cannot carry one along. A move is a PUSH, so Back steps between the tabs a
 * reader opened, and it keeps the way back to the Worklist.
 *
 * The first entry of `tabs` is where an address naming no known tab lands.
 */
export function useAddressedTab<Tab extends string>(
  screen: "companies" | "leads",
  recordId: string,
  tabs: readonly [Tab, ...Tab[]],
): [Tab, (next: Tab) => void] {
  const route = useRoute();
  const addressed =
    route.screen === screen && route.id === recordId ? route.id2 : undefined;
  return [
    tabs.find((tab) => tab === addressed) ?? tabs[0],
    (next: Tab) => navigateWithinRecord({ screen, id: recordId, id2: next }),
  ];
}

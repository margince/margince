import { useDeferredValue } from "react";
import { SETTINGS_SCREEN } from "../features/settings/settingsnav";
import type { Route } from "../lib/router";
import { opensReservedPage, RECORD_VIEW_SCREENS } from "./nav";

/**
 * Whether the page keeps the capped reading column (settings alone), and
 * whether it is a record.
 *
 * Read from the screen on the page, not the address. App.tsx's ScreenView
 * keeps the old screen up while the next chunk loads. A class keyed on the
 * live route would flip a beat before the screen changes.
 */
export function useReadingColumn(route: Route): {
  gridded: boolean;
  record: boolean;
} {
  const shown = useDeferredValue(route);
  return {
    gridded: shown.screen === SETTINGS_SCREEN,
    record: isRecordPage(shown),
  };
}

/** Whether `route` is a record's own page, as against its list or a form. */
export function isRecordPage(route: Route): boolean {
  // A RECORD id makes one: `#/companies` and `#/deals/new` are both lists.
  const recordPage = route.id !== undefined && !opensReservedPage(route);
  return recordPage && RECORD_VIEW_SCREENS.has(route.screen);
}

import { useDeferredValue } from "react";
import { SETTINGS_SCREEN } from "../features/settings/settingsnav";
import type { Route } from "../lib/router";
import { opensReservedPage, RECORD_VIEW_SCREENS } from "./nav";

/**
 * Which page keeps the capped reading column (settings alone), and whether the
 * page is a record.
 *
 * Decided from the screen ON THE PAGE, not the address. The screen is rendered
 * against a deferred route (App.tsx's ScreenView keeps the old screen up while
 * the next chunk loads), so a class keyed on the live route flipped a beat
 * before the screen changed.
 */
export function useReadingColumn(route: Route): {
  gridded: boolean;
  record: boolean;
} {
  const shown = useDeferredValue(route);
  // A RECORD id makes one: `#/companies` and `#/deals/new` are both lists.
  const recordPage = shown.id !== undefined && !opensReservedPage(shown);
  return {
    gridded: shown.screen === SETTINGS_SCREEN,
    record: recordPage && RECORD_VIEW_SCREENS.has(shown.screen),
  };
}

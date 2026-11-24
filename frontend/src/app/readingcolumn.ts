import { SETTINGS_SCREEN } from "../features/settings/settingsnav";
import type { Route } from "../lib/router";
import { opensReservedPage, RECORD_VIEW_SCREENS } from "./nav";

/**
 * Whether the page is Settings, whose column the shell caps, and whether it is
 * a record. Any other page takes the reading column with `.wrap.narrow`. Asked of the page on screen, which the title and the
 * rail read too.
 */
export function readingColumn(shown: Route): {
  gridded: boolean;
  record: boolean;
} {
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

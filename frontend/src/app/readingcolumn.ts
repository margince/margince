import { SETTINGS_SCREEN } from "../features/settings/settingsnav";
import type { Route } from "../lib/router";
import {
  GRIDDED_RECORD_SCREENS,
  GRIDDED_SCREENS,
  opensReservedPage,
} from "./nav";

/**
 * Which pages keep the capped reading column, and which of those are records.
 * Asked of the page on screen, which the title and the rail read too.
 */
export function readingColumn(shown: Route): {
  gridded: boolean;
  griddedRecord: boolean;
} {
  const griddedRecord = isRecordPage(shown);
  // The id-less half of the same policy: a screen that reads down but is not a
  // record, so there is no id to key on. Brief is the one today.
  const griddedScreen = GRIDDED_SCREENS.has(shown.screen);
  // A unit is NOT in this family, though it is leveled: the reading column is a
  // claim about the page's own content, and a unit's surface is the unit's to
  // lay out.
  const gridded =
    shown.screen === SETTINGS_SCREEN || griddedRecord || griddedScreen;
  return { gridded, griddedRecord };
}

/** Whether `route` is a record's own page, as against its list or a form. */
export function isRecordPage(route: Route): boolean {
  // A RECORD id makes one: `#/companies` and `#/deals/new` are both lists.
  const recordPage = route.id !== undefined && !opensReservedPage(route);
  return recordPage && GRIDDED_RECORD_SCREENS.has(route.screen);
}

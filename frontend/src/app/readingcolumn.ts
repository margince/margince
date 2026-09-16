import { useDeferredValue } from "react";
import { SETTINGS_SCREEN } from "../screens/settingsnav";
import {
  GRIDDED_RECORD_SCREENS,
  GRIDDED_SCREENS,
  opensCreateForm,
} from "./nav";
import type { Route } from "./router";

/**
 * Which pages keep the capped reading column, and which of those are records.
 *
 * Decided from the screen ON THE PAGE, not the address. The screen is rendered
 * against a deferred route (App.tsx's ScreenView keeps the old screen up while
 * the next chunk loads), so a width keyed on the live route flipped a beat
 * before the screen changed and a record page was drawn at a list's full width
 * for the length of a chunk load.
 */
export function useReadingColumn(route: Route): {
  gridded: boolean;
  griddedRecord: boolean;
} {
  const shown = useDeferredValue(route);
  // A RECORD id makes one: `#/companies` and `#/deals/new` are both lists.
  const recordPage = shown.id !== undefined && !opensCreateForm(shown);
  const griddedRecord = recordPage && GRIDDED_RECORD_SCREENS.has(shown.screen);
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

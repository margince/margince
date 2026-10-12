// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What state the contact's exchanges are in, as a decision apart from the tab
// that draws it. A wrong answer here is silent: a withheld section drawn as an
// empty list says the relationship never happened. Out here it is a function
// of its arguments, provable case by case.

import type { components } from "../api/schema";
import type { RecordTimeline } from "../design-system/recordtimeline";
import { type SectionState, sectionState } from "../design-system/surfacestate";
import type { RecordChronology } from "./recordchronology";

type Contact360 = components["schemas"]["Contact360"];

/**
 * timelineState reads the state of the exchanges the All and Threads cuts
 * draw. The Changes cut is the record's history panel, which judges its own.
 */
export function timelineState(
  view: Contact360 | undefined,
  chronology: RecordChronology,
  timeline: RecordTimeline,
  narrowed: boolean,
  loading: boolean,
): SectionState {
  // A narrowed read is the list's own, not the 360's section, with its own
  // wait and failure. A grant that withheld the section withholds the list
  // too, through the server's 403.
  const base = narrowed
    ? narrowedState(timeline)
    : sectionState(
        view,
        "activities",
        Boolean(view?.activities),
        timeline.activities.length,
        loading,
      );
  // A capped list that says nothing reads as the whole history. A reader at the
  // oldest of 25 rows would take it for the day the relationship began.
  return base === "ready" && chronology.truncated ? "partial" : base;
}

function narrowedState(timeline: RecordTimeline): SectionState {
  if (timeline.isPending) {
    return "loading";
  }
  if (timeline.isError) {
    return "failed";
  }
  return timeline.activities.length === 0 ? "empty" : "ready";
}

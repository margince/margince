// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What STATE the contact's chronology is in, as a decision apart from the tab
// that draws it.
//
// The tab reads two feeds that fail independently and four cuts that each read
// a different pair of them, and every combination has a right answer a reader
// acts on: a skeleton, a failure they can retry, a section their grant does not
// reach, a capped list that must say it is capped. It is the one part of that
// tab with no markup in it, and the one part where a wrong answer is silent —
// a withheld section drawn as an empty list says the relationship never
// happened. Out here it is a function of its arguments, provable case by case.

import type { components } from "../api/schema";
import type { RecordTimeline } from "../design-system/recordtimeline";
import { type SectionState, sectionState } from "../design-system/surfacestate";
import {
  type RecordChronology,
  readsExchangesOnly,
  type TimelineFilter,
} from "./recordchronology";

type Person360 = components["schemas"]["Person360"];

/**
 * timelineState reads the state of whichever feed the FILTER is actually
 * showing. The two halves fail independently: a 360 that withheld its
 * activities says nothing about the change feed, and reporting the Changes
 * view as withheld on that basis would hide rows that loaded perfectly well.
 */
export function timelineState(
  view: Person360 | undefined,
  filter: TimelineFilter,
  chronology: RecordChronology,
  timeline: RecordTimeline,
  narrowed: boolean,
  loading: boolean,
): SectionState {
  if (readsExchangesOnly(filter)) {
    // A narrowed read is the list's own, not the 360's section: it has its
    // own wait and its own failure, and a grant that withheld the section
    // withholds the list the same way through the server's 403.
    //
    // Conversations is judged here too, on exactly this feed: below, a
    // withheld activities section would reach the reader as a record nobody
    // has ever written to.
    const base = narrowed
      ? narrowedState(timeline)
      : sectionState(
          view,
          "activities",
          Boolean(view?.activities),
          timeline.activities.length,
          loading,
        );
    return base === "ready" && chronology.truncated ? "partial" : base;
  }
  // The whole chronology holds the activities half, so a grant that withheld
  // that section withholds part of THIS cut too — and a withheld half drawn as
  // an empty list is the one thing a section may never do. Answered BEFORE the
  // changes read lands, because the withholding is already known: waiting on a
  // second feed to say what the first one already said draws a skeleton over a
  // boundary the reader could have been told about at once. Once change rows
  // arrive it is partial rather than withheld — some of the record is here, and
  // the rest is missing rather than absent.
  if (
    filter === "all" &&
    view &&
    (view.sections_omitted ?? []).includes("activities")
  ) {
    return chronology.entries.length === 0 ? "withheld" : "partial";
  }
  if (chronology.loading) {
    return "loading";
  }
  if (chronology.failed) {
    return "failed";
  }
  // A capped list that says nothing reads as the whole history — a reader
  // looking at the oldest of 25 rows would take it for the day the
  // relationship began. True on the combined cut as much as on the narrow one.
  if (chronology.truncated) {
    return "partial";
  }
  return chronology.entries.length === 0 ? "empty" : "ready";
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

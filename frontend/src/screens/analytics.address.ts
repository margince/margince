// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Analytics' own address dials.
//
// Its own file for the reason `leads.address.ts` is one: a caller that builds a
// link to this screen must not import the screen to learn what the address is
// called. The Brief's pipeline reading opens the section its figure was read
// from, and reaching `analytics.tsx` for that one function would pull every
// report card and its queries into the morning's bundle.

import { navigate } from "../app/router";

// A SECTION is what the address names and what the tabs choose between; a
// REPORT is one result inside it. They were the same thing while every section
// held exactly one report, and keeping them the same would have meant the
// address changing the day a section grew a second result — breaking every
// link anyone had saved to it.
//
// The order is the tab strip's, and `analytics.tsx` declares which reports each
// section draws against this type, so a section cannot be addressable here and
// missing there.
export const SECTIONS = [
  "forecast",
  "pipeline",
  "performance",
  "outcomes",
  "coverage",
  "delivery",
] as const;

export type Section = (typeof SECTIONS)[number];

/** isSection narrows a URL segment, which is any string a reader can type. */
function isSection(value: string | undefined): value is Section {
  return SECTIONS.some((section) => section === value);
}

// The old address named a report. Those links are in bookmarks and in sent
// mail, so each one still answers, with the section that now holds it.
// Keyed by string rather than by a report name, because a RETIRED one is still
// an address somebody saved. `deals-by-stage` names no report the screen draws
// any more — the stage view reads pipeline-current — and the link in a
// bookmark or a sent mail must still land on the section that answers it.
const SECTION_OF_REPORT: Readonly<Record<string, Section>> = {
  forecast: "pipeline",
  "pipeline-current": "pipeline",
  "deals-by-stage": "pipeline",
  "open-deals-per-company": "pipeline",
  "win-loss": "performance",
  "stage-age": "performance",
  "projects-by-phase": "delivery",
  "project-commitments": "delivery",
  "projects-gone-quiet": "delivery",
};

export function sectionFromAddress(segment: string | undefined): Section {
  if (isSection(segment)) {
    return segment;
  }
  if (segment && segment in SECTION_OF_REPORT) {
    return SECTION_OF_REPORT[segment];
  }
  return "forecast";
}

/**
 * Open one section of Analytics.
 *
 * Which section is open is an ADDRESS and not local state, so a reader can link
 * to one and Back steps between the sections they looked at. One spelling of
 * that navigation, because the tab strip and a reading's door are the same
 * move: the strip chooses a view, and a door sends a reader to the section
 * holding the rows their figure was read from.
 */
export function openAnalyticsSection(section: Section): void {
  navigate({ screen: "analytics", id: section });
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { createElement, Fragment, type ReactNode } from "react";
import type { components } from "../api/schema";
import { formatDateAbbrev } from "../format/format";
import type { Locale } from "../i18n";
import type { MessageKey } from "../i18n/en";

type Summary = components["schemas"]["WeeklyNumericSummary"];
type Family = keyof components["schemas"]["WeeklyFigureCoverageSet"];
type NumericCoverage = Summary["bookings_coverage"];

const FIGURES = [
  "won",
  "lost",
  "moved",
  "tasks",
  "meetings",
  "leads",
  "commitments",
] as const;

/** One figure a weekly review prints, named by what it counts. */
export type WeeklyFigure = (typeof FIGURES)[number];

const FAMILY: Record<WeeklyFigure, Family> = {
  won: "deals",
  lost: "deals",
  moved: "deals",
  tasks: "tasks",
  meetings: "meetings",
  leads: "leads",
  commitments: "commitments",
};

/**
 * Whether a figure's count can be read as a measurement of the week.
 *
 * `unrecorded` is a week its source had not started on, so its zero is the
 * absence of a source; `unavailable` is a source that could not be read;
 * `partial` counts only part of the week and still shows its number.
 */
export type FigureState =
  | { kind: "measured" }
  | { kind: "partial"; since?: string; reason?: string }
  | { kind: "unrecorded"; since?: string; reason?: string }
  | { kind: "unavailable"; reason?: string };

function numericCoverageOf(
  summary: Summary | undefined,
  figure: WeeklyFigure,
): NumericCoverage | undefined {
  if (figure === "won") return summary?.bookings_coverage;
  if (figure === "meetings") return summary?.meetings_coverage;
  return undefined;
}

function unreadable(coverage: NumericCoverage | undefined): boolean {
  return (
    coverage !== undefined &&
    (coverage.withheld ||
      !["ok", "no_data", "partial"].includes(coverage.status))
  );
}

// An unrecorded source outranks an unreadable one: the week predating the
// source is the reason nothing could be read.
function figureState(
  summary: Summary | undefined,
  figure: WeeklyFigure,
): FigureState {
  const source = summary?.figure_coverage?.[FAMILY[figure]];
  const numeric = numericCoverageOf(summary, figure);
  if (source?.status === "not_recorded") {
    return {
      kind: "unrecorded",
      since: source.recorded_since,
      reason: source.reason,
    };
  }
  if (unreadable(numeric)) {
    return { kind: "unavailable", reason: numeric?.reason };
  }
  if (source?.status === "partial") {
    return {
      kind: "partial",
      since: source.recorded_since,
      reason: source.reason,
    };
  }
  if (numeric?.status === "partial") {
    return { kind: "partial", reason: numeric.reason };
  }
  return { kind: "measured" };
}

export type WeeklyFigures = Record<WeeklyFigure, FigureState>;

export function weeklyNumericStatus(summary: Summary | undefined): {
  basis: "brief.weekly.sharedDefinitions" | "brief.weekly.legacyDefinitions";
  partial: boolean;
  figures: WeeklyFigures;
} {
  const of = (figure: WeeklyFigure) => figureState(summary, figure);
  const figures: WeeklyFigures = {
    won: of("won"),
    lost: of("lost"),
    moved: of("moved"),
    tasks: of("tasks"),
    meetings: of("meetings"),
    leads: of("leads"),
    commitments: of("commitments"),
  };
  const incomplete = (coverage: NumericCoverage | undefined) =>
    unreadable(coverage) || coverage?.status === "partial";
  return {
    basis: summary
      ? "brief.weekly.sharedDefinitions"
      : "brief.weekly.legacyDefinitions",
    partial:
      incomplete(summary?.bookings_coverage) ||
      incomplete(summary?.meetings_coverage),
    figures,
  };
}

/** Whether a figure's count may be printed, and so said, at all. */
export function figureShown(state: FigureState): boolean {
  return state.kind === "measured" || state.kind === "partial";
}

/** Every figure's source began after the week: there is no week to judge. */
export function beforeRecordedHistory(figures: WeeklyFigures): boolean {
  return FIGURES.every((figure) => figures[figure].kind === "unrecorded");
}

/** Whether any figure's source had not started by the end of the week. */
export function anyUnrecorded(figures: WeeklyFigures): boolean {
  return FIGURES.some((figure) => figures[figure].kind === "unrecorded");
}

type Translate = (key: MessageKey, values?: Record<string, string>) => string;
type Place = Readonly<{ t: Translate; locale: Locale; zone: string }>;

type Reading = { value: string; detail?: ReactNode };

// The qualifier rides the detail line, never a popover: a partial figure has to
// say so at a glance or it reads as a whole week's count.
function qualified(detail: ReactNode, qualifier: string): ReactNode {
  if (
    detail === undefined ||
    detail === null ||
    detail === false ||
    detail === ""
  )
    return qualifier;
  if (typeof detail === "string") return [detail, qualifier].join(" · ");
  return createElement(Fragment, null, detail, " · ", qualifier);
}

/**
 * A stat card's reading for a figure, from the caller's own reading of it.
 *
 * A partial figure keeps the caller's value and adds its qualifier to the
 * detail line; a figure nobody measured replaces both.
 */
export function figureReading(
  state: FigureState,
  shown: Readonly<Reading>,
  { t, locale, zone }: Place,
): Reading {
  const from = (since: string | undefined) =>
    since && formatDateAbbrev(since, locale, zone);
  switch (state.kind) {
    case "measured":
      return shown;
    case "unavailable":
      return { value: t("reporting.unavailable"), detail: state.reason };
    case "partial": {
      const date = from(state.since);
      return {
        value: shown.value,
        detail: qualified(
          shown.detail,
          date
            ? t("brief.weekly.partialFrom", { date })
            : (state.reason ?? t("reporting.status.partial")),
        ),
      };
    }
    case "unrecorded": {
      const date = from(state.since);
      return {
        value: t("brief.weekly.notRecorded"),
        detail: date
          ? t("brief.weekly.recordedFrom", { date })
          : (state.reason ?? t("brief.weekly.noRecords")),
      };
    }
  }
}

/** A figure on one line, where there is no room for a detail. */
export function figureValue(
  state: FigureState,
  value: string,
  t: Translate,
): string {
  switch (state.kind) {
    case "measured":
      return value;
    case "unavailable":
      return t("reporting.unavailable");
    case "partial":
      return t("brief.weekly.partialValue", { value });
    case "unrecorded":
      return t("brief.weekly.notRecorded");
  }
}

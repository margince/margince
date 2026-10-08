// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, it } from "vitest";
import { formatDateAbbrev } from "../format/format";
import { viewerZone } from "../format/timezone";
import { en, type MessageKey } from "../i18n/en";
import {
  beforeHistoryWeeklyNumbers,
  partlyRecordedWeeklyNumbers,
  sharedWeeklyNumbers,
  unavailableWeeklyNumbers,
} from "./brief.fixtures";
import {
  beforeRecordedHistory,
  figureReading,
  figureShown,
  figureValue,
  weeklyNumericStatus,
} from "./brief.numeric";

// The catalog's own words, filled the way the app fills them.
function t(key: MessageKey, values?: Record<string, string>): string {
  const template: string = en[key];
  return Object.entries(values ?? {}).reduce(
    (text, [name, value]) => text.replaceAll(`{${name}}`, value),
    template,
  );
}
const place = { t, locale: "en" as const, zone: viewerZone() };
const day = (iso: string) => formatDateAbbrev(iso, "en", viewerZone());

it("keeps a complete zero-activity week readable", () => {
  const result = weeklyNumericStatus({
    ...sharedWeeklyNumbers,
    won_minor: 0,
    bookings_coverage: { status: "no_data", withheld: false },
    meetings_coverage: { status: "no_data", withheld: false },
  });
  expect(result.partial).toBe(false);
  expect(result.figures.won).toEqual({ kind: "measured" });
  expect(result.figures.meetings).toEqual({ kind: "measured" });
});

it("withholds an unreadable source and keeps a partial one's figure", () => {
  const { partial, figures } = weeklyNumericStatus(unavailableWeeklyNumbers);
  expect(partial).toBe(true);
  expect(figures.won.kind).toBe("unavailable");
  expect(figures.meetings).toEqual({
    kind: "partial",
    reason: "Some meeting history predates confirmed outcomes.",
  });
  expect(figureShown(figures.won)).toBe(false);
  expect(figureShown(figures.meetings)).toBe(true);
  expect(
    figureReading(figures.meetings, { value: "3 of 5", detail: "+1" }, place),
  ).toEqual({
    value: "3 of 5",
    detail: "+1 · Some meeting history predates confirmed outcomes.",
  });
});

it("identifies a legacy edition and states no coverage for it", () => {
  const { basis, partial, figures } = weeklyNumericStatus(undefined);
  expect(basis).toBe("brief.weekly.legacyDefinitions");
  expect(partial).toBe(false);
  expect(Object.values(figures).every((s) => s.kind === "measured")).toBe(true);
  expect(
    figureReading(figures.tasks, { value: "4", detail: "+1" }, place),
  ).toEqual({ value: "4", detail: "+1" });
  expect(figureValue(figures.tasks, "4", t)).toBe("4");
});

it("reads a week before every source began as before recorded history", () => {
  const { figures } = weeklyNumericStatus(beforeHistoryWeeklyNumbers);
  expect(beforeRecordedHistory(figures)).toBe(true);
  expect(figureShown(figures.lost)).toBe(false);
  expect(
    figureReading(figures.lost, { value: "0", detail: "+0" }, place),
  ).toEqual({
    value: "Not recorded",
    detail: `Recorded from ${day("2026-09-21T09:00:00Z")}`,
  });
  expect(figureValue(figures.tasks, "0 of 0", t)).toBe("Not recorded");
});

it("lets an unrecorded source outrank an unreadable one", () => {
  const { figures } = weeklyNumericStatus({
    ...unavailableWeeklyNumbers,
    figure_coverage: beforeHistoryWeeklyNumbers.figure_coverage,
  });
  expect(figures.won.kind).toBe("unrecorded");
});

it("keeps a partly recorded figure's number with its start date", () => {
  const { figures } = weeklyNumericStatus(partlyRecordedWeeklyNumbers);
  expect(beforeRecordedHistory(figures)).toBe(false);
  // The won card keeps its money line, and the qualifier follows it.
  expect(
    figureReading(figures.won, { value: "3", detail: "€965,000" }, place),
  ).toEqual({
    value: "3",
    detail: `€965,000 · Partial week: counted from ${day("2026-09-10T08:00:00Z")}`,
  });
  expect(figureValue(figures.moved, "3", t)).toBe("3 (partial)");
  expect(figureReading(figures.leads, { value: "7 of 9" }, place)).toEqual({
    value: "Not recorded",
    detail: "No lead source in scope.",
  });
  expect(figures.commitments).toEqual({ kind: "measured" });
});

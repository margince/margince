/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { afterEach, describe, expect, it, vi } from "vitest";
import { calendarDay } from "../format/calendarday";
import type { ISODate } from "../format/datepattern";
import { browserZone, DAY_MS } from "../format/timezone";
import {
  hintDismissed,
  INSTALL_HINT_SNOOZE_MS,
  type InstallHintRecord,
  installHintDue,
  parseInstallHint,
  shellDayNoted,
} from "./installhint";
import { readStored, removeStored, STORAGE_KEYS, writeStored } from "./storage";

const FIRST_RUN = Date.UTC(2026, 3, 14, 9);
const NEVER_SEEN = parseInstallHint(null);

function dayOf(at: number): ISODate {
  return calendarDay(new Date(at), browserZone());
}

function firstRun(): InstallHintRecord {
  return shellDayNoted(NEVER_SEEN, dayOf(FIRST_RUN));
}

function dueAt(record: InstallHintRecord, at: number): boolean {
  return installHintDue(record, dayOf(at), at);
}

afterEach(() => {
  removeStored(STORAGE_KEYS.installHint);
  vi.resetModules();
});

describe("when the install hint is due", () => {
  it("is never due on the first day this device runs the app", () => {
    expect(dueAt(firstRun(), FIRST_RUN)).toBe(false);
  });

  it("is due from the next calendar day on", () => {
    expect(dueAt(firstRun(), FIRST_RUN + DAY_MS)).toBe(true);
  });

  it("is not due before the first day is recorded", () => {
    expect(dueAt(NEVER_SEEN, FIRST_RUN + DAY_MS)).toBe(false);
  });

  it("keeps the first day it recorded", () => {
    const later = shellDayNoted(firstRun(), dayOf(FIRST_RUN + 3 * DAY_MS));
    expect(later.firstDay).toBe(dayOf(FIRST_RUN));
  });

  it("is due on the first day once the reader completed a task on Home", () => {
    expect(dueAt({ ...firstRun(), taskDone: true }, FIRST_RUN)).toBe(true);
  });

  it("sleeps 30 days after the first dismissal, then comes back", () => {
    const dismissedAt = FIRST_RUN + DAY_MS;
    const snoozed = hintDismissed(firstRun(), dismissedAt);
    expect(dueAt(snoozed, dismissedAt)).toBe(false);
    expect(dueAt(snoozed, dismissedAt + INSTALL_HINT_SNOOZE_MS - 1)).toBe(
      false,
    );
    expect(dueAt(snoozed, dismissedAt + INSTALL_HINT_SNOOZE_MS)).toBe(true);
  });

  it("stays away for good after the second dismissal", () => {
    const first = FIRST_RUN + DAY_MS;
    const second = first + INSTALL_HINT_SNOOZE_MS;
    const final = hintDismissed(hintDismissed(firstRun(), first), second);
    expect(final.dismissals).toBe(2);
    expect(dueAt(final, second + 10 * INSTALL_HINT_SNOOZE_MS)).toBe(false);
    expect(dueAt({ ...final, taskDone: true }, second + 400 * DAY_MS)).toBe(
      false,
    );
  });

  it("is never due once the app is installed", () => {
    const installed = { ...firstRun(), taskDone: true, installed: true };
    expect(dueAt(installed, FIRST_RUN + 2 * DAY_MS)).toBe(false);
  });
});

describe("the stored schedule", () => {
  it("reads a value in another shape as never dismissed", () => {
    for (const foreign of [
      "yes",
      42,
      [],
      { dismissals: "2", snoozedUntil: "soon", firstDay: "tomorrow" },
      { dismissals: Number.NaN, snoozedUntil: Number.POSITIVE_INFINITY },
    ]) {
      expect(parseInstallHint(foreign)).toEqual(NEVER_SEEN);
    }
  });

  it("reads text that is not JSON as never seen, and starts the first day there", async () => {
    writeStored(STORAGE_KEYS.installHint, "{not json");
    const store = await import("./installhint");
    store.noteShellDay(dayOf(FIRST_RUN));
    const kept = JSON.parse(readStored(STORAGE_KEYS.installHint) ?? "null");
    expect(store.parseInstallHint(kept)).toEqual({
      ...NEVER_SEEN,
      firstDay: dayOf(FIRST_RUN),
    });
  });

  it("keeps a dismissal on this device for the next page", async () => {
    const before = await import("./installhint");
    before.noteShellDay(dayOf(FIRST_RUN));
    before.noteHintDismissed(FIRST_RUN + DAY_MS);
    vi.resetModules();
    const after = await import("./installhint");
    const { renderHook } = await import("@testing-library/react");
    const record = renderHook(() => after.useInstallHintRecord()).result
      .current;
    expect(record.dismissals).toBe(1);
    expect(record.snoozedUntil).toBe(
      FIRST_RUN + DAY_MS + INSTALL_HINT_SNOOZE_MS,
    );
  });
});

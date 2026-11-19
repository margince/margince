// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useSyncExternalStore } from "react";
import { type ISODate, isISODate } from "../format/datepattern";
import { DAY_MS } from "../format/timezone";
import {
  readStoredJson,
  STORAGE_KEYS,
  watchStored,
  writeStored,
} from "./storage";

export const INSTALL_HINT_SNOOZE_MS = 30 * DAY_MS;

/** The second dismissal is the reader's answer, not a "later". */
const FINAL_DISMISSAL = 2;

export type InstallHintRecord = Readonly<{
  firstDay: ISODate | null;
  taskDone: boolean;
  dismissals: number;
  snoozedUntil: number | null;
  installed: boolean;
}>;

const NEVER_SEEN: InstallHintRecord = {
  firstDay: null,
  taskDone: false,
  dismissals: 0,
  snoozedUntil: null,
  installed: false,
};

/** Field by field: a field another build wrote in another shape reads as unset. */
export function parseInstallHint(value: unknown): InstallHintRecord {
  if (typeof value !== "object" || value === null) {
    return NEVER_SEEN;
  }
  const field = (name: string): unknown =>
    Object.hasOwn(value, name) ? Reflect.get(value, name) : undefined;
  const firstDay = field("firstDay");
  const dismissals = field("dismissals");
  const snoozedUntil = field("snoozedUntil");
  return {
    firstDay:
      typeof firstDay === "string" && isISODate(firstDay) ? firstDay : null,
    taskDone: field("taskDone") === true,
    dismissals:
      typeof dismissals === "number" && Number.isInteger(dismissals)
        ? Math.min(Math.max(dismissals, 0), FINAL_DISMISSAL)
        : 0,
    snoozedUntil:
      typeof snoozedUntil === "number" && Number.isFinite(snoozedUntil)
        ? snoozedUntil
        : null,
    installed: field("installed") === true,
  };
}

/** Never on the first day this device ran the app, unless a task on Home was
 *  finished since; never while a dismissal snoozes it, nor after the last one. */
export function installHintDue(
  record: InstallHintRecord,
  today: ISODate,
  now: number,
): boolean {
  if (record.installed || record.dismissals >= FINAL_DISMISSAL) {
    return false;
  }
  if (record.snoozedUntil !== null && now < record.snoozedUntil) {
    return false;
  }
  if (record.taskDone) {
    return true;
  }
  return record.firstDay !== null && today > record.firstDay;
}

export function shellDayNoted(
  record: InstallHintRecord,
  today: ISODate,
): InstallHintRecord {
  return record.firstDay === null ? { ...record, firstDay: today } : record;
}

export function hintDismissed(
  record: InstallHintRecord,
  now: number,
): InstallHintRecord {
  return {
    ...record,
    dismissals: Math.min(record.dismissals + 1, FINAL_DISMISSAL),
    snoozedUntil: now + INSTALL_HINT_SNOOZE_MS,
  };
}

const listeners = new Set<() => void>();
let held: InstallHintRecord | null = null;

function read(): InstallHintRecord {
  held ??=
    readStoredJson(STORAGE_KEYS.installHint, parseInstallHint) ?? NEVER_SEEN;
  return held;
}

function notify(): void {
  for (const listener of listeners) {
    listener();
  }
}

// Held once written, so a browser that refuses storage still keeps the
// reader's answer until the next page.
function change(next: (record: InstallHintRecord) => InstallHintRecord): void {
  const current = read();
  const changed = next(current);
  if (changed === current) {
    return;
  }
  held = changed;
  writeStored(STORAGE_KEYS.installHint, JSON.stringify(changed));
  notify();
}

watchStored(STORAGE_KEYS.installHint, () => {
  held = null;
  notify();
});

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function useInstallHintRecord(): InstallHintRecord {
  return useSyncExternalStore(subscribe, read, read);
}

export function noteShellDay(today: ISODate): void {
  change((record) => shellDayNoted(record, today));
}

export function noteHomeTaskDone(): void {
  change((record) =>
    record.taskDone ? record : { ...record, taskDone: true },
  );
}

export function noteHintDismissed(now: number): void {
  change((record) => hintDismissed(record, now));
}

export function noteInstalled(): void {
  change((record) =>
    record.installed ? record : { ...record, installed: true },
  );
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readdirSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import * as Sweep from "../../scripts/lib/activity-precision.sweep";
import { inWorker } from "../../scripts/lib/in-worker";
import { filesUnder } from "../../scripts/lib/source-tree";

// A `day` activity knows its date and no time. Its time reaches a reader in a
// zone only through activitywhen.ts, which prints its UTC date and no clock.
//
// A reader in a zone is derived, never listed. It is the format call of an
// Intl.DateTimeFormat, or a Date's toLocale*String. In format/ it is also a
// function that puts an hour or minute count into its sentence, or builds a
// RelativeTimeFormat. The value is followed through calls, props, fields,
// object literals, bindings, assignments, `.map` callbacks and element reads.
// It is followed through `new Date`, `Date.parse`, `Math`, arithmetic and
// template text too. A call returning a boolean hands it nowhere.
//
// A `due_at` is an activity time only on a row that can be a meeting. A
// condition on the same row's `source` that rules the meeting out ends it.
//
// A field `x_at` is an activity time too where its record carries
// `x_precision`. A precision handed to the owner must trace to a wire
// precision. Or it is `instant` for an `x_precision` the wire left out, handed
// beside that record's own `x_at`. Any other literal passes
// only when the time it is paired with reads no activity time. That is the
// sibling reaching the same owner call, or else that call's own time.
//
// Not followed, each a known blind spot:
// - a value kept in a Map or pushed into an array, and read back;
// - ISO text cut by hand, as in `at.slice(0, 10)`;
// - a Date's own getters, as in `getDate()`;
// - a comparison against now or another day, which the owner's predicates answer;
// - a projection that carries an activity time without its precision.

const srcRoot = join(dirname(fileURLToPath(import.meta.url)), "..");
const { shipped } = Sweep;
// The census type-checks the tree. It runs off the coverage run: see in-worker.ts.
const SWEEP = new URL(
  "../../scripts/lib/activity-precision.sweep.ts",
  import.meta.url,
);

const { files, found } = await inWorker<typeof Sweep, "sweep">(SWEEP, "sweep");

describe("an activity's time reaches a clock only through its precision", () => {
  it("reads every folder of the tree, extension screens included", () => {
    const read = new Set(
      files
        .filter((path) => path.startsWith(srcRoot))
        .map((path) => relative(srcRoot, path).split(/[/\\]/)[0]),
    );
    const listed = readdirSync(srcRoot, { withFileTypes: true })
      .filter((entry) => entry.isDirectory())
      .map((entry) => entry.name)
      .filter((name) => filesUnder(join(srcRoot, name)).some(shipped));
    expect(listed.length).toBeGreaterThan(3);
    expect([...read].filter((name) => !name.includes("."))).toEqual(
      expect.arrayContaining(listed),
    );
    expect(files.some((path) => path.includes("/extensions/"))).toBe(true);
  });

  it("derives the formatters that read a time in a zone", () => {
    expect([...found.readers]).toEqual(
      expect.arrayContaining([
        "formatDate",
        "formatDayMonth",
        "formatDateTime",
        "formatTimeOfDay",
        "formatTimeRange",
        "formatRelativeTime",
        "formatElapsed",
        "calendarDay",
      ]),
    );
    expect(found.readers.has("formatNumber")).toBe(false);
  });

  it("judges every activity time it reads", () => {
    expect(found.unjudged).toEqual([]);
  });

  it("sees each shape of the defect, and passes the owner", async () => {
    const defects = [
      "formatDateTime(activity.occurred_at, 'en', zone);",
      "const at2 = activity.occurred_at; formatTimeOfDay(at2, 'en', zone);",
      "const when3 = (iso: string) => formatDateTime(iso, 'en', zone); when3(activity.occurred_at);",
      "function Clock4({ at }: { at: string }) { return formatTimeOfDay(at, 'en', zone); } void (<Clock4 at={activity.occurred_at} />);",
      "const row5: { atIso: string } = { atIso: activity.occurred_at }; formatDateTime(row5.atIso, 'en', zone);",
      "formatDateTime(item.occurred_at ?? '', 'en', zone);",
      "formatTimeOfDay(item.due_at ?? '', 'en', zone);",
      "function start8(a: Activity) { return a.occurred_at; } formatDateTime(start8(activity), 'en', zone);",
      "formatRelativeTime(activity.occurred_at ?? activity.created_at, 'en');",
      "function Card10({ when }: { when: (iso: string) => string }) { return when(activity.occurred_at); } void (<Card10 when={(iso) => formatDateTime(iso, 'en', zone)} />);",
      "const rows11: { atIso: string }[] = [activity].map((a) => ({ atIso: a.occurred_at })); formatDateTime(rows11[0]?.atIso ?? '', 'en', zone);",
      "const { occurred_at: at12 } = activity; formatDateTime(at12, 'en', zone);",
      "let at13 = ''; at13 = activity.occurred_at; formatTimeOfDay(at13, 'en', zone);",
      "formatElapsed(Date.now() - Date.parse(activity.occurred_at), t, 'en');",
      "formatElapsed(Math.max(0, Date.now() - new Date(activity.occurred_at).getTime()), t, 'en');",
      "new Date(activity.occurred_at).toLocaleTimeString('en');",
      "new Date(activity.occurred_at).toLocaleString('en');",
      "const clock18 = new Intl.DateTimeFormat('en', { hour: '2-digit' }); clock18.format(new Date(activity.occurred_at));",
      // biome-ignore lint/suspicious/noTemplateCurlyInString: the planted source holds a template literal
      "formatTimeOfDay(`${activity.occurred_at}`, 'en', zone);",
      "formatActivityWhen(activity.occurred_at, 'instant', 'en', zone);",
      "formatActivityWhen(activity.occurred_at, activity.occurred_precision ?? 'instant', 'en', zone);",
      "const row22 = { atIso: activity.occurred_at, atPrecision: 'instant' as const }; formatActivityWhen(row22.atIso, row22.atPrecision, 'en', zone);",
      "function When23(p: { at: string; precision: OccurredPrecision }) { return formatActivityWhen(p.at, p.precision, 'en', zone); } void (<When23 at={activity.occurred_at} precision='instant' />);",
      "const when24 = (iso: string) => formatDateTime(iso, 'en', zone); function Card24({ when }: { when: (iso: string) => string }) { return when(activity.occurred_at); } void (<Card24 when={when24} />);",
      "function Inner25({ when }: { when: (iso: string) => string }) { return when(activity.occurred_at); } function Outer25({ when }: { when: (iso: string) => string }) { return <Inner25 when={when} />; } void (<Outer25 when={(iso) => formatTimeOfDay(iso, 'en', zone)} />);",
      "activityAgo({ at: activity.occurred_at, precision: 'instant' }, Date.now(), t, 'en', zone);",
      "function instantOf(): OccurredPrecision { return 'instant'; } formatActivityWhen(activity.occurred_at, instantOf(), 'en', zone);",
      "const KNOWN = { precision: 'instant' } as const; formatActivityWhen(activity.occurred_at, KNOWN.precision, 'en', zone);",
      "function moment29(at: string) { return { at, precision: 'instant' as const }; } activityAgo(moment29(activity.occurred_at), Date.now(), t, 'en', zone);",
      "const times30 = [activity].map((a) => a.occurred_at); formatDateTime(times30[0] ?? '', 'en', zone);",
      "formatDate(activity.occurred_at, 'en', zone);",
      "new Date(activity.occurred_at).toLocaleString('en', { month: 'long' });",
      "new Date(activity.occurred_at).toLocaleDateString('en');",
      "calendarDay(new Date(activity.occurred_at), zone);",
      "const day35 = new Intl.DateTimeFormat('en', { day: 'numeric' }); day35.format(new Date(activity.occurred_at));",
      "void (item.source !== 'task' ? formatDate(item.due_at ?? '', 'en', zone) : '');",
      "formatDateTime(touch.last_inbound_at ?? '', 'en', zone);",
      "formatActivityWhen(touch.last_inbound_at ?? '', touch.last_inbound_precision ?? 'day', 'en', zone);",
      "formatActivityWhen(touch.last_inbound_at ?? '', touch.last_outbound_precision ?? 'instant', 'en', zone);",
      "formatActivityWhen(touch.last_inbound_at ?? '', touch.summary?.last_inbound_precision ?? 'instant', 'en', zone);",
    ];
    const sound = [
      "formatActivityWhen(activity.occurred_at, activity.occurred_precision, 'en', zone);",
      "activityAgo({ at: activity.occurred_at, precision: activity.occurred_precision }, Date.now(), t, 'en', zone);",
      "activityDay(activity.occurred_at, activity.occurred_precision, 'en', zone, formatDayMonth);",
      "formatDateTime(activity.created_at, 'en', zone);",
      "formatDateTime(activity.due_at ?? '', 'en', zone);",
      "const row42 = { atIso: activity.created_at, atPrecision: 'instant' as const }; formatActivityWhen(row42.atIso, row42.atPrecision, 'en', zone);",
      "function instantOf43(): OccurredPrecision { return 'instant'; } formatActivityWhen(activity.created_at, instantOf43(), 'en', zone);",
      "void (item.source === 'task' ? formatDateTime(item.due_at ?? '', 'en', zone) : '');",
      "void (item.source === 'meeting' ? '' : formatTimeOfDay(item.due_at ?? '', 'en', zone));",
      "if (item.source === 'weekly_commitment') formatDate(item.due_at ?? '', 'en', zone);",
      "formatActivityWhen(touch.last_inbound_at ?? '', touch.last_inbound_precision ?? 'instant', 'en', zone);",
    ];
    expect(
      await inWorker<typeof Sweep, "plant">(SWEEP, "plant", [
        ...defects,
        ...sound,
      ]),
    ).toEqual(defects.map((_, index) => index + 1));
  });

  it("finds no reader left that draws a day entry's clock", () => {
    expect(found.findings).toEqual([]);
  });

  it("finds no owner call handed a precision it cannot trace", () => {
    expect(found.unproven).toEqual([]);
  });
});

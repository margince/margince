// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plant, shipped } from "../../scripts/lib/activity-precision.sweep";
import { srcDir, treeOf } from "../tree";
import { judge, rule } from "./activity-precision.census";

describe("an activity's time reaches a clock only through its precision", () => {
  it("sees each shape of the defect, and passes the owner", () => {
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
    expect(plant([...defects, ...sound])).toEqual(
      defects.map((_, index) => index + 1),
    );
  });
});

describe("the activity-precision rule", () => {
  // A planted screen beside the real format/ modules, from which the rule
  // derives the readers in a zone.
  const screen = join(srcDir, "features", "planted", "plantedclock.tsx");
  const formatDir = join(srcDir, "format");
  const formatModules = readdirSync(formatDir)
    .map((name) => join(formatDir, name))
    .filter(shipped)
    .map((path) => ({
      path,
      area: "src" as const,
      text: readFileSync(path, "utf8"),
    }));
  const planted = (body: string) => {
    const tree = treeOf([
      ...formatModules,
      {
        path: screen,
        area: "src",
        text: [
          'import type { components } from "../../api/schema";',
          'import { formatDateTime } from "../../format/format";',
          'import { formatActivityWhen } from "../../format/activitywhen";',
          'declare const activity: components["schemas"]["Activity"];',
          "void [formatDateTime, formatActivityWhen];",
          body,
        ].join("\n"),
      },
    ]);
    return judge(rule.corpus(tree), tree).filter((finding) =>
      finding.includes("plantedclock.tsx"),
    );
  };

  it("reports a day entry's time drawn on a clock", () => {
    expect(
      planted(
        "export const at = formatDateTime(activity.occurred_at, 'en', 'UTC');",
      ),
    ).toHaveLength(1);
  });

  it("passes the time drawn through its own precision", () => {
    expect(
      planted(
        "export const at = formatActivityWhen(activity.occurred_at, activity.occurred_precision, 'en', 'UTC');",
      ),
    ).toEqual([]);
  });
});

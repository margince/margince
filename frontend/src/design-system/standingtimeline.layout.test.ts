// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import {
  axisOf,
  axisPct,
  BREAK_AT,
  clusterEvents,
  groupMarks,
  instant,
  isWeekly,
  LABEL_MAX_PX,
  LABEL_ROWS,
  type LabelBox,
  labelSpan,
  labelWidthPx,
  MARK_PX,
  MAX_LABELS,
  pct,
  placeLabels,
  rangeOf,
  type TimelineKind,
  TODAY_CLEAR_PX,
  tickLabelsShown,
  ticksOf,
} from "./standingtimeline.layout";

const day = (iso: string) => instant(iso);

describe("the axis's span", () => {
  it("runs from the first month's first day to the last month's last day", () => {
    const range = rangeOf([
      day("2026-06-03"),
      day("2026-09-30"),
      day("2026-08-13"),
    ]);
    expect(new Date(range.start).toISOString()).toBe(
      "2026-06-01T00:00:00.000Z",
    );
    expect(new Date(range.end).toISOString()).toBe("2026-09-30T00:00:00.000Z");
    expect(isWeekly(range)).toBe(false);
  });

  it("counts whole weeks, four at least, for a record under six weeks old", () => {
    const range = rangeOf([day("2026-08-02"), day("2026-08-15")]);
    expect(new Date(range.start).toISOString()).toBe(
      "2026-08-01T00:00:00.000Z",
    );
    expect(new Date(range.end).toISOString()).toBe("2026-08-29T00:00:00.000Z");
    expect(isWeekly(range)).toBe(true);
  });

  it("reads a date alone as its UTC midnight", () => {
    expect(instant("2026-08-13")).toBe(Date.UTC(2026, 7, 13));
  });
});

describe("where a moment sits", () => {
  const range = { start: day("2026-08-01"), end: day("2026-08-11") };

  it("is its share of the span", () => {
    expect(pct(day("2026-08-06"), range)).toBe(50);
  });

  it("is held to the axis", () => {
    expect(pct(day("2026-07-01"), range)).toBe(0);
    expect(pct(day("2026-09-01"), range)).toBe(100);
  });
});

describe("the grid lines", () => {
  it("mark each month on a span of months", () => {
    const ticks = ticksOf(rangeOf([day("2026-06-03"), day("2026-09-30")]));
    expect(ticks.map((tick) => new Date(tick.at).getUTCMonth() + 1)).toEqual([
      6, 7, 8, 9,
    ]);
    expect(ticks[0]?.pct).toBe(0);
  });

  it("mark every other month past seven months", () => {
    const ticks = ticksOf(rangeOf([day("2026-03-01"), day("2026-11-30")]));
    expect(ticks.map((tick) => new Date(tick.at).getUTCMonth() + 1)).toEqual([
      3, 5, 7, 9, 11,
    ]);
  });

  it("mark each week on a span of weeks", () => {
    const ticks = ticksOf(rangeOf([day("2026-08-02"), day("2026-08-15")]));
    expect(ticks.map((tick) => new Date(tick.at).getUTCDate())).toEqual([
      1, 8, 15, 22, 29,
    ]);
  });
});

describe("the room after today", () => {
  it("leaves a week at least, so today never stands on the axis's end", () => {
    const today = day("2026-10-21");
    const { range } = axisOf(today, [day("2026-10-02"), day("2026-10-20")]);
    expect(pct(today, range)).toBeLessThan(90);
  });

  it("still reaches the last date ahead", () => {
    const axis = axisOf(day("2026-08-13"), [
      day("2026-06-03"),
      day("2026-09-30"),
    ]);
    expect(new Date(axis.range.end).toISOString()).toBe(
      "2026-09-30T00:00:00.000Z",
    );
    expect(axis.far).toBeUndefined();
  });
});

describe("a date far ahead", () => {
  const today = day("2026-10-30");
  const recent = [day("2026-09-07"), day("2026-09-23")];
  const close = day("2029-08-29");

  it("does not stretch the axis: the span stays on the recent months", () => {
    const axis = axisOf(today, [...recent, close]);
    expect(new Date(axis.range.start).toISOString()).toBe(
      "2026-09-01T00:00:00.000Z",
    );
    expect(axis.range.end).toBeLessThan(day("2027-01-01"));
    expect(axis.far).toBe(close);
  });

  it("sits past the break at the right end, the dated span before it", () => {
    const axis = axisOf(today, [...recent, close]);
    expect(axisPct(close, axis)).toBe(100);
    expect(axisPct(axis.range.end, axis)).toBe(BREAK_AT);
    expect(axisPct(today, axis)).toBeLessThan(BREAK_AT);
    expect(axisPct(day("2026-09-23"), axis)).toBeGreaterThan(15);
  });

  it("keeps an instant late on the span's last day on the span", () => {
    const axis = axisOf(day("2026-10-30"), [
      day("2026-09-07"),
      day("2026-11-30T17:00:00Z"),
    ]);
    expect(axis.far).toBeUndefined();
    expect(axisPct(day("2026-11-30T17:00:00Z"), axis)).toBe(100);
  });

  it("is drawn to scale while it is no further ahead than the line looks back", () => {
    const axis = axisOf(today, [day("2025-10-01"), day("2027-06-30")]);
    expect(axis.far).toBeUndefined();
    expect(axisPct(day("2027-06-30"), axis)).toBe(
      pct(day("2027-06-30"), axis.range),
    );
  });
});

const CANVAS = 760;
const box = (
  pct: number,
  kind: TimelineKind = "done",
  titleChars = 12,
  subChars = 12,
): LabelBox => ({ pct, kind, titleChars, subChars });

describe("the dots that share a label", () => {
  it("are the ones closer than the cluster width, led by the most telling", () => {
    const groups = clusterEvents(
      [
        { pct: 10, kind: "done" },
        { pct: 11, kind: "late" },
        { pct: 12, kind: "done" },
        { pct: 40, kind: "done" },
      ],
      CANVAS,
    );
    expect(groups).toEqual([
      { members: [0, 1, 2], lead: 1 },
      { members: [3], lead: 3 },
    ]);
  });

  it("lead with the latest of events of one kind", () => {
    const [group] = clusterEvents(
      [
        { pct: 50, kind: "done" },
        { pct: 51, kind: "done" },
      ],
      CANVAS,
    );
    expect(group?.lead).toBe(1);
  });
});

describe("the label rows", () => {
  const rows = (labels: readonly LabelBox[]) =>
    placeLabels(labels, CANVAS).map((place) => place?.row ?? null);
  const sides = (labels: readonly LabelBox[]) =>
    placeLabels(labels, CANVAS).map((place) => place?.hangsLeft ?? null);

  it("keep labels far apart on the first row", () => {
    expect(rows([box(5), box(40), box(80)])).toEqual([0, 0, 0]);
  });

  it("step a label down when the one before it has not ended", () => {
    expect(rows([box(10, "done", 20), box(35), box(60)])).toEqual([0, 0, 0]);
    expect(rows([box(10, "done", 20), box(14), box(50)])).toEqual([1, 0, 0]);
  });

  it("leave a label off rather than open a fourth row", () => {
    const places = rows([
      box(10, "done", 30),
      box(12, "done", 30),
      box(14, "done", 30),
      box(16, "done", 30),
    ]);
    expect(places.filter((row) => row !== null)).toHaveLength(LABEL_ROWS);
    expect(Math.max(...places.map((row) => row ?? 0))).toBeLessThan(LABEL_ROWS);
  });

  it("name an overdue promise before an older meeting it crowds", () => {
    const places = rows([
      box(40, "done", 30),
      box(42, "done", 30),
      box(44, "done", 30),
      box(46, "late", 30),
    ]);
    expect(places[3]).toBe(0);
  });

  it("hang left near the end, or where the words would run off the axis", () => {
    expect(sides([box(92, "done", 4, 4)])).toEqual([false]);
    expect(sides([box(92.5, "done", 4, 4)])).toEqual([true]);
    expect(sides([box(85, "done", 30)])).toEqual([true]);
  });

  it("place at most six labels on a crowded axis, none on another or on a lead line", () => {
    const crowd: [number, TimelineKind][] = [
      ...Array.from({ length: 10 }, (_, index): [number, TimelineKind] => [
        30 + index * 1.5,
        "done",
      ]),
      [52, "late"],
      [56, "last"],
      [80, "ahead"],
      [100, "end"],
    ];
    const kinds = crowd.map(([, kind]) => kind);
    const labels = crowd.map(([at, kind]) => box(at, kind, 16, 16));
    const places = placeLabels(labels, CANVAS);
    const shown = labels.flatMap((label, index) => {
      const place = places[index];
      return place
        ? [
            {
              row: place.row,
              span: labelSpan(label, CANVAS, place.hangsLeft),
              at: (label.pct / 100) * CANVAS,
            },
          ]
        : [];
    });
    expect(shown.length).toBeLessThanOrEqual(MAX_LABELS);
    expect(shown.every((label) => label.row < LABEL_ROWS)).toBe(true);
    for (const a of shown) {
      for (const b of shown) {
        if (a === b) continue;
        if (a.row === b.row) {
          expect(a.span[0] < b.span[1] && b.span[0] < a.span[1]).toBe(false);
        }
        if (b.row > a.row) {
          expect(a.span[0] < b.at && b.at < a.span[1]).toBe(false);
        }
      }
      expect(a.span[0]).toBeGreaterThanOrEqual(0);
      expect(a.span[1]).toBeLessThanOrEqual(CANVAS + 16);
    }
    const named = kinds.filter((_, index) => places[index] !== null);
    expect(named).toEqual(
      expect.arrayContaining(["late", "last", "ahead", "end"]),
    );
  });
});

describe("the grid-line labels", () => {
  it("are all drawn when the lines are far apart", () => {
    const ticks = [0, 25, 50, 75].map((at) => ({ pct: at, chars: 8 }));
    expect(tickLabelsShown(ticks, 760)).toEqual([true, true, true, true]);
  });

  it("give up the axis's end, which hangs left, when it would run into the label before", () => {
    const ticks = [
      { pct: 80, chars: 8 },
      { pct: 100, chars: 7 },
    ];
    expect(tickLabelsShown(ticks, 400)).toEqual([true, false]);
  });
});

describe("a label's width", () => {
  it("stops at the cap, so one long title cannot take a row to itself", () => {
    const long = box(10, "done", 80, 80);
    expect(labelWidthPx(long)).toBeLessThanOrEqual(LABEL_MAX_PX + 16);
    expect(labelWidthPx(box(10, "done", 5, 5))).toBeLessThan(
      labelWidthPx(long),
    );
  });
});

describe("the mark lane", () => {
  const canvas = 760;

  it("keeps every chip at least a chip's width from the next", () => {
    const groups = groupMarks([5, 6, 7, 20, 21, 40, 41, 42, 60], canvas, 90);
    groups.slice(1).forEach((group, index) => {
      expect(group.px - (groups[index]?.px ?? 0)).toBeGreaterThanOrEqual(
        MARK_PX,
      );
    });
  });

  it("shares one chip between marks closer than a chip, standing where the oldest does", () => {
    const [first, second] = groupMarks([10, 10.5, 30], canvas, 90);
    expect(first?.members).toEqual([0, 1]);
    expect(first?.px).toBeCloseTo(76, 0);
    expect(second?.members).toEqual([2]);
  });

  it("holds marks near Today back from its pill", () => {
    const now = 80;
    const groups = groupMarks([60, 78, 79.5], canvas, now);
    const nowPx = (now / 100) * canvas;
    groups.forEach((group) => {
      expect(group.px).toBeLessThanOrEqual(nowPx - TODAY_CLEAR_PX);
    });
    expect(groups.at(-1)?.members).toEqual([1, 2]);
  });

  it("keeps a mark at the axis's start on the canvas", () => {
    expect(groupMarks([0], canvas, 90)[0]?.px).toBe(MARK_PX / 2);
  });

  it("draws nothing for no marks", () => {
    expect(groupMarks([], canvas, 90)).toEqual([]);
  });
});

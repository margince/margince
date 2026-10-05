import { describe, expect, it } from "vitest";
import { line, receipt } from "./magic.testkit";
import { axisTicks, placeMarks } from "./magic.timeline";

describe("the strip's clock", () => {
  it("names whole hours across a night, and a midnight by its day", () => {
    expect(
      axisTicks("2026-09-12T08:00:00Z", "2026-09-13T08:00:00Z", "UTC").map(
        (tick) => [tick.iso, tick.day],
      ),
    ).toEqual([
      ["2026-09-12T12:00:00.000Z", false],
      ["2026-09-12T18:00:00.000Z", false],
      ["2026-09-13T00:00:00.000Z", true],
      ["2026-09-13T06:00:00.000Z", false],
    ]);
  });

  it("counts hours on the reader's clock rather than the server's", () => {
    // 17:40 to 10:05 in Berlin: every third hour of THEIR evening and night,
    // with the ends left clear for the start and "Now" labels.
    expect(
      axisTicks(
        "2026-09-12T15:40:00Z",
        "2026-09-13T08:05:00Z",
        "Europe/Berlin",
      ).map((tick) => tick.iso),
    ).toEqual([
      "2026-09-12T19:00:00.000Z",
      "2026-09-12T22:00:00.000Z",
      "2026-09-13T01:00:00.000Z",
      "2026-09-13T04:00:00.000Z",
    ]);
  });

  it("ticks on the reader's whole hours where their zone is not a whole hour off UTC", () => {
    // Kathmandu is UTC+5:45, so its midnight falls at a quarter past a UTC hour.
    expect(
      axisTicks(
        "2026-09-12T08:00:00Z",
        "2026-09-13T08:00:00Z",
        "Asia/Kathmandu",
      ).map((tick) => [tick.iso, tick.day]),
    ).toEqual([
      ["2026-09-12T12:15:00.000Z", false],
      ["2026-09-12T18:15:00.000Z", true],
      ["2026-09-13T00:15:00.000Z", false],
    ]);
  });

  it("names each day across a week", () => {
    expect(
      axisTicks("2026-09-06T08:05:00Z", "2026-09-13T08:05:00Z", "UTC").map(
        (tick) => [tick.iso, tick.day],
      ),
    ).toEqual([
      ["2026-09-08T00:00:00.000Z", true],
      ["2026-09-09T00:00:00.000Z", true],
      ["2026-09-10T00:00:00.000Z", true],
      ["2026-09-11T00:00:00.000Z", true],
      ["2026-09-12T00:00:00.000Z", true],
    ]);
  });

  it("names every seventh day across a month, from the first midnight", () => {
    expect(
      axisTicks("2026-08-14T08:05:00Z", "2026-09-13T08:05:00Z", "UTC").map(
        (tick) => tick.iso,
      ),
    ).toEqual([
      "2026-08-22T00:00:00.000Z",
      "2026-08-29T00:00:00.000Z",
      "2026-09-05T00:00:00.000Z",
    ]);
  });

  it("draws no axis over a window with no length", () => {
    expect(
      axisTicks("2026-09-13T08:00:00Z", "2026-09-13T08:00:00Z", "UTC"),
    ).toEqual([]);
  });
});

describe("the strip's marks", () => {
  it("stands a mark on one already there rather than hiding it", () => {
    const marks = placeMarks(
      receipt({
        done: [
          line({ id: "00000000-0000-7000-8000-000000000001" }),
          line({ id: "00000000-0000-7000-8000-000000000002" }),
        ],
      }),
    );
    expect(marks.map((mark) => mark.bottom)).toEqual([0, 12]);
  });

  it("sets two bulk jobs side by side on the baseline, where height is read", () => {
    const marks = placeMarks(
      receipt({
        done: [
          line({ id: "00000000-0000-7000-8000-000000000001", count: 42 }),
          line({ id: "00000000-0000-7000-8000-000000000002", count: 5 }),
        ],
      }),
    );
    expect(marks.map((mark) => [mark.bottom, mark.nudge])).toEqual([
      [0, 0],
      [0, 8],
    ]);
  });

  it("sets a bulk job beside a dot already on the baseline rather than over it", () => {
    const marks = placeMarks(
      receipt({
        done: [
          line({ id: "00000000-0000-7000-8000-000000000001" }),
          line({ id: "00000000-0000-7000-8000-000000000002", count: 42 }),
        ],
      }),
    );
    // Half the dot, the gap, half the bar: the bar's edge clears the dot's.
    expect(marks.map((mark) => [mark.bottom, mark.nudge])).toEqual([
      [0, 0],
      [0, 10],
    ]);
  });

  it("holds a line from before the window at its start", () => {
    const [mark] = placeMarks(
      receipt({ done: [line({ occurred_at: "2026-09-11T08:00:00Z" })] }),
    );
    expect(mark.at).toBe(0);
  });

  it("draws a job over many records taller than a single change", () => {
    // Placed in time order: the bulk job ran the evening before the change.
    const [many, single] = placeMarks(
      receipt({
        done: [
          line({ id: "00000000-0000-7000-8000-000000000001" }),
          line({
            id: "00000000-0000-7000-8000-000000000002",
            occurred_at: "2026-09-12T20:00:00Z",
            count: 1200,
          }),
        ],
      }),
    );
    expect(many.line.count).toBe(1200);
    expect(many.height).toBeGreaterThan(single.height);
  });
});

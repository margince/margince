import { describe, expect, it } from "vitest";
import { formatDateTime } from "../format/format";
import { viewerZone } from "../format/timezone";
import { translate } from "../i18n";
import { row } from "../testing/worklist.testkit";
import { whenText } from "./worklistcopy";
import { scheduledText } from "./worklistheld";

const zone = viewerZone();

const t = (key: "worklist.when.scheduled", values: { when: string }) =>
  translate("en", key, values);

describe("scheduledText — when a meeting owing an outcome was scheduled", () => {
  it("names the booked time, not a held one", () => {
    const item = row({
      source: "meeting_outcome",
      occurred_at: "2026-09-22T09:00:00Z",
    });
    expect(scheduledText(item, t, "en", zone)).toBe(
      `scheduled ${formatDateTime("2026-09-22T09:00:00Z", "en", zone)}`,
    );
  });

  it("names a date-only meeting's own date, and no time", () => {
    // Noon UTC on the 15th is already the 16th in Kiritimati.
    const item = row({
      source: "meeting_outcome",
      occurred_at: "2026-09-15T12:00:00Z",
      occurred_precision: "day",
    });
    expect(scheduledText(item, t, "en", "Pacific/Kiritimati")).toBe(
      "scheduled 15/09/2026",
    );
  });

  it("says nothing for any other row", () => {
    const item = row({ source: "task", occurred_at: "2026-09-22T09:00:00Z" });
    expect(scheduledText(item, t, "en", zone)).toBeNull();
  });
});

// A meeting row's start is drawn on the reader's clock. A meeting known only
// by its date names that date, so every colleague sees its day.
describe("whenText — when a meeting starts", () => {
  const say = (
    key: Parameters<typeof translate>[1],
    values?: Record<string, string>,
  ) => translate("en", key, values);
  // Noon UTC on the 15th, read by a colleague in Pago Pago at 09:00 that day.
  const meeting = (precision: "day" | "instant") =>
    row({
      source: "meeting",
      due_at: "2026-09-15T12:00:00Z",
      occurred_precision: precision,
    });
  const RECORD = "Pacific/Kiritimati";
  const VIEWER = "Pacific/Pago_Pago";

  it("names a date-only meeting's date, even on its day", () => {
    const sameDay = new Date("2026-09-15T20:00:00Z");
    expect(whenText(meeting("day"), say, "en", VIEWER, RECORD, sameDay)).toBe(
      say("worklist.when.starts", { when: "15/09/2026" }),
    );
  });

  it("names a timed meeting's clock in the reader's zone", () => {
    const sameDay = new Date("2026-09-15T20:00:00Z");
    expect(
      whenText(meeting("instant"), say, "en", VIEWER, RECORD, sameDay),
    ).toBe(say("worklist.when.starts", { when: "01:00" }));
  });
});

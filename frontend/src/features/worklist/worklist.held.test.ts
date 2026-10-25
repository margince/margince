import { describe, expect, it } from "vitest";
import { formatDateTime } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { translate } from "../../i18n";
import { row } from "../../testing/worklist.testkit";
import { whenText } from "./worklist.copy";
import { scheduledText } from "./worklist.held";

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

  it("names a date-only meeting's date on the record's clock, and no time", () => {
    // Noon on the 15th in Kiritimati is still the 14th in UTC.
    const item = row({
      source: "meeting_outcome",
      occurred_at: "2026-09-14T22:00:00Z",
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
// by its date is read on the record's, so every colleague sees its day.
describe("whenText — when a meeting starts", () => {
  const say = (
    key: Parameters<typeof translate>[1],
    values?: Record<string, string>,
  ) => translate("en", key, values);
  // Noon on the 15th in Kiritimati, read by a colleague in Pago Pago.
  const meeting = (precision: "day" | "instant") =>
    row({
      source: "meeting",
      due_at: "2026-09-14T22:00:00Z",
      occurred_precision: precision,
    });
  const RECORD = "Pacific/Kiritimati";
  const VIEWER = "Pacific/Pago_Pago";

  it("names a date-only meeting's date in the record zone, even on its day", () => {
    const sameDay = new Date("2026-09-14T23:00:00Z");
    expect(whenText(meeting("day"), say, "en", VIEWER, RECORD, sameDay)).toBe(
      say("worklist.when.starts", { when: "15/09/2026" }),
    );
  });

  it("names a timed meeting's clock in the reader's zone", () => {
    const sameDay = new Date("2026-09-14T23:00:00Z");
    expect(
      whenText(meeting("instant"), say, "en", VIEWER, RECORD, sameDay),
    ).toBe(say("worklist.when.starts", { when: "11:00" }));
  });
});

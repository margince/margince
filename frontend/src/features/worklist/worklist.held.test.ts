import { describe, expect, it } from "vitest";
import { formatDateTime } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { translate } from "../../i18n";
import { row } from "../../testing/worklist.testkit";
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

  it("says nothing for any other row", () => {
    const item = row({ source: "task", occurred_at: "2026-09-22T09:00:00Z" });
    expect(scheduledText(item, t, "en", zone)).toBeNull();
  });
});

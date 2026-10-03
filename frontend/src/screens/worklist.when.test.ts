import { describe, expect, it } from "vitest";
import { viewerZone } from "../format/timezone";
import { translate } from "../i18n";
import { row } from "./worklist.testkit";
import { heldText } from "./worklist.when";

const t = (key: "worklist.when.held", values: { when: string }) =>
  translate("en", key, values);

describe("heldText — when a meeting owing an outcome took place", () => {
  it("names when the meeting was held", () => {
    const item = row({
      source: "meeting_outcome",
      occurred_at: "2026-09-22T09:00:00Z",
    });
    expect(heldText(item, t, "en", viewerZone())).toMatch(/^held .*2026/);
  });

  it("says nothing for any other row", () => {
    const item = row({ source: "task", occurred_at: "2026-09-22T09:00:00Z" });
    expect(heldText(item, t, "en", viewerZone())).toBeNull();
  });
});

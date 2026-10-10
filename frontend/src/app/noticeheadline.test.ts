import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { en } from "../i18n/en";
import { FIELD_PAIRS, noticeHeadline } from "./noticeheadline";

// The server's own rendering of a staged change's fields, held equal to its
// producer by compose's agentsummarypairs_test.go.
const PRODUCED = readFileSync(
  new URL("./noticepairs.fixture.txt", import.meta.url),
  "utf8",
)
  .trim()
  .split("\n");

const t = (key: keyof typeof en) => en[key];

describe("noticeHeadline", () => {
  it("recognises every line the server's summary builder produces", () => {
    expect(PRODUCED.length).toBeGreaterThan(0);
    for (const line of PRODUCED) expect(FIELD_PAIRS.test(line)).toBe(true);
  });

  it("heads an approval notice that is only field pairs by what it is", () => {
    const { headline, detail } = noticeHeadline(
      { kind: "approval_pending", subject: PRODUCED[0] ?? "" },
      t,
    );
    expect(headline).toBe("Approvals waiting on you");
    expect(detail).toBe(PRODUCED[0]);
  });

  it("leaves a sentence, and any other kind, as sent", () => {
    const sentence = "Fold one tag into another: into_tag_id=01a1";
    expect(
      noticeHeadline({ kind: "approval_pending", subject: sentence }, t),
    ).toEqual({ headline: sentence });
    expect(
      noticeHeadline(
        { kind: "automation_failed", subject: PRODUCED[0] ?? "" },
        t,
      ),
    ).toEqual({ headline: PRODUCED[0] });
  });
});

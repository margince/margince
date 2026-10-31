import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { en } from "../i18n/en";
import {
  FIELD_PAIRS,
  noticeHeadline,
  splitStagedSummary,
} from "./noticeheadline";

// The server's own rendering of a staged change's fields, held equal to its
// producer by compose's agentsummarypairs_test.go.
const PRODUCED = readFileSync(
  new URL("./noticepairs.fixture.txt", import.meta.url),
  "utf8",
)
  .trim()
  .split("\n");

const t = (key: keyof typeof en) => en[key];

// restSummary and stagedSentence put a sentence and ": " in front of the pairs
// (agentsummary_test.go pins the head "Update record deal").
const HEAD = "Update record deal";

describe("noticeHeadline", () => {
  it("recognises every line the server's summary builder produces", () => {
    expect(PRODUCED.length).toBeGreaterThan(0);
    for (const line of PRODUCED) expect(FIELD_PAIRS.test(line)).toBe(true);
  });

  it("heads an approval notice by its sentence and keeps the pairs as detail", () => {
    for (const pairs of PRODUCED) {
      expect(
        noticeHeadline(
          { kind: "approval_pending", subject: `${HEAD}: ${pairs}` },
          t,
        ),
      ).toEqual({ headline: HEAD, detail: pairs });
    }
  });

  it("heads an approval notice that is only field pairs by what it is", () => {
    const { headline, detail } = noticeHeadline(
      { kind: "approval_pending", subject: PRODUCED[0] ?? "" },
      t,
    );
    expect(headline).toBe("Approvals waiting on you");
    expect(detail).toBe(PRODUCED[0]);
  });

  it("leaves a plain sentence, and any other kind, as sent", () => {
    const sentence = "Is Ada Example a contact worth keeping?";
    expect(
      noticeHeadline({ kind: "approval_pending", subject: sentence }, t),
    ).toEqual({ headline: sentence });
    expect(
      noticeHeadline(
        { kind: "automation_failed", subject: `${HEAD}: ${PRODUCED[0]}` },
        t,
      ),
    ).toEqual({ headline: `${HEAD}: ${PRODUCED[0]}` });
  });
});

describe("splitStagedSummary", () => {
  it("keeps a colon inside the sentence with the sentence", () => {
    expect(
      splitStagedSummary(`Found on site: ${HEAD}: ${PRODUCED[0]}`),
    ).toEqual({ sentence: `Found on site: ${HEAD}`, fields: PRODUCED[0] });
  });

  it("reads the overflow suffix the server adds past eight fields", () => {
    for (const more of ["+1 more", "+1 weitere", "+1 trường khác"]) {
      expect(splitStagedSummary(`${HEAD}: a=1, b=2, ${more}`)).toEqual({
        sentence: HEAD,
        fields: `a=1, b=2, ${more}`,
      });
    }
  });

  it("reads a key in the reader's own language", () => {
    expect(
      splitStagedSummary("Tạo bản ghi bình luận: thuộc=thread-9, body=OK"),
    ).toEqual({
      sentence: "Tạo bản ghi bình luận",
      fields: "thuộc=thread-9, body=OK",
    });
  });

  it("leaves a summary it cannot read whole as it was", () => {
    const comma = `${HEAD}: note=Call the buyer, then send the quote`;
    expect(splitStagedSummary(comma)).toEqual({ sentence: comma });
  });
});

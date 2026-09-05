import { describe, expect, it } from "vitest";

import { asksWhy, contextFor, repliesToTheSubject } from "./compose-context";

// What the composer claims about a message, and when it claims nothing.
//
// The case these tests are built around is the one that costs a rep their day:
// a reply that asks a question the thread already answers. Every assertion here
// is about NOT prompting where the record speaks, and about prompting where it
// genuinely does not.

describe("repliesToTheSubject", () => {
  it("an inbound email the subject sent is a reply", () => {
    expect(repliesToTheSubject({ kind: "email", direction: "inbound" })).toBe(
      true,
    );
  });

  it("an inbound channel message is a reply too", () => {
    expect(repliesToTheSubject({ kind: "message", direction: "inbound" })).toBe(
      true,
    );
  });

  it("our OWN outbound mail is not the subject writing to us", () => {
    // The case a rep reaches by re-opening their own sent message. It looks
    // like a thread and reads like a reply, and the subject never answered —
    // so it is an unprompted follow-up, and the composer has to ask.
    expect(repliesToTheSubject({ kind: "email", direction: "outbound" })).toBe(
      false,
    );
  });

  it("a filed note is not correspondence, whatever its direction", () => {
    // A note is something a rep wrote about the person, not to them. Treating
    // it as a thread would let "I met them at a fair" authorize a cold mail.
    expect(repliesToTheSubject({ kind: "note", direction: "inbound" })).toBe(
      false,
    );
  });

  it("no anchor at all is not a reply", () => {
    expect(repliesToTheSubject(undefined)).toBe(false);
  });
});

describe("contextFor", () => {
  it("claims NOTHING when the anchor already says it is a reply", () => {
    // The engine derives reply_to_inbound from the thread. A claim on top could
    // only agree — adding noise to the decision record — or contradict it,
    // which is recorded as a claim the evidence does not carry.
    expect(
      contextFor({
        anchor: { kind: "email", direction: "inbound" },
        chosen: "",
      }),
    ).toBeUndefined();
  });

  it("keeps claiming nothing even if something was chosen earlier", () => {
    // A reader who picked a category and then opened a reply must not have
    // their stale pick travel: the thread is the stronger evidence and the
    // server would record the disagreement.
    expect(
      contextFor({
        anchor: { kind: "email", direction: "inbound" },
        chosen: "marketing",
      }),
    ).toBeUndefined();
  });

  it("sends what the reader chose when there is no anchor to derive from", () => {
    expect(
      contextFor({ anchor: undefined, chosen: "active_deal_followup" }),
    ).toBe("active_deal_followup");
  });

  it("sends nothing while an unanchored message is unanswered", () => {
    // Not a default. Omitting is what lets the engine answer from the record
    // rather than from a guess this screen made.
    expect(contextFor({ anchor: undefined, chosen: "" })).toBeUndefined();
  });
});

describe("asksWhy", () => {
  it("does not ask on a reply", () => {
    expect(asksWhy({ kind: "email", direction: "inbound" })).toBe(false);
  });

  it("asks on a first message", () => {
    expect(asksWhy(undefined)).toBe(true);
  });

  it("asks when the anchor is our own outbound mail", () => {
    expect(asksWhy({ kind: "email", direction: "outbound" })).toBe(true);
  });
});

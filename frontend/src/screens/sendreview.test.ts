import { describe, expect, it } from "vitest";
import { ProblemError } from "./common";
import { sendReviewOf } from "./sendreview";

// What a rep can do about a refused send, read from the refusal itself.
//
// The surface must not decide which actions exist: only the server knows who is
// asking and what state the review is in. What this file pins is that the
// reading is faithful — everything the server offered and nothing it did not.
describe("sendReviewOf", () => {
  const refusal = (details: unknown) =>
    new ProblemError({ code: "consent_not_granted", details });

  it("reads the review a refusal named", () => {
    const review = sendReviewOf(
      refusal({ review_id: "r-1", available_actions: ["request_decision"] }),
    );
    expect(review).toEqual({ reviewId: "r-1", actions: ["request_decision"] });
  });

  // An agent, or a refusal with no message to decide about. The reference still
  // travels so a person reading the transcript can pick the review up; what is
  // absent is the shortcut.
  it("keeps the reference when the server offers no action", () => {
    const review = sendReviewOf(
      refusal({ review_id: "r-2", available_actions: [] }),
    );
    expect(review).toEqual({ reviewId: "r-2", actions: [] });
  });

  // A server newer than this build names an action it has never heard of.
  // Dropping it is safe because the reference survives: the rep loses one
  // shortcut, not the review. Rendering the raw enum would put `send_anyway` in
  // front of a reader as a button label, which tells them nothing.
  it("drops an action this build cannot draw", () => {
    const review = sendReviewOf(
      refusal({
        review_id: "r-3",
        available_actions: ["send_anyway", "request_decision"],
      }),
    );
    expect(review?.actions).toEqual(["request_decision"]);
  });

  it("draws one button however often the server names it", () => {
    const review = sendReviewOf(
      refusal({
        review_id: "r-4",
        available_actions: ["request_decision", "request_decision"],
      }),
    );
    expect(review?.actions).toEqual(["request_decision"]);
  });

  // Most refusals this surface sees are not about consent at all: a
  // disconnected mailbox, a shared unsubscribe link. None of them holds a
  // message for anybody to decide about.
  it("answers nothing for a refusal that named no review", () => {
    expect(sendReviewOf(refusal({}))).toBeNull();
    expect(sendReviewOf(refusal(undefined))).toBeNull();
    expect(
      sendReviewOf(new ProblemError({ code: "mailbox_not_send_capable" })),
    ).toBeNull();
    expect(sendReviewOf(new Error("network"))).toBeNull();
  });

  // A body that disagrees with the contract is a body this surface must
  // survive. A rep whose send was refused should not also see a crash.
  it("survives a malformed body", () => {
    expect(sendReviewOf(refusal({ review_id: 7 }))).toBeNull();
    expect(sendReviewOf(refusal({ review_id: "" }))).toBeNull();
    expect(sendReviewOf(refusal(["not", "an", "object"]))).toBeNull();
    expect(sendReviewOf(refusal(null))).toBeNull();
    const oddActions = sendReviewOf(
      refusal({ review_id: "r-5", available_actions: "nope" }),
    );
    expect(oddActions).toEqual({ reviewId: "r-5", actions: [] });
    const oddEntries = sendReviewOf(
      refusal({ review_id: "r-6", available_actions: [1, null] }),
    );
    expect(oddEntries).toEqual({ reviewId: "r-6", actions: [] });
  });
});

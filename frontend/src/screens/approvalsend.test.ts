import { describe, expect, it } from "vitest";
import type { Approval } from "./approvals.queries";
import { stagedSendOf } from "./approvalsend";

// What the approval card asks the engine about, read off the proposal.
//
// Each staged send kind spells its payload its own way, and the card must ask
// the question the release will ask — same addressees, same anchor or records,
// same claim — or its answer is about a different message.

function approval(over: Partial<Approval>): Approval {
  return {
    id: "ap-1",
    kind: "send_email",
    status: "pending",
    proposed_by: "agent:runner",
    created_at: "2026-08-01T09:00:00Z",
    ...over,
  };
}

describe("the automation's held draft", () => {
  it("asks about its one addressee, against the thread it answers", () => {
    expect(
      stagedSendOf(
        approval({
          kind: "held_draft",
          proposed_change: {
            anchor_activity_id: "act-1",
            to: "anna@example.test",
            subject: "Re: the quote",
            body: "Attached.",
            communication_context: "reply_to_inbound",
          },
        }),
      ),
    ).toEqual({
      recipients: ["anna@example.test"],
      anchorActivityId: "act-1",
      context: "reply_to_inbound",
      marketingPurpose: undefined,
    });
  });

  // A draft with no thread is not a reply anybody can preview, and guessing an
  // anchor would ask about a conversation the message will not join.
  it("asks nothing when the draft names no thread", () => {
    expect(
      stagedSendOf(
        approval({
          kind: "held_draft",
          proposed_change: { to: "anna@example.test", body: "Hi" },
        }),
      ),
    ).toBeUndefined();
  });
});

describe("an agent's reply", () => {
  it("asks about every addressee, against the approval's own target", () => {
    expect(
      stagedSendOf(
        approval({
          kind: "send_email",
          target_entity_type: "activity",
          target_entity_id: "act-9",
          proposed_change: {
            to: ["anna@example.test"],
            cc: ["ops@example.test"],
            subject: "Follow-up",
            body: "Hi",
          },
        }),
      ),
    ).toEqual({
      recipients: ["anna@example.test", "ops@example.test"],
      anchorActivityId: "act-9",
      context: undefined,
      marketingPurpose: undefined,
    });
  });

  // The target is the anchor only when it IS an activity. A reply staged
  // against anything else is a payload this build does not understand.
  it("asks nothing when the target is not the thread", () => {
    expect(
      stagedSendOf(
        approval({
          kind: "send_email",
          target_entity_type: "deal",
          target_entity_id: "d-1",
          proposed_change: { to: ["anna@example.test"] },
        }),
      ),
    ).toBeUndefined();
  });
});

describe("an agent's account-started mail", () => {
  it("asks about the records it would be filed under, with its claim", () => {
    expect(
      stagedSendOf(
        approval({
          kind: "send_account_email",
          proposed_change: {
            to: ["buyer@example.test"],
            links: [
              { entity_type: "organization", entity_id: "org-1" },
              { entity_type: "deal", entity_id: "deal-1" },
            ],
            communication_context: "marketing",
            marketing_purpose: "newsletter",
          },
        }),
      ),
    ).toEqual({
      recipients: ["buyer@example.test"],
      links: [
        { entity_type: "organization", entity_id: "org-1" },
        { entity_type: "deal", entity_id: "deal-1" },
      ],
      context: "marketing",
      marketingPurpose: "newsletter",
    });
  });

  // One malformed link is not a shorter list. Asking about the records that
  // happened to parse would answer about a message filed differently from the
  // one that will go.
  it("asks nothing when a record link is not a link", () => {
    expect(
      stagedSendOf(
        approval({
          kind: "send_account_email",
          proposed_change: {
            to: ["buyer@example.test"],
            links: [
              { entity_type: "organization", entity_id: "org-1" },
              { entity_type: "spaceship", entity_id: "x" },
            ],
          },
        }),
      ),
    ).toBeUndefined();
  });

  // A claim the preview door would refuse is dropped rather than sent: refused,
  // the card would say "could not check" about a message that merely carried a
  // word this build does not know.
  it("drops a claim outside the contract's vocabulary", () => {
    expect(
      stagedSendOf(
        approval({
          kind: "send_account_email",
          proposed_change: {
            to: ["buyer@example.test"],
            links: [{ entity_type: "person", entity_id: "p-1" }],
            communication_context: "security_notice",
          },
        }),
      )?.context,
    ).toBeUndefined();
  });
});

describe("what is not a mail anybody can preview", () => {
  it("asks nothing about a channel reply, which names no addressee", () => {
    expect(
      stagedSendOf(
        approval({
          kind: "send_message",
          target_entity_type: "activity",
          target_entity_id: "act-1",
          proposed_change: { body: "Hi" },
        }),
      ),
    ).toBeUndefined();
  });

  it("asks nothing about a proposal with nobody to write to", () => {
    expect(
      stagedSendOf(
        approval({
          kind: "send_email",
          target_entity_type: "activity",
          target_entity_id: "act-1",
          proposed_change: { to: [], subject: "Follow-up" },
        }),
      ),
    ).toBeUndefined();
  });

  it("asks nothing about a kind that changes a record rather than sending", () => {
    expect(
      stagedSendOf(
        approval({
          kind: "update_record",
          proposed_change: { to: "somebody@example.test" },
        }),
      ),
    ).toBeUndefined();
  });
});

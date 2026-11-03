/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider, translate } from "../i18n";
import {
  decidingRecipient,
  SendMark,
  SendPermission,
  sendStatus,
} from "./sendpermission";

type Preview = components["schemas"]["SendAuthorizationPreview"];
type Recipient = components["schemas"]["SendAuthorizationPreviewRecipient"];

// A recipient answer in the shape the wire actually sends.
//
// decided_by is REQUIRED here even though the contract marks it optional,
// because a fixture that omits it would model a server that never says whose
// decision a refusal is — and every test written against that fixture would be
// describing a product we do not ship.
function answer(
  over: Partial<Recipient> & Pick<Recipient, "decided_by">,
): Recipient {
  return {
    address: "anna@example.test",
    verdict: "deny",
    reason_code: "no_compatible_evidence",
    ...over,
  };
}

function preview(...recipients: Recipient[]): Preview {
  return {
    allowed: recipients.every((r) => r.verdict === "allow"),
    recipients,
  };
}

afterEach(cleanup);

function draw(
  ui: Preview | undefined,
  onOverride?: () => void,
  unanswered = false,
  picksReason = false,
) {
  return render(
    <LocaleProvider initial="en">
      <SendPermission
        preview={ui}
        onOverride={onOverride}
        unanswered={unanswered}
        picksReason={picksReason}
      />
    </LocaleProvider>,
  );
}

describe("which state the engine's answer puts the composer in", () => {
  it("says nothing at all when nothing would refuse the message", () => {
    const { state } = decidingRecipient(
      preview(
        answer({
          verdict: "allow",
          reason_code: "allowed",
          would_refuse: false,
          can_be_overruled: false,
        }),
      ),
    );
    expect(state).toBe("allowed");
  });

  // The engine reading an incomplete record is the case the override exists for:
  // a rep who watched the customer ask them to send it knows something the CRM
  // does not.
  it("offers the override when the engine simply has no record", () => {
    const { state } = decidingRecipient(
      preview(answer({ would_refuse: true, can_be_overruled: true })),
    );
    expect(state).toBe("unproven");
  });

  it("offers nothing when the refusal is the subject's own act", () => {
    const { state } = decidingRecipient(
      preview(
        answer({
          reason_code: "marketing_objection",
          decided_by: "subject",
          would_refuse: true,
          can_be_overruled: false,
        }),
      ),
    );
    expect(state).toBe("refused");
  });

  // The two axes disagree, and this is the case that proves the surface reads
  // the right one. A hard bounce is the ENGINE's own reading — decided_by is
  // machine — and still absolute. Branching on decided_by would offer a button
  // that cannot do what it promises.
  it("offers no override on a machine reading that is nonetheless absolute", () => {
    const { state } = decidingRecipient(
      preview(
        answer({
          reason_code: "hard_bounce",
          decided_by: "machine",
          would_refuse: true,
          can_be_overruled: false,
        }),
      ),
    );
    expect(state).toBe("refused");
  });

  // Under observe a deny is recorded and the message still goes. Drawing the
  // verdict would talk a rep out of mail that was about to send.
  it("says nothing about a deny the rollout mode does not act on", () => {
    const { state } = decidingRecipient(
      preview(
        answer({
          verdict: "deny",
          mode: "observe",
          would_refuse: false,
          can_be_overruled: true,
        }),
      ),
    );
    expect(state).toBe("allowed");
  });

  // The whole message is refused for one refused recipient, so the composer has
  // to choose WHICH refusal to show. Showing the liftable one first would send a
  // rep to write a justification that cannot help.
  it("shows the refusal a rep cannot act on ahead of the one they can", () => {
    const { state, recipient } = decidingRecipient(
      preview(
        answer({
          address: "first@example.test",
          would_refuse: true,
          can_be_overruled: true,
        }),
        answer({
          address: "second@example.test",
          reason_code: "marketing_objection",
          decided_by: "subject",
          would_refuse: true,
          can_be_overruled: false,
        }),
      ),
    );
    expect(state).toBe("refused");
    expect(recipient?.address).toBe("second@example.test");
  });

  // A server that did not answer is not a server saying yes. Guessing
  // overrulable on an absent field is guessing that a rep may lift it.
  it("treats a missing answer as one nobody may lift", () => {
    const { state } = decidingRecipient(
      preview({
        address: "anna@example.test",
        verdict: "deny",
        reason_code: "no_compatible_evidence",
        would_refuse: true,
      }),
    );
    expect(state).toBe("refused");
  });

  it("says nothing before the preview has answered", () => {
    expect(decidingRecipient(undefined).state).toBe("allowed");
  });
});

describe("what a rep is shown", () => {
  it("renders nothing on an allowed send, so it costs no attention", () => {
    const { container } = draw(
      preview(
        answer({
          verdict: "allow",
          reason_code: "allowed",
          decided_by: "machine",
        }),
      ),
    );
    expect(container).toBeEmptyDOMElement();
  });

  it("names who decided, rather than only that it is not allowed", () => {
    draw(
      preview(
        answer({
          reason_code: "marketing_objection",
          decided_by: "subject",
          would_refuse: true,
          can_be_overruled: false,
        }),
      ),
    );
    expect(screen.getByText(/objected to marketing/i)).toBeInTheDocument();
    expect(
      screen.getByText(/only an administrator can still send past this/i),
    ).toBeInTheDocument();
  });

  // The rule that matters most: a control nobody may press teaches a rep the
  // product argues with them, and they raise a ticket instead of moving on.
  it("offers no control on a refusal nobody may lift", () => {
    draw(
      preview(
        answer({
          reason_code: "marketing_objection",
          decided_by: "subject",
          would_refuse: true,
          can_be_overruled: false,
        }),
      ),
      () => {},
    );
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("offers the override where the engine merely has no record", () => {
    draw(
      preview(answer({ would_refuse: true, can_be_overruled: true })),
      () => {},
    );
    expect(
      screen.getByRole("button", { name: /record reason for contact/i }),
    ).toBeInTheDocument();
  });

  // A read-only surface — an approval card somebody else's message lands on —
  // still explains itself, it just cannot take the answer.
  it("explains without a control when the surface cannot take an override", () => {
    draw(preview(answer({ would_refuse: true, can_be_overruled: true })));
    expect(
      screen.getByText(/no recorded reason to contact/i),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button")).toBeNull();
  });

  // An unrecognised reason must not reach a rep as an internal identifier.
  it("never shows the engine's own vocabulary to a rep", () => {
    draw(
      preview(
        answer({
          reason_code: "frequency_cap_reached",
          would_refuse: true,
          can_be_overruled: false,
        }),
      ),
    );
    expect(screen.queryByText(/frequency_cap_reached/)).toBeNull();
    expect(screen.getByText(/clears automatically/i)).toBeInTheDocument();
  });

  // The rep can fix this one in the same dialog, so the generic refusal would
  // send them looking for a cause that is not there.
  it("says the picked reason contradicts the thread, and how to fix it", () => {
    draw(
      preview(
        answer({
          reason_code: "claim_contradicts_resolution",
          decided_by: "machine",
          would_refuse: true,
          can_be_overruled: false,
        }),
      ),
      undefined,
      false,
      true,
    );
    expect(
      screen.getByText(
        translate("en", "sendPermission.reason.claimContradicts"),
      ),
    ).toBeInTheDocument();
    expect(screen.getByText(/does not match this thread/)).toBeVisible();
    expect(
      screen.queryByText(translate("en", "sendPermission.reason.other")),
    ).toBeNull();
  });
});

// Each code in the state the preview really sends it in: the three a recorded
// reason can lift arrive overrulable, the other two do not.
describe("every refusal says what is wrong and what to do", () => {
  it.each([
    ["no_compatible_evidence", true, "sendPermission.unprovenHint"],
    ["no_marketing_consent", true, "sendPermission.reason.noMarketingConsent"],
    [
      "legacy_transactional_unevidenced",
      true,
      "sendPermission.reason.legacyPurpose",
    ],
    ["unknown_purpose", false, "sendPermission.reason.unknownPurpose"],
    [
      "claim_contradicts_resolution",
      false,
      "sendPermission.reason.claimContradicts",
    ],
  ] as const)("%s", (code, overrulable, key) => {
    draw(
      preview(
        answer({
          reason_code: code,
          decided_by: "machine",
          would_refuse: true,
          can_be_overruled: overrulable,
        }),
      ),
      () => {},
      false,
      true,
    );
    expect(screen.getByText(translate("en", key))).toBeInTheDocument();
    expect(
      screen.queryByText(translate("en", "sendPermission.reason.other")),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: /record reason for contact/i }) !==
        null,
    ).toBe(overrulable);
  });
});

// A scheduled send or an approval row has no picker, so nothing asks for one.
describe("a read-only surface states the fact without the instruction", () => {
  it.each([
    [
      "no_marketing_consent",
      "sendPermission.reason.noMarketingConsentFact",
      "sendPermission.reason.noMarketingConsent",
    ],
    [
      "legacy_transactional_unevidenced",
      "sendPermission.reason.legacyPurposeFact",
      "sendPermission.reason.legacyPurpose",
    ],
    [
      "unknown_purpose",
      "sendPermission.reason.unknownPurposeFact",
      "sendPermission.reason.unknownPurpose",
    ],
    [
      "claim_contradicts_resolution",
      "sendPermission.reason.claimContradictsFact",
      "sendPermission.reason.claimContradicts",
    ],
  ] as const)("%s", (code, fact, instruction) => {
    draw(
      preview(
        answer({
          reason_code: code,
          decided_by: "machine",
          would_refuse: true,
          can_be_overruled:
            code !== "unknown_purpose" &&
            code !== "claim_contradicts_resolution",
        }),
      ),
    );
    expect(screen.getByText(translate("en", fact))).toBeInTheDocument();
    expect(screen.queryByText(translate("en", instruction))).toBeNull();
    expect(screen.queryByText(/pick/i)).toBeNull();
  });
});

// A composer whose address matches no contact draws no reason to record. Its own
// reason pick is still there, so the sentence still asks for it.
it("asks for the reason pick where there is no reason to record", () => {
  draw(
    preview(
      answer({
        reason_code: "no_marketing_consent",
        decided_by: "machine",
        would_refuse: true,
        can_be_overruled: true,
      }),
    ),
    undefined,
    false,
    true,
  );
  expect(
    screen.getByText(
      translate("en", "sendPermission.reason.noMarketingConsent"),
    ),
  ).toBeInTheDocument();
});

describe("what the hint tells a rep to do", () => {
  // With the control, the hint says how to answer. Without it, "say so" would
  // be the dead button in prose: an instruction the surface cannot take.
  it("tells a rep how to answer only where there is a way to", () => {
    draw(
      preview(answer({ would_refuse: true, can_be_overruled: true })),
      () => {},
    );
    expect(screen.getByText(/record it under your name/i)).toBeInTheDocument();
    expect(
      screen.queryByText(/refused until a reason is recorded/i),
    ).toBeNull();
  });

  it("says what happens to the message where nobody can answer here", () => {
    draw(preview(answer({ would_refuse: true, can_be_overruled: true })));
    expect(
      screen.getByText(/refused until a reason is recorded/i),
    ).toBeInTheDocument();
    expect(screen.queryByText(/record it under your name/i)).toBeNull();
  });
});

describe("when the question did not arrive", () => {
  // Silence is what "allowed" looks like. A failed check has to say it failed,
  // or the rep learns of the refusal from the Send button — the exact failure
  // the component exists to end.
  it("says the check did not happen rather than falling silent", () => {
    draw(undefined, undefined, true);
    expect(screen.getByRole("status")).toHaveTextContent(
      /send check did not complete/i,
    );
  });

  // A stale answer must not outrank the fact that the latest ask failed: the
  // preview on hand describes a message that may since have changed.
  it("outranks an earlier answer", () => {
    draw(
      preview(
        answer({
          reason_code: "marketing_objection",
          decided_by: "subject",
          would_refuse: true,
          can_be_overruled: false,
        }),
      ),
      undefined,
      true,
    );
    expect(
      screen.getByText(/send check did not complete/i),
    ).toBeInTheDocument();
    expect(screen.queryByText(/objected to marketing/i)).toBeNull();
  });
});

describe("an answer that has not arrived is not an answer", () => {
  // THE DEFECT THIS SLICE EXISTS FOR. `decidingRecipient(undefined)` returned
  // "allowed", so a composer waiting on the engine looked exactly like one that
  // had been told yes — and a rep who pressed Send in that window met the
  // refusal at the button.
  //
  // Three situations reached that one answer: nobody asked, the asking is in
  // flight, and the answer came back clean. Only the third is permission.
  it("draws checking while the question is in flight", () => {
    render(
      <LocaleProvider initial="en">
        <SendPermission preview={undefined} asking />
      </LocaleProvider>,
    );
    expect(screen.getByRole("status")).toHaveTextContent(/check/i);
  });

  it("does not claim the send is allowed while it is still asking", () => {
    const { state } = decidingRecipient(undefined, true);
    expect(state).toBe("checking");
  });

  // An unasked question is not a refusal either. A surface with no recipient
  // yet has nothing to ask about, and drawing a warning there would tell a rep
  // something is wrong with a message they have not written.
  it("stays quiet when there is nothing to ask about", () => {
    const { state } = decidingRecipient(undefined, false);
    expect(state).toBe("allowed");
  });

  // And a real allow still reads as one. Without this the fix above could be
  // "never say allowed", which would take the composer's quiet confirmation
  // away with it.
  it("still allows a message every recipient permits", () => {
    const { state } = decidingRecipient(
      preview(answer({ decided_by: "machine", verdict: "allow" })),
      false,
    );
    expect(state).toBe("allowed");
  });
});

describe("one status for the message", () => {
  const refusing = (over: Partial<Recipient>) =>
    preview(
      answer({
        decided_by: "machine",
        would_refuse: true,
        can_be_overruled: false,
        ...over,
      }),
    );

  it.each([
    [{ preview: undefined, asking: true }, "checking"],
    [{ preview: undefined }, "idle"],
    [{ preview: undefined, unanswered: true }, "unavailable"],
    [{ preview: preview() }, "unavailable"],
    [
      { preview: preview(answer({ decided_by: "machine", verdict: "allow" })) },
      "ready",
    ],
    [
      {
        preview: refusing({
          reason_code: "no_compatible_evidence",
          resolved_category: "active_deal_followup",
        }),
      },
      "missingContext",
    ],
    [
      { preview: refusing({ reason_code: "unknown_purpose" }) },
      "missingContext",
    ],
    [
      {
        preview: refusing({
          reason_code: "no_compatible_evidence",
          resolved_category: "requested_followup",
        }),
      },
      "missingEvidence",
    ],
    [
      { preview: refusing({ reason_code: "no_marketing_consent" }) },
      "missingPermission",
    ],
    [
      { preview: refusing({ reason_code: "marketing_objection" }) },
      "restricted",
    ],
    [{ preview: refusing({ reason_code: "a_code_from_later" }) }, "restricted"],
  ] as const)("%o is %s", (answered, kind) => {
    expect(sendStatus(answered).kind).toBe(kind);
  });

  // A refused Send outranks the preview it followed: the preview said ready,
  // the send said no, and the send is the newer answer.
  it("takes a refused send over an allowing preview", () => {
    const status = sendStatus({
      preview: preview(answer({ decided_by: "machine", verdict: "allow" })),
      refusal: {
        refusals: [
          {
            address: "a@x.test",
            reasonCode: "no_marketing_consent",
            canBeOverruled: true,
          },
          {
            address: "b@x.test",
            reasonCode: "consent_withdrawn",
            canBeOverruled: false,
          },
        ],
      },
    });
    expect(status).toMatchObject({
      kind: "restricted",
      deciding: { address: "b@x.test" },
      refusing: 2,
    });
  });

  it("reads a refused send with no readable reason as a stop, never as ready", () => {
    expect(
      sendStatus({ preview: undefined, refusal: { refusals: [] } }).kind,
    ).toBe("restricted");
  });

  it("shows no ready mark beside a refused send", () => {
    render(
      <LocaleProvider initial="en">
        <SendMark
          preview={preview(answer({ decided_by: "machine", verdict: "allow" }))}
          refusal={{ refusals: [] }}
        />
      </LocaleProvider>,
    );
    expect(
      screen.queryByRole("button", { name: /recipients checked/i }),
    ).toBeNull();
  });

  // Distinct conditions get distinct titles, so a missing deal never reads as
  // a consent problem and a missing consent never reads as an objection.
  it.each([
    [
      {
        reason_code: "no_compatible_evidence",
        resolved_category: "active_deal_followup",
      },
      "This message needs one more detail",
      "Choose the deal for this follow-up.",
    ],
    [
      { reason_code: "no_marketing_consent" },
      "Consent needed for this message",
      "Marketing needs the recipient’s consent, and none is on record.",
    ],
    [
      { reason_code: "marketing_objection" },
      "Message cannot be sent",
      "The recipient objected to marketing. Only an administrator can still send past this, one message at a time.",
    ],
  ] as const)("explains %o in its own words", (over, title, sentence) => {
    render(
      <LocaleProvider initial="en">
        <SendPermission preview={refusing(over)} />
      </LocaleProvider>,
    );
    expect(screen.getByText(title)).toBeInTheDocument();
    expect(screen.getByText(sentence)).toBeInTheDocument();
  });

  it("says how many of the recipients a refusal applies to where it may not name them", () => {
    render(
      <LocaleProvider initial="en">
        <SendPermission
          preview={preview(
            answer({
              decided_by: "subject",
              reason_code: "marketing_objection",
              would_refuse: true,
              can_be_overruled: false,
            }),
            answer({
              address: "ben@example.test",
              decided_by: "machine",
              verdict: "allow",
              reason_code: "allowed",
              would_refuse: false,
            }),
          )}
        />
      </LocaleProvider>,
    );
    expect(
      screen.getByText("This applies to 1 of 2 recipients."),
    ).toBeInTheDocument();
    expect(screen.queryByText(/anna@example\.test/)).toBeNull();
  });
});

describe("one rule picks the deciding refusal before and after Send", () => {
  // Mixed kinds and overrulability, in an order no single rule would pass by
  // accident: the preview and the refused Send must explain the same address.
  const wire = [
    {
      address: "e@x.test",
      reason_code: "no_compatible_evidence",
      can_be_overruled: true,
    },
    {
      address: "c@x.test",
      reason_code: "unknown_purpose",
      can_be_overruled: false,
    },
    {
      address: "p@x.test",
      reason_code: "no_marketing_consent",
      can_be_overruled: true,
    },
    {
      address: "r@x.test",
      reason_code: "frequency_cap_reached",
      can_be_overruled: false,
    },
  ] as const;

  it.each([
    [wire, "r@x.test"],
    [wire.filter((w) => w.address !== "r@x.test"), "c@x.test"],
    [wire.filter((w) => w.can_be_overruled), "p@x.test"],
  ])("picks the same recipient from %#", (rows, expected) => {
    const before = sendStatus({
      preview: preview(
        ...rows.map((row) =>
          answer({ ...row, decided_by: "machine", would_refuse: true }),
        ),
      ),
    });
    const after = sendStatus({
      preview: undefined,
      refusal: {
        refusals: rows.map((row) => ({
          address: row.address,
          reasonCode: row.reason_code,
          canBeOverruled: row.can_be_overruled,
        })),
      },
    });
    expect(before.deciding?.address).toBe(expected);
    expect(after.deciding?.address).toBe(expected);
    expect(after.kind).toBe(before.kind);
  });
});

describe("the mark makes no passed claim on a blocked form", () => {
  it("draws nothing for an allowed message the form will still stop", () => {
    render(
      <LocaleProvider initial="en">
        <SendMark
          preview={preview(
            answer({
              decided_by: "machine",
              verdict: "allow",
              would_refuse: false,
            }),
          )}
          blocked
        />
      </LocaleProvider>,
    );
    expect(
      screen.queryByRole("button", { name: /recipients checked/i }),
    ).toBeNull();
  });

  it("draws the passed check once nothing on the form is in the way", () => {
    render(
      <LocaleProvider initial="en">
        <SendMark
          preview={preview(
            answer({
              decided_by: "machine",
              verdict: "allow",
              would_refuse: false,
            }),
          )}
        />
      </LocaleProvider>,
    );
    expect(
      screen.getByRole("button", { name: /recipients checked/i }),
    ).toBeInTheDocument();
  });
});

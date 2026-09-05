/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { decidingRecipient, SendPermission } from "./sendpermission";

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
) {
  return render(
    <LocaleProvider initial="en">
      <SendPermission
        preview={ui}
        onOverride={onOverride}
        unanswered={unanswered}
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
    expect(
      screen.getByText(/asked not to receive marketing/i),
    ).toBeInTheDocument();
    expect(screen.getByText(/including an administrator/i)).toBeInTheDocument();
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
      screen.getByRole("button", { name: /say why/i }),
    ).toBeInTheDocument();
  });

  // A read-only surface — an approval card somebody else's message lands on —
  // still explains itself, it just cannot take the answer.
  it("explains without a control when the surface cannot take an override", () => {
    draw(preview(answer({ would_refuse: true, can_be_overruled: true })));
    expect(
      screen.getByText(/no record of why you may write/i),
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
    expect(screen.getByText(/clears on its own/i)).toBeInTheDocument();
  });
});

describe("what the hint tells a rep to do", () => {
  // With the control, the hint says how to answer. Without it, "say so" would
  // be the dead button in prose: an instruction the surface cannot take.
  it("tells a rep how to answer only where there is a way to", () => {
    draw(
      preview(answer({ would_refuse: true, can_be_overruled: true })),
      () => {},
    );
    expect(screen.getByText(/say so/i)).toBeInTheDocument();
    expect(screen.queryByText(/will be refused/i)).toBeNull();
  });

  it("says what happens to the message where nobody can answer here", () => {
    draw(preview(answer({ would_refuse: true, can_be_overruled: true })));
    expect(screen.getByText(/will be refused/i)).toBeInTheDocument();
    expect(screen.queryByText(/say so/i)).toBeNull();
  });
});

describe("when the question did not arrive", () => {
  // Silence is what "allowed" looks like. A failed check has to say it failed,
  // or the rep learns of the refusal from the Send button — the exact failure
  // the component exists to end.
  it("says the check did not happen rather than falling silent", () => {
    draw(undefined, undefined, true);
    expect(screen.getByRole("status")).toHaveTextContent(/could not check/i);
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
    expect(screen.getByText(/could not check/i)).toBeInTheDocument();
    expect(screen.queryByText(/asked not to receive marketing/i)).toBeNull();
  });
});

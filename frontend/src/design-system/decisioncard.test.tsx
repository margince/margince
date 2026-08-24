// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  type DecisionApproval,
  DecisionCard,
  type DecisionCardLabels,
  decisionUrgency,
} from "./decisioncard";

afterEach(cleanup);

// Every deadline in this file is an offset from ONE fixed instant, handed to the
// component as `now`. No test here reads the machine's clock: a card whose
// urgency band depends on what time the suite runs at is a card that changes its
// verdict overnight, which is the failure `make fe-clock-drift` exists to catch.
const NOW = Date.parse("2026-08-24T09:00:00.000Z");
const HOUR = 60 * 60 * 1000;

const LABELS: DecisionCardLabels = {
  accept: "Accept",
  edit: "Edit",
  reject: "Reject",
  skip: "Later",
  expired: "This ran out of time.",
  draftSubject: "Subject",
  draftBody: "Message",
  showMore: "Show the whole message",
  showLess: "Show less",
  noContent: "This proposal carries nothing to read.",
};

function approval(over: Partial<DecisionApproval> = {}): DecisionApproval {
  return {
    id: "0198c4f1-2b6a-7c3d-9e0f-11223344aabb",
    kind: "held_draft",
    status: "pending",
    proposed_by: "agent:mailroom",
    created_at: "2026-08-24T07:41:00.000Z",
    expires_at: new Date(NOW + 9 * HOUR).toISOString(),
    summary: "An automation drafted a reply to Marek.",
    proposed_change: {
      subject: "Re: kickoff",
      body: "Thanks for making the time yesterday.",
    },
    ...over,
  };
}

function card(over: Partial<Parameters<typeof DecisionCard>[0]> = {}) {
  return (
    <DecisionCard
      approval={approval()}
      now={NOW}
      labels={LABELS}
      onAccept={() => undefined}
      onEdit={() => undefined}
      onReject={() => undefined}
      {...over}
    />
  );
}

describe("DecisionCard — what the reader is being asked", () => {
  // The defect this pins is the one the whole primitive exists to fix: the row
  // it replaces named the addressee and stopped, so the one thing a person needs
  // in order to decide was the one thing not on screen.
  it("shows the drafted subject as the headline and the drafted body under it", () => {
    render(card());
    expect(screen.getByText("Re: kickoff")).toBeInTheDocument();
    expect(
      screen.getByText("Thanks for making the time yesterday."),
    ).toBeInTheDocument();
    // The summary is demoted, not dropped: it says WHY this is staged, which the
    // subject does not.
    expect(
      screen.getByText("An automation drafted a reply to Marek."),
    ).toBeInTheDocument();
  });

  // The subject is the headline, so a second "Subject: Re: kickoff" row would put
  // the same string on the card twice and leave the reader working out that the
  // two lines are one fact.
  it("prints the drafted subject exactly once", () => {
    render(card());
    expect(screen.getAllByText("Re: kickoff")).toHaveLength(1);
  });

  // `current_X` beside `proposed_X` is how this product's stagers spell a field
  // change (compose/signalproposals.go says so in as many words), and both sides
  // have to be on the card: "is this right?" is unanswerable from the new value
  // alone.
  it("draws a current/proposed pair as an old-to-new diff", () => {
    render(
      card({
        approval: approval({
          kind: "lifecycle_change",
          proposed_change: {
            current_lifecycle: "customer",
            proposed_lifecycle: "former_customer",
          },
        }),
      }),
    );
    expect(screen.getByText("customer")).toBeInTheDocument();
    expect(screen.getByText("former_customer")).toBeInTheDocument();
    expect(screen.getByText("lifecycle")).toBeInTheDocument();
  });

  // A `proposed_` key with no sibling is a value the proposal ADDS. Drawing it
  // against a struck-through blank would claim we know the old one was empty.
  it("does not invent an old side for a value the proposal only adds", () => {
    const { container } = render(
      card({
        approval: approval({
          proposed_change: { proposed_lifecycle: "customer" },
        }),
      }),
    );
    // No diff row at all: the old-to-new pair is the markup that would claim we
    // know what the value used to be.
    expect(container.querySelector(".dcard-diff")).toBeNull();
    // It is still shown — as a payload field, in the deck's full reading.
    expect(screen.getByText("proposed_lifecycle")).toBeInTheDocument();
  });

  // `empty` is the only state allowed to say "there is none", and what there is
  // none OF is the caller's word.
  it("says so in the caller's words when the payload carries nothing to read", () => {
    render(card({ approval: approval({ proposed_change: {} }) }));
    expect(
      screen.getByText("This proposal carries nothing to read."),
    ).toBeInTheDocument();
  });

  // The deck reads the whole payload; a row does not, because a queue of rows
  // each unrolling nine wire keys is not a queue anybody can work.
  it("keeps the unrecognised payload fields off the row layout", () => {
    const extra = approval({
      proposed_change: { subject: "Re: kickoff", consent_purpose: "contract" },
    });
    render(card({ approval: extra, layout: "row" }));
    expect(screen.queryByText("consent_purpose")).not.toBeInTheDocument();
    cleanup();
    render(card({ approval: extra, layout: "deck" }));
    expect(screen.getByText("consent_purpose")).toBeInTheDocument();
  });
});

describe("DecisionCard — the deadline", () => {
  // The thresholds have ONE home. The badge the caller draws beside the card
  // reads the same function, which is what keeps a deadline from looking urgent
  // on the card and calm in the chip.
  it("bands the time left at one hour and at six", () => {
    expect(decisionUrgency(9 * HOUR)).toBe("calm");
    expect(decisionUrgency(3 * HOUR)).toBe("soon");
    expect(decisionUrgency(20 * 60 * 1000)).toBe("urgent");
    expect(decisionUrgency(0)).toBe("lapsed");
    expect(decisionUrgency(-HOUR)).toBe("lapsed");
  });

  it("carries the band the time left earns", () => {
    const { container } = render(
      card({
        approval: approval({
          expires_at: new Date(NOW + 3 * HOUR).toISOString(),
        }),
      }),
    );
    expect(container.querySelector(".dcard")).toHaveAttribute(
      "data-urgency",
      "soon",
    );
  });

  // Absence is not a state. A proposal that never lapses must not read as calm,
  // because "calm" is a claim about a deadline it does not have.
  it("claims no band at all for a proposal with no deadline", () => {
    const { container } = render(
      card({ approval: approval({ expires_at: null }) }),
    );
    expect(container.querySelector(".dcard")).not.toHaveAttribute(
      "data-urgency",
    );
  });

  // The whole point of the lapse: Accept is not drawn. A control whose only
  // possible answer is a refusal is worse than no control.
  it("offers no Accept once the deadline has passed, and says why", () => {
    render(
      card({
        approval: approval({ expires_at: new Date(NOW - HOUR).toISOString() }),
      }),
    );
    expect(
      screen.queryByRole("button", { name: "Accept" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Edit" }),
    ).not.toBeInTheDocument();
    expect(screen.getByText("This ran out of time.")).toBeInTheDocument();
  });

  // The server expires lazily at read time, so a row can arrive already stamped
  // — and it means the same thing to a reader as one the live clock overtook.
  it("reads a wire-stamped expiry as lapsed even with time left on the clock", () => {
    render(
      card({
        approval: approval({
          status: "expired",
          expires_at: new Date(NOW + 9 * HOUR).toISOString(),
        }),
      }),
    );
    expect(
      screen.queryByRole("button", { name: "Accept" }),
    ).not.toBeInTheDocument();
  });
});

describe("DecisionCard — the verbs", () => {
  it("sends each verdict to its own callback", async () => {
    const user = userEvent.setup();
    const onAccept = vi.fn();
    const onReject = vi.fn();
    const onSkip = vi.fn();
    render(card({ onAccept, onReject, onSkip, onEdit: undefined }));
    await user.click(screen.getByRole("button", { name: "Accept" }));
    await user.click(screen.getByRole("button", { name: "Reject" }));
    await user.click(screen.getByRole("button", { name: "Later" }));
    expect(onAccept).toHaveBeenCalledTimes(1);
    expect(onReject).toHaveBeenCalledTimes(1);
    expect(onSkip).toHaveBeenCalledTimes(1);
    // Not offered, so not drawn: the inbox has no editable field on every kind.
    expect(
      screen.queryByRole("button", { name: "Edit" }),
    ).not.toBeInTheDocument();
  });

  // A surface with no word for "later" must not get an unnamed button for it.
  it("withholds Later where the surface has no word for it", () => {
    const { skip, ...unnamed } = LABELS;
    expect(skip).toBe("Later");
    render(card({ labels: unnamed, onSkip: () => undefined }));
    expect(
      screen.queryByRole("button", { name: "Later" }),
    ).not.toBeInTheDocument();
  });

  // History offers nothing to press. The verdict badge is the caller's.
  it("draws no verbs on a decided card", () => {
    render(card({ decided: true, approval: approval({ status: "approved" }) }));
    expect(
      screen.queryByRole("button", { name: "Accept" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Reject" }),
    ).not.toBeInTheDocument();
  });

  // The editor REPLACES the verbs rather than sitting beside them: a card
  // offering both a bare Accept and an "approve edited" sends two different
  // writes from one screen.
  it("hands the verbs over to the editor while one is open", () => {
    render(card({ editor: <p>the editor</p> }));
    expect(screen.getByText("the editor")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Accept" }),
    ).not.toBeInTheDocument();
  });
});

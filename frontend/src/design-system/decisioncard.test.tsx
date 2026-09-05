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
  type DecisionDeadline,
  DecisionStatusChip,
  type DecisionStatusLabels,
  DecisionToolChip,
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
  loading: "Reading the proposal",
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

// What a kind that has DECLARED what it shows puts on the card, against the
// generic reading that prints the payload's own keys.
//
// The declarations live on the screen side (screens/approvalkind.ts) and arrive
// here already resolved, so these tests hand the primitive what that resolution
// produces. They are about the drawing, not the vocabulary.
describe("DecisionCard — a kind that says what it shows", () => {
  const CLOSE_DATE = approval({
    kind: "close_date_correction",
    summary: 'Confirm the real close date for "Riverty" (proposed 2026-10-01)',
    proposed_change: {
      deal_id: "01a03781-9083-7565-8d65-5939ec0f3e70",
      basis: "deal has gone quiet; confirm it is still alive",
      expected_close_date: "2026-10-01",
      flags: ["unrealistic_stale"],
    },
  });

  const CLOSE_DATE_DISPLAY = [
    {
      field: "basis",
      label: "Why",
      value: "deal has gone quiet; confirm it is still alive",
      lead: true as const,
    },
    {
      field: "expected_close_date",
      label: "Proposed date",
      value: "01.10.2026",
    },
    {
      field: "flags",
      label: "What is wrong with it",
      value: "nothing has moved on it",
    },
  ];

  // The reason is the first thing in the body and carries no caption. It is a
  // sentence the server wrote for a person, and labelling it would frame an
  // explanation as one more data point.
  it("leads the body with the reason, unlabelled", () => {
    const { container } = render(
      card({ approval: CLOSE_DATE, display: CLOSE_DATE_DISPLAY }),
    );
    const lead = container.querySelector(".dcard-lead");
    expect(lead).toHaveTextContent(
      "deal has gone quiet; confirm it is still alive",
    );
    // Not repeated as a labelled fact underneath.
    expect(screen.queryByText("Why")).not.toBeInTheDocument();
  });

  // The identifiers are the point of the whole exercise: a person asked to
  // decide something must not be shown the row it is stored in.
  it("shows the declared fields under their names and drops the rest", () => {
    render(card({ approval: CLOSE_DATE, display: CLOSE_DATE_DISPLAY }));
    expect(screen.getByText("Proposed date")).toBeInTheDocument();
    expect(screen.getByText("01.10.2026")).toBeInTheDocument();
    expect(screen.getByText("What is wrong with it")).toBeInTheDocument();
    expect(screen.getByText("nothing has moved on it")).toBeInTheDocument();
    // Never the wire.
    expect(screen.queryByText("deal_id")).not.toBeInTheDocument();
    expect(
      screen.queryByText("01a03781-9083-7565-8d65-5939ec0f3e70"),
    ).not.toBeInTheDocument();
    expect(screen.queryByText("unrealistic_stale")).not.toBeInTheDocument();
  });

  // A declared field the payload does not carry is absent rather than blank:
  // `previous_close_date` is genuinely missing on a deal that never had one,
  // and an empty row under a caption reads as a fact nobody wrote.
  it("omits a declared field the payload does not carry", () => {
    render(
      card({
        approval: CLOSE_DATE,
        display: [
          ...CLOSE_DATE_DISPLAY,
          {
            field: "previous_close_date",
            label: "Date on it now",
            value: null,
          },
        ],
      }),
    );
    expect(screen.queryByText("Date on it now")).not.toBeInTheDocument();
  });

  // A declared kind keeps its reason AND its fields on a row. The deck-only
  // suppression exists for the raw reading — a queue of rows each unrolling
  // nine wire keys — and a declared list is at most four short labelled rows.
  // Suppressing them cost the reader what they were being asked about: the
  // Worklist shows one decision at a time, and its close-date card rendered
  // the reason with no proposed date beneath it.
  it("keeps the reason and the declared fields on a row", () => {
    const { container } = render(
      card({
        approval: CLOSE_DATE,
        display: CLOSE_DATE_DISPLAY,
        layout: "row",
      }),
    );
    expect(container.querySelector(".dcard-lead")).toHaveTextContent(
      "deal has gone quiet",
    );
    expect(screen.getByText("Proposed date")).toBeInTheDocument();
    expect(screen.getByText("01.10.2026")).toBeInTheDocument();
  });

  // The generic reading is the honest fallback, not a worse one: a kind
  // carrying an agent's tool arguments has no typed payload to describe, and
  // captions invented for unknown keys would be guesses.
  it("still prints wire keys for a kind that declared nothing", () => {
    render(
      card({
        approval: approval({
          kind: "update_record",
          proposed_change: { stage_id: "01a03781-9083-7565-8d65-5939ec0f3e70" },
        }),
      }),
    );
    expect(screen.getByText("stage_id")).toBeInTheDocument();
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

// The chip's own words, in the same shape a screen builds them: a countdown
// sentence over the span, and one verdict word per decided state.
const STATUS_LABELS: DecisionStatusLabels = {
  expiresIn: (msRemaining) => `expires in ${Math.round(msRemaining / HOUR)}h`,
  approved: "Approved",
  rejected: "Rejected",
  expired: "Expired",
};

// The chip reads a deadline, not a whole approval, so its fixture is one — and
// that is what lets the "status this build has never heard of" case below be
// written at all.
function deadline(over: Partial<DecisionDeadline> = {}): DecisionDeadline {
  return {
    status: "pending",
    expires_at: new Date(NOW + 9 * HOUR).toISOString(),
    ...over,
  };
}

function statusChip(
  over: Partial<Parameters<typeof DecisionStatusChip>[0]> = {},
) {
  return (
    <DecisionStatusChip
      approval={deadline()}
      decided={false}
      now={NOW}
      labels={STATUS_LABELS}
      {...over}
    />
  );
}

describe("DecisionStatusChip", () => {
  it("counts down in the caller's words while a proposal is still live", () => {
    render(statusChip());
    expect(screen.getByText("expires in 9h")).toBeInTheDocument();
  });

  // The tone comes off `decisionUrgency`, which is also what tints the card's
  // edge. These two frames are the bands either side of the six-hour line, so a
  // chip that grew its own thresholds fails here rather than on a screen.
  it("escalates its tone through the same bands the card's edge reads", () => {
    const { container } = render(
      statusChip({
        approval: deadline({
          expires_at: new Date(NOW + 3 * HOUR).toISOString(),
        }),
      }),
    );
    expect(container.querySelector(".badge-warn")).toBeInTheDocument();
    cleanup();

    const urgent = render(
      statusChip({
        approval: deadline({
          expires_at: new Date(NOW + 20 * 60 * 1000).toISOString(),
        }),
      }),
    ).container;
    expect(urgent.querySelector(".badge-danger")).toBeInTheDocument();
  });

  // Silence, not a second notice: the card this sits on already says it ran out
  // of time where its verbs would have been.
  it("draws nothing on a lapsed proposal that has not been answered", () => {
    const { container } = render(
      statusChip({
        approval: deadline({ expires_at: new Date(NOW - HOUR).toISOString() }),
      }),
    );
    expect(container).toBeEmptyDOMElement();
  });

  it("draws nothing for a proposal that has no deadline at all", () => {
    const { container } = render(
      statusChip({ approval: deadline({ expires_at: null }) }),
    );
    expect(container).toBeEmptyDOMElement();
  });

  it("shows the verdict rather than a countdown once it has been decided", () => {
    render(
      statusChip({ decided: true, approval: deadline({ status: "approved" }) }),
    );
    expect(screen.getByText("Approved")).toBeInTheDocument();
    cleanup();

    render(
      statusChip({ decided: true, approval: deadline({ status: "rejected" }) }),
    );
    expect(screen.getByText("Rejected")).toBeInTheDocument();
  });

  // The status vocabulary grows on the server, so this build is always one
  // deploy from a status it has no word for — the case `inbox.kinds.test.tsx`
  // already settled for `kind`. A decided card with no badge says less about
  // itself than a slightly imprecise one, so it falls back rather than vanishes.
  it("falls back to the lapsed word for a status it has not learned", () => {
    render(
      statusChip({
        decided: true,
        approval: deadline({ status: "superseded" }),
      }),
    );
    expect(screen.getByText("Expired")).toBeInTheDocument();
  });
});

describe("DecisionToolChip", () => {
  it("names the tool in the caller's words", () => {
    render(<DecisionToolChip verb="send_email" label={(v) => `via ${v}`} />);
    expect(screen.getByText("via send_email")).toBeInTheDocument();
  });

  // The kind→verb catalogue is the screen's; a kind it cannot map yields no
  // verb, and naming a tool nobody can check would be worse than saying nothing.
  it("stays silent when the caller could not map the kind to a verb", () => {
    const { container } = render(
      <DecisionToolChip verb={undefined} label={(v) => `via ${v}`} />,
    );
    expect(container).toBeEmptyDOMElement();
  });
});

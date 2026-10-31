// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { act, cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
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
  showMore: "Show full message",
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
  // it replaces named the addressee and stopped, so the one thing a contact needs
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

  // A preview that needs wider sight arrives with no staged call and no
  // summary: the server keeps both back. The card still renders and says so.
  it("renders an approval whose plan the server held back", () => {
    const withheld = approval({
      preview: { version: 2, state: "needs_wider_sight", changes: [] },
    });
    delete withheld.summary;
    delete withheld.proposed_change;
    render(card({ approval: withheld }));
    expect(screen.queryByText("Re: kickoff")).not.toBeInTheDocument();
    expect(
      screen.getByText("This proposal carries nothing to read."),
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
// The declarations live on the screen side (features/automations/approvalkind.ts) and arrive
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
      label: "Issues",
      value: "no recent progress",
    },
  ];

  // The reason is the first thing in the body and carries no caption. It is a
  // sentence the server wrote for a contact, and labelling it would frame an
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

  // The identifiers are the point of the whole exercise: a contact asked to
  // decide something must not be shown the row it is stored in.
  it("shows the declared fields under their names and drops the rest", () => {
    render(card({ approval: CLOSE_DATE, display: CLOSE_DATE_DISPLAY }));
    expect(screen.getByText("Proposed date")).toBeInTheDocument();
    expect(screen.getByText("01.10.2026")).toBeInTheDocument();
    expect(screen.getByText("Issues")).toBeInTheDocument();
    expect(screen.getByText("no recent progress")).toBeInTheDocument();
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
            label: "Current close date",
            value: null,
          },
        ],
      }),
    );
    expect(screen.queryByText("Current close date")).not.toBeInTheDocument();
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

  it("names Reject in words, as the danger answer beside Accept", () => {
    render(card());
    const reject = screen.getByRole("button", { name: "Reject" });
    expect(reject).toHaveTextContent("Reject");
    expect(reject).not.toHaveAttribute("aria-label");
    expect(reject).toHaveClass("btn-danger");
    expect(screen.getByRole("button", { name: "Accept" })).toHaveClass(
      "btn-primary",
    );
    expect(reject.nextElementSibling).toBe(
      screen.getByRole("button", { name: "Accept" }),
    );
  });

  it("keeps Later and Edit as glyphs on the full card", () => {
    render(card({ onSkip: () => undefined }));
    expect(screen.getByRole("button", { name: "Later" })).toHaveAttribute(
      "aria-label",
      "Later",
    );
    expect(screen.getByRole("button", { name: "Edit" })).toHaveAttribute(
      "aria-label",
      "Edit",
    );
  });

  it("draws Accept busy and every other verb disabled while a verdict is out", () => {
    render(card({ pending: true, onSkip: () => undefined }));
    const accept = screen.getByRole("button", { name: "Accept" });
    expect(accept).toHaveAttribute("aria-busy", "true");
    expect(accept).not.toBeDisabled();
    for (const name of ["Reject", "Later", "Edit"]) {
      expect(screen.getByRole("button", { name })).toBeDisabled();
    }
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

function HoldingCard({
  compact,
  approval: asked = approval(),
  onPause,
}: Readonly<{
  compact?: boolean;
  approval?: DecisionApproval;
  onPause?: (paused: boolean) => void;
}>) {
  const [held, setHeld] = useState(false);
  return card({
    approval: asked,
    layout: compact ? "row" : "deck",
    compact: compact ? { detail: "Proposal", more: "More options" } : undefined,
    onReject: () => setHeld(true),
    heldRejection: held
      ? {
          label: "Rejected",
          undo: "Undo",
          onUndo: () => setHeld(false),
          onPause,
        }
      : undefined,
  });
}

describe("DecisionCard — a rejection held for its undo", () => {
  it.each([false, true])(
    "swaps the verbs for the bar, focuses its words, and hands focus back to Reject (compact: %s)",
    async (compact) => {
      const user = userEvent.setup();
      render(<HoldingCard compact={compact} />);

      await user.click(screen.getByRole("button", { name: "Reject" }));

      expect(screen.getByText("Rejected")).toHaveFocus();
      const undo = screen.getByRole("button", { name: "Undo" });
      expect(undo).not.toHaveAttribute("aria-describedby");
      for (const name of ["Reject", "Accept", "Edit"]) {
        expect(screen.queryByRole("button", { name })).not.toBeInTheDocument();
      }

      await user.click(undo);

      expect(screen.queryByText("Rejected")).not.toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Reject" })).toHaveFocus();
    },
  );

  it("tells the caller while a pointer or focus rests on Undo", async () => {
    const user = userEvent.setup();
    const onPause = vi.fn();
    render(<HoldingCard onPause={onPause} />);
    await user.click(screen.getByRole("button", { name: "Reject" }));
    const undo = screen.getByRole("button", { name: "Undo" });

    await user.hover(undo);
    expect(onPause).toHaveBeenLastCalledWith(true);
    await user.unhover(undo);
    expect(onPause).toHaveBeenLastCalledWith(false);
    await user.tab();
    expect(undo).toHaveFocus();
    expect(onPause).toHaveBeenLastCalledWith(true);
    await user.tab();
    expect(onPause).toHaveBeenLastCalledWith(false);
  });

  it("draws the words without Undo once the rejection is on its way", () => {
    render(card({ heldRejection: { label: "Rejected", undo: "Undo" } }));
    expect(screen.getByText("Rejected")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Undo" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Reject" }),
    ).not.toBeInTheDocument();
  });

  it("hands focus to the lapse notice when an undone card has run out", async () => {
    const user = userEvent.setup();
    const { rerender } = render(<HoldingCard />);
    await user.click(screen.getByRole("button", { name: "Reject" }));
    rerender(
      <HoldingCard
        approval={approval({ expires_at: new Date(NOW - HOUR).toISOString() })}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Undo" }));

    expect(screen.getByText(LABELS.expired)).toHaveFocus();
  });

  it("hands focus from a sent bar to the card once it is decided", () => {
    const sent = { label: "Rejected", undo: "Undo" };
    const { rerender } = render(card({ heldRejection: sent }));
    act(() => screen.getByText("Rejected").focus());

    rerender(card({ heldRejection: sent, decided: true }));

    const article = screen.getByRole("article");
    expect(article).toHaveFocus();
    expect(article).toHaveAttribute("tabindex", "-1");
  });

  it("draws neither the bar nor the verbs once the decision is history", () => {
    render(
      card({
        decided: true,
        heldRejection: { label: "Rejected", undo: "Undo", onUndo: vi.fn() },
      }),
    );
    expect(
      screen.queryByRole("button", { name: "Undo" }),
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
    expect(container.querySelector(".badge-warning")).toBeInTheDocument();
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

// The dense form of the row: one line, the proposal behind a control, and the
// two heavier verdicts behind a menu. What is being held here is what the line
// may NOT carry — a queue of these is scanned, and anything on the line that a
// reader has to weigh is a line they have to stop at.
describe("DecisionCard at list density", () => {
  const COMPACT = { detail: "Proposal", more: "More options" };

  it("keeps the proposal off the line and behind a control", async () => {
    const user = userEvent.setup();
    const { container } = render(
      card({ layout: "row", compact: COMPACT, onSkip: () => undefined }),
    );

    const line = container.querySelector(".dcard-line");
    expect(line).toBeTruthy();
    expect(line?.textContent).not.toContain("Message");
    expect(screen.queryByText("Subject")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: COMPACT.detail }));

    expect(await screen.findByText("Message")).toBeInTheDocument();
  });

  // Escape closes it and the trigger takes its focus back. A panel a keyboard
  // can open and not leave is a trap.
  it("closes the proposal on Escape and hands the focus back", async () => {
    const user = userEvent.setup();
    render(card({ layout: "row", compact: COMPACT }));

    const trigger = screen.getByRole("button", { name: COMPACT.detail });
    await user.click(trigger);
    expect(await screen.findByText("Message")).toBeInTheDocument();

    await user.keyboard("{Escape}");

    expect(screen.queryByText("Message")).not.toBeInTheDocument();
    expect(trigger).toHaveFocus();
  });

  it("spells Reject and Accept and folds Later and Edit into the menu", async () => {
    const user = userEvent.setup();
    render(card({ layout: "row", compact: COMPACT, onSkip: () => undefined }));

    expect(screen.getByRole("button", { name: "Accept" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Reject" })).toHaveTextContent(
      "Reject",
    );
    expect(
      screen.queryByRole("button", { name: "Later" }),
    ).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: COMPACT.more }));

    expect(
      await screen.findByRole("button", { name: "Later" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Edit" })).toBeInTheDocument();
  });

  // A menu over no verbs is a control that opens nothing.
  it("draws no menu when the caller offered neither later nor edit", () => {
    render(
      card({
        layout: "row",
        compact: COMPACT,
        onEdit: undefined,
        onSkip: undefined,
      }),
    );

    expect(
      screen.queryByRole("button", { name: COMPACT.more }),
    ).not.toBeInTheDocument();
  });

  // The DECK form is the one place the whole payload belongs, so the density is
  // the row's alone. A tall card that hid its proposal would be the one thing a
  // whole plate exists not to do.
  it("ignores the density in the deck layout", () => {
    render(card({ layout: "deck", compact: COMPACT }));

    expect(screen.getByText("Message")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: COMPACT.detail }),
    ).not.toBeInTheDocument();
  });

  // Past the deadline the verbs are gone, and the row says so where they were
  // rather than in a fold nobody opened.
  it("says a lapsed row ran out of time on the line itself", () => {
    const { container } = render(
      card({
        layout: "row",
        compact: COMPACT,
        approval: approval({
          expires_at: new Date(NOW - HOUR).toISOString(),
        }),
      }),
    );

    expect(container.querySelector(".dcard-line")?.textContent).toContain(
      LABELS.expired,
    );
    expect(
      screen.queryByRole("button", { name: "Accept" }),
    ).not.toBeInTheDocument();
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

it.each(["row", "deck"] as const)(
  "does not repeat the proposal summary in the %s body",
  (layout) => {
    render(
      card({
        layout,
        approval: approval({
          kind: "commitment_task",
          summary: "Send revised pricing",
          proposed_change: { summary: "Send revised pricing" },
        }),
        display: [
          {
            field: "summary",
            label: "Task",
            value: "Send revised pricing",
            lead: true,
          },
        ],
      }),
    );
    expect(screen.getAllByText("Send revised pricing")).toHaveLength(1);
  },
);

it("keeps a distinct explanation below the proposal summary", () => {
  render(
    card({
      approval: approval({
        kind: "commitment_task",
        summary: "Send revised pricing",
        proposed_change: { summary: "Send revised pricing" },
      }),
      display: [
        {
          field: "reason",
          label: "Reason",
          value: "The customer requested updated terms.",
          lead: true,
        },
      ],
    }),
  );
  expect(screen.getByText("Send revised pricing")).toBeInTheDocument();
  expect(
    screen.getByText("The customer requested updated terms."),
  ).toBeInTheDocument();
});

it.each(["row", "deck"] as const)(
  "uses the authorized comparison in the %s layout",
  (layout) => {
    render(
      card({
        layout,
        approval: approval({
          kind: "lifecycle_change",
          proposed_change: {
            current_lifecycle: "raw_before",
            proposed_lifecycle: "raw_after",
          },
        }),
        display: [
          {
            field: "lifecycle",
            label: "Account stage",
            value: "Customer to Former customer",
            comparison: { from: "Customer", to: "Former customer" },
          },
        ],
      }),
    );
    expect(screen.getByText("Account stage")).toBeInTheDocument();
    expect(screen.getByText("Customer")).toBeInTheDocument();
    expect(screen.getByText("Former customer")).toBeInTheDocument();
    expect(screen.queryByText("raw_before")).not.toBeInTheDocument();
    expect(screen.queryByText("raw_after")).not.toBeInTheDocument();
  },
);

it.each(["row", "deck"] as const)(
  "does not reconstruct a refused comparison in the %s layout",
  (layout) => {
    const { container } = render(
      card({
        layout,
        approval: approval({
          kind: "lifecycle_change",
          proposed_change: {
            current_lifecycle: "raw_before",
            proposed_lifecycle: "raw_after",
          },
        }),
        display: [
          {
            field: "preview_status",
            label: "Preview",
            value: "The record changed. Review a new proposal.",
            comparison: null,
            lead: true,
          },
        ],
      }),
    );
    expect(
      screen.getByText("The record changed. Review a new proposal."),
    ).toBeInTheDocument();
    expect(container.querySelector(".dcard-diff")).toBeNull();
    expect(screen.queryByText("raw_before")).not.toBeInTheDocument();
    expect(screen.queryByText("raw_after")).not.toBeInTheDocument();
  },
);

it("replaces historical evidence with the current authorized evidence", () => {
  const { rerender } = render(
    card({
      layout: "deck",
      approval: approval({
        evidence: [{ evidence_snippet: "Historical private excerpt" }],
        preview: {
          version: 1,
          state: "unavailable",
          changes: [],
          reviewer_reason: "record_authority",
          evidence: [],
        },
      }),
    }),
  );
  expect(
    screen.queryByText(/Historical private excerpt/),
  ).not.toBeInTheDocument();
  rerender(
    card({
      layout: "deck",
      approval: approval({
        preview: {
          version: 1,
          state: "ready",
          changes: [],
          reviewer_reason: "record_authority",
          evidence: [
            {
              evidence_snippet: "Current authorized excerpt",
              source_type: "signal",
            },
          ],
        },
      }),
    }),
  );
  expect(screen.getByText(/Current authorized excerpt/)).toBeInTheDocument();
});

it("shows authorized preview quotes before a row decision without expansion", () => {
  render(
    card({
      layout: "row",
      approval: approval({
        evidence: [{ evidence_snippet: "Historical private excerpt" }],
        preview: {
          version: 1,
          state: "stale",
          changes: [],
          reviewer_reason: "record_authority",
          evidence: [
            {
              evidence_snippet: "Current authorized source quotation",
              source_type: "activity",
              source_lines: [2, 3],
            },
          ],
        },
      }),
    }),
  );
  expect(screen.getByText(/Current authorized source quotation/)).toBeVisible();
  expect(
    screen.queryByText(/Historical private excerpt/),
  ).not.toBeInTheDocument();
});

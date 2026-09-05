// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { DealIdentityLine } from "../deals";
import { DealPulse } from "./dealpulse";
import { DealSeats } from "./dealseats";
import { DealStrip } from "./dealstrip";

// What the deal page owes a reader before they read anything.
//
// The four readings and the sentence above them exist for the scanning read —
// somebody working thirty deals before a forecast call, who needs to find the
// one that needs them without reading thirty briefings. So these tests are
// about what a scanner can SEE: whose move it is, and which figure is bad news.
//
// They also pin the two clauses that are deliberately absent. The design asked
// for "awaiting reply N days" and "they replied twice, we replied once", and
// neither is a fact this product holds — there is no send timestamp on an offer
// and no deal-scoped direction count anywhere. A card that reads well and
// cannot be checked is what this page exists to stop.

afterEach(cleanup);

type Deal = components["schemas"]["Deal"];
type DealStatusCard = components["schemas"]["DealStatusCard"];

const DEAL_ID = "01a03000-0000-7000-8000-000000000001";
const MAIL_ID = "01a03000-0000-7000-8000-0000000000aa";

function deal(over: Partial<Deal> = {}): Deal {
  return {
    id: DEAL_ID,
    name: "Fleet telematics rollout",
    amount_minor: 4_500_000,
    currency: "EUR",
    status: "open",
    stalled: false,
    source: "ui",
    version: 1,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...over,
  } as Deal;
}

// One harness, with the query client: the seat cells resolve a person through
// `EntityRef` now, so every card on this record needs one. There were two
// helpers before — this and `showFacts` below — differing only in whether they
// supplied it.
function show(node: React.ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{node}</LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("the sentence says whose move it is", () => {
  const card = (over: Partial<DealStatusCard> = {}): DealStatusCard =>
    ({
      deal_id: DEAL_ID,
      story: { sentences: [] },
      generated_at: "2026-08-24T00:00:00Z",
      generated_by: "model",
      ...over,
    }) as DealStatusCard;

  it("names the day they wrote, from the move's own evidence", () => {
    show(
      <DealPulse
        card={card({
          reply_to: MAIL_ID,
          next: {
            action: "draft_email",
            reason: "Answer them.",
            evidence: [
              {
                text: "Unanswered: Slots for the pilot review",
                activity_id: MAIL_ID,
                occurred_at: "2026-05-20T09:00:00Z",
              },
            ],
          },
        })}
        timeline={[]}
      />,
    );
    expect(screen.getByText(/It's your move/)).toBeInTheDocument();
    expect(screen.getByText(/20 May/)).toBeInTheDocument();
  });

  it("falls back to the timeline when the move is about something else", () => {
    // reply_to is deliberately independent of `next`: a booked meeting outranks
    // an unanswered mail as the move, but somebody is still waiting.
    show(
      <DealPulse
        card={card({
          reply_to: MAIL_ID,
          next: {
            action: "open_meeting_brief",
            reason: "Read it.",
            evidence: [],
          },
        })}
        timeline={[
          {
            id: MAIL_ID,
            kind: "email",
            occurred_at: "2026-05-20T09:00:00Z",
          } as components["schemas"]["Activity"],
        ]}
      />,
    );
    expect(screen.getByText(/20 May/)).toBeInTheDocument();
  });

  it("says it is our move without a date when the row is off-page", () => {
    // The timeline holds one page and the reader may have filtered it, so the
    // row behind reply_to can be missing. Whose move it is, is still known —
    // inventing the date to complete the sentence would not be.
    show(<DealPulse card={card({ reply_to: MAIL_ID })} timeline={[]} />);
    expect(screen.getByText(/It's your move/)).toBeInTheDocument();
    expect(screen.getByText(/nobody has answered/)).toBeInTheDocument();
  });

  it("says it is their move when nobody here is owed an answer", () => {
    show(<DealPulse card={card({ reply_to: null })} timeline={[]} />);
    expect(screen.getByText(/Their move/)).toBeInTheDocument();
  });

  it("renders nothing at all while the card is loading", () => {
    // A headline that guessed would be the loudest wrong thing on the page.
    const { container } = show(<DealPulse card={undefined} timeline={[]} />);
    expect(container.querySelector(".d360-pulse")).toBeNull();
  });
});

describe("the readings say what is wrong, with the figure behind it", () => {
  it("marks a close date no human confirmed", () => {
    // The whole reason the close card exists: the nightly run replaces a date
    // that aged into the past, and until now the page rendered that
    // identically to one somebody agreed with the buyer.
    show(
      <DealStrip
        deal={deal({
          expected_close_date: "2099-09-30",
          close_date_provisional: true,
          forecast_category: "best_case",
        })}
        coverageWithheld={false}
      />,
    );
    expect(
      screen.getByText(/provisional, not confirmed by a human/),
    ).toBeInTheDocument();
  });

  it("says nothing about confirmation when a human set the date", () => {
    show(
      <DealStrip
        deal={deal({
          expected_close_date: "2099-09-30",
          close_date_provisional: false,
        })}
        coverageWithheld={false}
      />,
    );
    expect(screen.queryByText(/provisional/)).toBeNull();
  });

  it("names the day the customer asked us to wait until", () => {
    // wait_until has been settable in the deal form and rendered nowhere.
    show(
      <DealStrip
        deal={deal({
          expected_close_date: "2099-09-30",
          wait_until: "2099-08-15",
        })}
        coverageWithheld={false}
      />,
    );
    expect(screen.getByText(/asked us to wait until/)).toBeInTheDocument();
  });

  it("reports a stalled deal as danger, not as a plain figure", () => {
    show(
      <DealStrip
        deal={deal({ stalled: true, last_activity_at: "2026-05-20T09:00:00Z" })}
        coverageWithheld={false}
      />,
    );
    expect(screen.getByText(/stalled/)).toBeInTheDocument();
  });

  it("counts engaged seats against the total", () => {
    show(
      <DealStrip
        deal={deal()}
        coverageWithheld={false}
        coverage={{
          deal_id: DEAL_ID,
          stakeholders: [
            { person_id: "p1", role: "champion", engaged: true },
            { person_id: "p2", role: "user", engaged: false },
          ],
          our_side: [],
          risks: [],
          sections_omitted: [],
        }}
      />,
    );
    expect(screen.getByText("1 of 2 engaged")).toBeInTheDocument();
    expect(screen.getByText(/a champion is named/)).toBeInTheDocument();
  });

  it("says the people are hidden rather than reporting nobody", () => {
    // Withheld and empty are different answers, and a card that read one as
    // the other would report a clean bill of health from a check that never
    // ran.
    show(<DealStrip deal={deal()} coverageWithheld={true} />);
    expect(
      screen.getByText(/may not read who is on this deal/),
    ).toBeInTheDocument();
    expect(screen.queryByText(/No stakeholder is recorded/)).toBeNull();
  });

  it("names the offer's state and never a date it was sent", () => {
    // There is no send timestamp on an Offer — created_at, updated_at and
    // accepted_at are the only three, and none of them is the send. "Awaiting
    // reply N days" would have had to invent the start of that interval.
    show(
      <DealStrip
        deal={deal()}
        coverageWithheld={false}
        offers={[
          {
            id: "o1",
            deal_id: DEAL_ID,
            offer_number: "2026-0184",
            revision: 2,
            status: "sent",
            currency: "EUR",
            net_minor: 4_500_000,
            tax_minor: 0,
            gross_minor: 4_500_000,
            line_items: [],
            source: "ui",
            version: 1,
            created_at: "2026-08-12T00:00:00Z",
            updated_at: "2026-08-12T00:00:00Z",
          } as unknown as components["schemas"]["Offer"],
        ]}
      />,
    );
    expect(screen.getByText(/Offer 2026-0184 · Sent/)).toBeInTheDocument();
    expect(screen.queryByText(/awaiting reply/i)).toBeNull();
  });
});

describe("the rail says who is on the deal", () => {
  it("lists a seat with its role and whether they are engaged", () => {
    show(
      <DealSeats
        pending={false}
        withheld={false}
        overlay={false}
        coverage={{
          deal_id: DEAL_ID,
          stakeholders: [
            {
              person_id: "p1",
              person_name: "Thorsten Ortner",
              role: "economic_buyer",
              engaged: true,
            },
          ],
          our_side: [],
          risks: [],
          sections_omitted: [],
        }}
      />,
    );
    expect(screen.getByText("Thorsten Ortner")).toBeInTheDocument();
    expect(screen.getByText("Engaged")).toBeInTheDocument();
  });

  it("states the overlay refusal instead of disappearing with the rail", () => {
    // The coverage read is disabled against a mirrored deal, so no seats will
    // ever arrive. Dropping the card — which is what happened before, because
    // the whole rail was omitted in overlay mode — draws a deal with nobody on
    // it: an absence the server never claimed.
    show(<DealSeats pending={false} withheld={false} overlay={true} />);
    expect(
      screen.getByText(/Not available while reading from HubSpot/i),
    ).toBeInTheDocument();
  });

  it("shows a seat whose identity is withheld without dropping the row", () => {
    // The seat still counts toward coverage, so it is shown; only the name is
    // withheld. Dropping the row would undercount the deal's own coverage.
    show(
      <DealSeats
        pending={false}
        withheld={false}
        overlay={false}
        coverage={{
          deal_id: DEAL_ID,
          stakeholders: [{ person_id: "p1", role: "user", engaged: false }],
          our_side: [],
          risks: [],
          sections_omitted: [],
        }}
      />,
    );
    expect(screen.getByText(en["coverage.seatWithheld"])).toBeInTheDocument();
    expect(screen.getByText("No two-way contact")).toBeInTheDocument();
  });
});

// The facts a rep checks first, on the record's identity line: value, stage,
// owner.
//
// The owner was once on no part of the page — not the header, not the rail,
// not the readings — so "whose deal is this" could only be answered by opening
// Edit.
describe("the identity line says what it is worth, where it is, and whose it is", () => {
  const stages = [
    { id: "st-1", name: "Qualified" },
    { id: "st-2", name: "Proposal" },
  ];

  it("names the stage rather than showing its id", () => {
    show(
      <DealIdentityLine
        deal={{ amount_minor: 6_400_000, currency: "EUR", stage_id: "st-1" }}
        stages={stages}
        locale="en"
      />,
    );
    expect(screen.getByText("Qualified")).toBeInTheDocument();
    expect(screen.queryByText("st-1")).not.toBeInTheDocument();
  });

  it("says a deal is unassigned rather than leaving the owner blank", () => {
    show(
      <DealIdentityLine
        deal={{ amount_minor: 1000, currency: "EUR", stage_id: "st-1" }}
        stages={stages}
        locale="en"
      />,
    );
    // An empty value here reads as a rendering fault. "Unassigned" is a fact
    // about the deal, and it is the one a rep acts on.
    expect(screen.getByText(/Unassigned/)).toBeInTheDocument();
  });

  it("names the field it may not show rather than printing a dash", () => {
    // A rep who may read the deal but not its amount. A bare "—" would read
    // as "this deal has no value", which is a different and wrong statement —
    // and a lone mask among joined facts says only that something is hidden.
    show(
      <DealIdentityLine
        deal={{
          amount_minor: null,
          currency: "EUR",
          stage_id: "st-1",
          masked_fields: ["amount_minor"],
        }}
        stages={stages}
        locale="en"
      />,
    );
    expect(screen.getByText("Value")).toBeInTheDocument();
    expect(screen.queryByText("—")).not.toBeInTheDocument();
  });

  it("never prints a stage id the pipeline cannot name", () => {
    // The case a null stage_id CANNOT test: an id that is present and does not
    // resolve. An overlay-mirror deal carries the incumbent's own pipeline id,
    // and a deal read before its pipeline finishes loading has stages empty —
    // both reach the fallback with a real uuid in hand, and printing it puts a
    // machine identifier where a reader expects "Qualified".
    const foreign = "01a02be8-c8d5-7d9b-bb60-a5e1ad68533c";
    show(
      <DealIdentityLine
        deal={{ amount_minor: 1000, currency: "EUR", stage_id: foreign }}
        stages={stages}
        locale="en"
      />,
    );
    expect(screen.getByText("—")).toBeInTheDocument();
    expect(screen.queryByText(foreign)).not.toBeInTheDocument();
  });
});

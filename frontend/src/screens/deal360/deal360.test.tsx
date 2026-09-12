// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";
import { readFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { DealIdentityLine } from "../dealidentity";
import { DealPulse } from "./dealpulse";
import { DealSeats } from "./dealseats";
import { DEAL_OFFERS_ANCHOR, DealStrip } from "./dealstrip";

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

// One harness, with the query client: the seat cells resolve a contact through
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
            { contact_id: "p1", role: "champion", engaged: true },
            { contact_id: "p2", role: "user", engaged: false },
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

  it("says the contacts are hidden rather than reporting nobody", () => {
    // Withheld and empty are different answers, and a card that read one as
    // the other would report a clean bill of health from a check that never
    // ran.
    show(<DealStrip deal={deal()} coverageWithheld={true} />);
    expect(
      screen.getByText(/may not read who is on this deal/),
    ).toBeInTheDocument();
    expect(screen.queryByText(/No stakeholder is recorded/)).toBeNull();
  });

  // The money reading's way out. Its receipt lists the offers; the door goes to
  // the offers CARD, which is on the same tab one screen down — so it is a
  // scroll rather than a route, and the id is the strip's own so the two cannot
  // drift apart.
  it("reveals the offers card from the money reading", async () => {
    const offers = document.createElement("div");
    offers.id = DEAL_OFFERS_ANCHOR;
    document.body.append(offers);
    // jsdom implements no scrolling at all, so the page's own element is what
    // records the call.
    const scrolled = vi.fn();
    offers.scrollIntoView = scrolled;

    show(<DealStrip deal={deal()} coverageWithheld={false} />);
    await userEvent.setup().click(
      screen.getByRole("button", {
        name: "Open",
        description: en["deal.strip.money"],
      }),
    );

    expect(scrolled).toHaveBeenCalledTimes(1);
    offers.remove();
  });

  // A deal nobody has priced keeps the door, because the offers card is where
  // the price gets written — the reading whose figure is missing is the one
  // whose reader most needs it.
  it("keeps the money door on a deal nobody has priced", () => {
    show(
      <DealStrip
        deal={deal({ amount_minor: undefined, currency: undefined })}
        coverageWithheld={false}
      />,
    );

    expect(
      screen.getByText(en["deal.strip.money.unpriced"]),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", {
        name: "Open",
        description: en["deal.strip.money"],
      }),
    ).toBeInTheDocument();
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
              contact_id: "p1",
              contact_name: "Thorsten Ortner",
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
          stakeholders: [{ contact_id: "p1", role: "user", engaged: false }],
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

  it("says how a won deal was won when no contract carried it", () => {
    // The server treats this answer as load-bearing — the whole justification
    // for letting the deal close without paperwork is that the gap becomes
    // countable — and nothing read it back, so the rep who answered could not
    // check their own answer.
    show(
      <DealIdentityLine
        deal={{
          amount_minor: 1000,
          currency: "EUR",
          stage_id: "st-1",
          status: "won",
          won_without_contract_reason: "purchase_order",
        }}
        stages={stages}
        locale="en"
      />,
    );
    expect(
      screen.getByText(en["deals.winReasonPurchaseOrder"]),
    ).toBeInTheDocument();
  });

  it("prints the words a contact wrote rather than the category they chose", () => {
    // `other` is the only reason carrying a detail, and the detail is the only
    // part of this answer somebody typed. "Something else: renewed on a
    // handshake" says the category twice and buries it.
    show(
      <DealIdentityLine
        deal={{
          amount_minor: 1000,
          currency: "EUR",
          stage_id: "st-1",
          status: "won",
          won_without_contract_reason: "other",
          won_without_contract_detail: "Renewed on a handshake at the fair",
        }}
        stages={stages}
        locale="en"
      />,
    );
    expect(
      screen.getByText("Renewed on a handshake at the fair"),
    ).toBeInTheDocument();
    expect(
      screen.queryByText(en["deals.winReasonOther"]),
    ).not.toBeInTheDocument();
  });

  it("keeps a long hand-written reason readable rather than only hoverable", () => {
    // The identity line is a row of SHORT facts, so this one is bounded — but
    // VISUALLY, with the whole string in the DOM. An earlier version trimmed it
    // in TS and put the rest in a `title`, which reads as solved and is not: a
    // tooltip wants a mouse, is ignored by most screen readers, and never
    // appears for a keyboard or touch reader. The contact most likely to look is
    // the one checking the words they just typed.
    const long =
      `Renewed on a handshake at the trade fair ${"and again ".repeat(20)}`.trim();
    show(
      <DealIdentityLine
        deal={{
          amount_minor: 1000,
          currency: "EUR",
          stage_id: "st-1",
          status: "won",
          won_without_contract_reason: "other",
          won_without_contract_detail: long,
        }}
        stages={stages}
        locale="en"
      />,
    );
    // Every word of it, reachable by a reader that does not hover.
    expect(screen.getByText(long)).toBeInTheDocument();
    // And the stage is still on the line beside it, which is what the bound is
    // for. What this pins is that the fact carries the class the bound hangs
    // on, so removing it is a failing test rather than a silently wide line;
    // that the class does not CLIP is held by the rule itself, below, because
    // jsdom applies no stylesheet and presence in the DOM proves nothing about
    // what a reader can see.
    expect(screen.getByText(long)).toHaveClass("deal-win-detail");
    expect(screen.getByText("Qualified")).toBeInTheDocument();
  });

  // The bound is on WIDTH, never on content — asserted against the stylesheet,
  // because jsdom applies none and a rendered tree cannot tell a wrapped value
  // from a clipped one.
  //
  // This has been wrong twice in opposite directions, which is why it is held
  // rather than described. Clipped with the rest in a `title` needs a mouse;
  // clipped with no `title` is unreadable for everyone. Either way the reader
  // who loses is the contact checking the words they just typed, and the value
  // exists to be audited.
  it("bounds the won-reason detail's width and never its content", () => {
    const css = readFileSync(
      join(resolve(__dirname, ".."), "dealstatus.css"),
      "utf8",
    );
    const rule = /\.deal-win-detail\s*\{([^}]*)\}/.exec(css);
    expect(rule, ".deal-win-detail is gone from dealstatus.css").not.toBeNull();
    const body = rule?.[1] ?? "";

    // A width bound, so the identity line stays a line of short facts.
    expect(body).toMatch(/max-width:/);
    // And nothing that hides what does not fit inside it.
    for (const clip of [
      /overflow\s*:\s*hidden/,
      /text-overflow\s*:/,
      /white-space\s*:\s*nowrap/,
      /line-clamp\s*:/,
    ]) {
      expect(
        body,
        `.deal-win-detail clips its content (${clip.source}); the value is what a controller is shown`,
      ).not.toMatch(clip);
    }
  });

  it("says nothing about paperwork on a won deal a contract carried", () => {
    // The ordinary case, and it needs no sentence. A line on every won deal
    // would bury the ones that need reading — which is the whole point of
    // showing this at all.
    show(
      <DealIdentityLine
        deal={{
          amount_minor: 1000,
          currency: "EUR",
          stage_id: "st-1",
          status: "won",
        }}
        stages={stages}
        locale="en"
      />,
    );
    for (const key of [
      "deals.winReasonPurchaseOrder",
      "deals.winReasonVerbal",
      "deals.winReasonRenewalByEmail",
      "deals.winReasonImported",
      "deals.winReasonOther",
    ] as const) {
      expect(screen.queryByText(en[key])).not.toBeInTheDocument();
    }
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

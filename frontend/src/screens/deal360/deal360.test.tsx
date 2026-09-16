// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { DealPulse } from "./dealpulse";

// What the deal page owes a reader before they read anything: the sentence
// naming whose move it is, and who is on the deal in the rail beside it. The
// four readings this suite used to cover moved into dealcockpit.test.tsx, and
// the head's facts strip into dealheaderfacts.test.tsx.
//
// This file pins the two clauses that are deliberately absent from the whole
// page. The design asked for "awaiting reply N days" and "they replied twice,
// we replied once", and neither is a fact this product holds — there is no
// send timestamp on an offer and no deal-scoped direction count anywhere. A
// card that reads well and cannot be checked is what this page exists to stop.

afterEach(cleanup);

type DealStatusCard = components["schemas"]["DealStatusCard"];

const DEAL_ID = "01a03000-0000-7000-8000-000000000001";
const MAIL_ID = "01a03000-0000-7000-8000-0000000000aa";

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

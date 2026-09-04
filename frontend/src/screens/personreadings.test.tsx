/** @vitest-environment jsdom */
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { PersonReadings } from "./personreadings";

type Person360 = components["schemas"]["Person360"];

// Four readings, each with three states — a value, an honest "none", and a
// withheld section — and each read off the field the rest of the page reads.
// The two that carry a verdict (whose move, what we owe) are the ones a rep
// acts on, so the rules that colour them are what these pin.

afterEach(cleanup);

const AS_OF = "2026-08-24T09:00:00Z";

function view(extra: Partial<Person360> = {}): Person360 {
  return {
    as_of: AS_OF,
    person: { id: "p1", full_name: "Frédéric de Gombert" },
    sections_omitted: [],
    ...extra,
  } as unknown as Person360;
}

function show(page: Person360) {
  render(
    <LocaleProvider initial="en">
      <PersonReadings view={page} />
    </LocaleProvider>,
  );
  return screen.getByTestId("person-readings");
}

function card(grid: HTMLElement, label: string): HTMLElement {
  const title = within(grid).getByText(label);
  const found = title.closest<HTMLElement>(".stat-card");
  if (!found) {
    throw new Error(`no card labelled "${label}"`);
  }
  return found;
}

describe("whose move it is", () => {
  it("is ours when they wrote last", () => {
    const grid = show(
      view({
        last_inbound_at: "2026-08-20T10:00:00Z",
        last_outbound_at: "2026-08-01T10:00:00Z",
      }),
    );
    const move = card(grid, "Whose move");
    expect(within(move).getByText("Yours")).toBeTruthy();
    expect(within(move).getByText("last from them: 4 days")).toBeTruthy();
  });

  it("counts the messages each way behind the call, off the page the read carries", async () => {
    const grid = show(
      view({
        last_inbound_at: "2026-08-20T10:00:00Z",
        last_outbound_at: "2026-08-01T10:00:00Z",
        activities: {
          data: [
            { id: "a1", kind: "email", direction: "inbound" },
            { id: "a2", kind: "email", direction: "inbound" },
            { id: "a3", kind: "email", direction: "outbound" },
            // A note has no direction and is neither side's message.
            { id: "a4", kind: "note" },
          ],
          page: { has_more: false },
        },
      } as unknown as Partial<Person360>),
    );
    const user = userEvent.setup();
    const move = card(grid, "Whose move");
    await user.click(
      within(move).getByRole("button", { name: "Evidence" }),
    );
    expect(screen.getByText("Reciprocity")).toBeTruthy();
    expect(screen.getByText("2 in · 1 out")).toBeTruthy();
  });

  it("is theirs while a reply is still inside the ordinary wait", () => {
    const grid = show(
      view({
        last_inbound_at: "2026-08-18T10:00:00Z",
        last_outbound_at: "2026-08-22T10:00:00Z",
      }),
    );
    expect(within(card(grid, "Whose move")).getByText("Theirs")).toBeTruthy();
  });

  it("reads gone quiet once their silence outlasts the rail's own span", () => {
    // Fifteen days since they wrote, and our last word is later than theirs:
    // the same fourteen-day rule the rail's overall reading turns on.
    const grid = show(
      view({
        last_inbound_at: "2026-08-09T10:00:00Z",
        last_outbound_at: "2026-08-22T10:00:00Z",
      }),
    );
    expect(
      within(card(grid, "Whose move")).getByText("Gone quiet"),
    ).toBeTruthy();
  });

  it("awaits a reply to a first word rather than calling it quiet", () => {
    // We wrote yesterday and they have never written: theirs to answer, with
    // no alarm yet — the same fourteen days a contact who HAS written gets.
    const grid = show(view({ last_outbound_at: "2026-08-23T10:00:00Z" }));
    const move = card(grid, "Whose move");
    expect(within(move).getByText("Theirs")).toBeTruthy();
    expect(within(move).getByText("nothing from them yet")).toBeTruthy();
  });

  it("reads a first word left unanswered past the span as gone quiet", () => {
    const grid = show(view({ last_outbound_at: "2026-08-01T10:00:00Z" }));
    expect(
      within(card(grid, "Whose move")).getByText("Gone quiet"),
    ).toBeTruthy();
  });

  it("says nothing was ever exchanged rather than inventing a side", () => {
    const grid = show(view());
    expect(
      within(card(grid, "Whose move")).getByText("Never spoken"),
    ).toBeTruthy();
  });

  it("says the reading is withheld, never that there is none", () => {
    const grid = show(view({ sections_omitted: ["last_touch"] }));
    expect(
      within(card(grid, "Whose move")).getByText("Not shown"),
    ).toBeTruthy();
  });
});

describe("what we owe them", () => {
  it("counts only our own open commitments, and the lateness of the worst", () => {
    const grid = show(
      view({
        claims: [
          {
            id: "c1",
            kind: "commitment_ours",
            body: "Send the breakdown",
            status: "open",
            due_at: "2026-08-05T09:00:00Z",
            source_activity_id: "a1",
            source_quote: "",
            needs_review: false,
          },
          {
            id: "c2",
            kind: "commitment_ours",
            body: "Book the kickoff",
            status: "open",
            due_at: "2026-09-08T09:00:00Z",
            source_activity_id: "a1",
            source_quote: "",
            needs_review: false,
          },
          {
            id: "c3",
            kind: "commitment_theirs",
            body: "Confirm the plan",
            status: "open",
            source_activity_id: "a1",
            source_quote: "",
            needs_review: false,
          },
          {
            id: "c4",
            kind: "commitment_ours",
            body: "Already done",
            status: "done",
            source_activity_id: "a1",
            source_quote: "",
            needs_review: false,
          },
        ],
      }),
    );
    const promises = card(grid, "Open promises");
    expect(within(promises).getByText("2")).toBeTruthy();
    expect(within(promises).getByText("overdue 19 days")).toBeTruthy();
  });

  it("says nothing is owed when no promise of ours is open", () => {
    const grid = show(view({ claims: [] }));
    expect(
      within(card(grid, "Open promises")).getByText("nothing owed"),
    ).toBeTruthy();
  });
});

describe("what they decide and when we next meet", () => {
  it("prices the deal in the reader's own notation and names the stage", () => {
    const grid = show(
      view({
        commercial: {
          committee: [],
          deal: {
            deal_id: "d1",
            title: "PIM rollout Phase 2",
            stage: "Negotiation",
            amount_minor: 18_550_000,
            currency: "EUR",
            close_date: "2026-09-09",
          },
        },
        next_meeting: {
          activity_id: "m1",
          starts_at: "2026-08-26T14:00:00Z",
          subject: "Demo, Product Cloud",
        },
      }),
    );
    const deal = card(grid, "Deals they decide");
    // en-GB compact notation, one decimal: the reader's own spelling. The
    // sample carries a real tenth, because whether a whole thousand shows a
    // trailing ".0" is the ICU build's decision, not this reading's.
    expect(within(deal).getByText("€185.5k")).toBeTruthy();
    expect(within(deal).getByText(/Negotiation/)).toBeTruthy();
    const meeting = card(grid, "Next meeting");
    expect(within(meeting).getByText("Demo, Product Cloud")).toBeTruthy();
  });

  it("says there is no open deal, which is an answer and not an absence", () => {
    const grid = show(view({ commercial: { committee: [] } }));
    expect(
      within(card(grid, "Deals they decide")).getByText("No open deal"),
    ).toBeTruthy();
  });
});

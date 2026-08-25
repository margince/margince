/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { CompanySpine } from "./companyspine";

// The thread's one rule: it may only draw what the payload supports. Every
// stop is a record or a date, and the ONE stop with neither behind it — the
// silence — is arithmetic between two dates the payload does carry.

type View = components["schemas"]["Organization360"];

const page = { has_more: false, next_cursor: null };
const AS_OF = "2026-08-25T09:00:00Z";
const SPOKE = "2026-08-18T09:00:00Z";

function view(overrides: Record<string, unknown> = {}): View {
  return {
    as_of: AS_OF,
    organization: {
      id: "o-1",
      display_name: "Kugellager",
      captured_by: "human:u1",
      source: "manual",
      version: 1,
      created_at: "2026-06-01T08:00:00Z",
      updated_at: "2026-08-01T08:00:00Z",
    },
    sections_omitted: [],
    ...overrides,
  } as unknown as View;
}

function draw(v: View) {
  render(
    <LocaleProvider initial="en">
      <CompanySpine view={v} />
    </LocaleProvider>,
  );
}

afterEach(cleanup);

describe("the silence between the last word and today", () => {
  it("counts the days since the last conversation and says nobody replied", () => {
    draw(
      view({
        last_outbound_at: SPOKE,
        last_inbound_at: null,
        health: { last_meeting_at: SPOKE, single_threaded: true },
      }),
    );

    expect(screen.getByText("7 days")).toBeTruthy();
    expect(screen.getByText("They have never written back")).toBeTruthy();
    expect(
      screen.getByText("One contact, and no reply from them"),
    ).toBeTruthy();
  });

  it("draws no gap when they answered after we did", () => {
    // A conversation in progress is not a silence, and telling a reader to
    // chase somebody who has already written is the one thing this stop must
    // never do.
    draw(
      view({
        last_outbound_at: SPOKE,
        last_inbound_at: "2026-08-24T09:00:00Z",
        health: { last_meeting_at: SPOKE },
      }),
    );

    expect(screen.queryByText(/days/)).toBeNull();
    expect(screen.queryByText("They have never written back")).toBeNull();
  });

  it("tells a stalled relationship from one that never started", () => {
    // Both are silence; they lead to different moves. A reply that stopped is
    // a relationship to revive, and no reply at all is one to begin.
    draw(
      view({
        last_outbound_at: SPOKE,
        last_inbound_at: "2026-07-01T09:00:00Z",
        health: { last_meeting_at: SPOKE },
      }),
    );

    expect(screen.getByText("Silence since then")).toBeTruthy();
    expect(screen.queryByText("They have never written back")).toBeNull();
  });

  it("draws nothing at all on an account nobody has spoken to", () => {
    // No conversation is no thread: a silence measured from nothing would be
    // counting days since the record was created, which is not a fact about
    // the relationship.
    const { container } = render(
      <LocaleProvider initial="en">
        <CompanySpine view={view({ last_outbound_at: null })} />
      </LocaleProvider>,
    );
    expect(container.textContent).toBe("");
  });
});

describe("what the thread says is coming", () => {
  it("puts a slipped commitment on the thread as late, not as ahead", () => {
    draw(
      view({
        last_outbound_at: SPOKE,
        health: { last_meeting_at: SPOKE },
        next_steps: {
          data: [
            {
              activity_id: "s-1",
              subject: "Send the NDA",
              due_at: "2026-08-20T09:00:00Z",
              overdue: true,
            },
          ],
          page,
        },
      }),
    );

    expect(screen.getByText("Send the NDA")).toBeTruthy();
    expect(screen.getByText("Past its date")).toBeTruthy();
  });

  it("names what is riding on the close date", () => {
    draw(
      view({
        last_outbound_at: SPOKE,
        health: { last_meeting_at: SPOKE },
        state_strip: {
          account: { lifecycle: "opportunity", relationship_types: [] },
          commercial: {
            open_count: 1,
            stalled_count: 0,
            priced_count: 1,
            converted_count: 0,
            open_pipeline_minor_base: 4_800_000,
            base_currency: "EUR",
            next_close_on: "2026-10-20",
          },
        },
      }),
    );

    expect(screen.getByText("Expected close")).toBeTruthy();
    expect(screen.getByText(/riding on it/)).toBeTruthy();
  });

  it("says a pipeline is unpriced rather than printing it as worth nothing", () => {
    draw(
      view({
        last_outbound_at: SPOKE,
        health: { last_meeting_at: SPOKE },
        state_strip: {
          account: { lifecycle: "opportunity", relationship_types: [] },
          commercial: {
            open_count: 1,
            stalled_count: 0,
            priced_count: 0,
            converted_count: 0,
            next_close_on: "2026-10-20",
          },
        },
      }),
    );

    expect(screen.getByText("1 open, none priced yet")).toBeTruthy();
    expect(screen.queryByText(/€0/)).toBeNull();
  });
});

// What the thread's head says, and what it refuses to say it about.
//
// These rules moved here with the fact itself: the day's brief used to carry a
// "Last exchange" reading beside a thread that already dated the same
// conversation, so the card printed one exchange twice. The thread kept the
// fact and the brief lost the reading — and these are the three ways picking
// the wrong row goes wrong.
describe("the last thing that was actually said", () => {
  // The 360 serves activities newest-first (ORDER BY occurred_at DESC), so the
  // head of the list is the most recent and the thread makes no ordering
  // decision of its own.
  function activities(
    rows: readonly { id: string; kind: string; subject: string; at: string }[],
  ) {
    return {
      data: rows.map((row) => ({
        id: row.id,
        kind: row.kind,
        is_done: false,
        subject: row.subject,
        occurred_at: row.at,
        source: "manual",
        captured_by: "human:test",
        created_at: row.at,
        updated_at: row.at,
      })),
      page,
    };
  }

  it("names the newest exchange, not an older one", () => {
    draw(
      view({
        last_outbound_at: SPOKE,
        activities: activities([
          { id: "a-1", kind: "email", subject: "Where we landed", at: SPOKE },
          {
            id: "a-2",
            kind: "email",
            subject: "An older thread",
            at: "2026-07-01T09:00:00Z",
          },
        ]),
      }),
    );

    expect(screen.getByText("Where we landed")).toBeTruthy();
    expect(screen.queryByText("An older thread")).toBeNull();
  });

  // The timeline is unfiltered: tasks live in the same table and sort by the
  // same column. A task is something we wrote to ourselves, not something that
  // was said to the account.
  it("skips a task when picking what was last said", () => {
    draw(
      view({
        last_outbound_at: SPOKE,
        activities: activities([
          {
            id: "a-task",
            kind: "task",
            subject: "Chase the signature",
            at: "2026-08-20T09:00:00Z",
          },
          { id: "a-mail", kind: "email", subject: "About capacity", at: SPOKE },
        ]),
      }),
    );

    expect(screen.getByText("About capacity")).toBeTruthy();
    expect(screen.queryByText("Chase the signature")).toBeNull();
  });

  // `occurred_at DESC` sorts a meeting booked for next week to the head of the
  // list. It has not been said yet, and dating the thread's head from it would
  // put the account's last word in the future.
  it("skips an activity that has not happened yet", () => {
    draw(
      view({
        last_outbound_at: SPOKE,
        activities: activities([
          {
            id: "a-future",
            kind: "meeting",
            subject: "Executive alignment",
            at: "2026-09-20T09:00:00Z",
          },
          { id: "a-past", kind: "email", subject: "About scope", at: SPOKE },
        ]),
      }),
    );

    expect(screen.getByText("About scope")).toBeTruthy();
    expect(screen.queryByText("Executive alignment")).toBeNull();
  });
});

// The gap is counted from the last thing WE sent, and from nothing else.
//
// An account met in June, written to in July, whose only reply came in
// between: the meeting date would report six weeks of silence that did not
// happen, and comparing the reply against the MEETING would report no silence
// at all on an account that has been waiting a month.
describe("what the waiting is measured from", () => {
  const MET = "2026-06-26T09:00:00Z";
  const THEY_WROTE = "2026-07-26T09:00:00Z";
  const WE_WROTE = "2026-07-29T09:00:00Z";

  it("counts from our last message, not from the older meeting", () => {
    draw(
      view({
        last_outbound_at: WE_WROTE,
        last_inbound_at: THEY_WROTE,
        health: { last_meeting_at: MET, single_threaded: false },
      }),
    );

    // 29 July to 25 August, not 26 June to 25 August.
    expect(screen.getByText("27 days")).toBeTruthy();
    expect(screen.queryByText("60 days")).toBeNull();
    // They answered once, so this is a thread that stalled rather than one
    // that never started.
    expect(screen.getByText("Silence since then")).toBeTruthy();
  });

  it("draws no gap once they have answered our latest message", () => {
    draw(
      view({
        last_outbound_at: MET,
        last_inbound_at: WE_WROTE,
        health: { last_meeting_at: MET },
      }),
    );

    expect(screen.queryByText("Silence since then")).toBeNull();
    expect(screen.queryByText("They have never written back")).toBeNull();
  });
});

// A meeting is not a message somebody owes a reply to.
//
// `silenceSince` falls back to the last meeting so the thread's HEAD still has
// a date when the timeline is withheld. The gap may not borrow that fallback:
// an account we met once and never wrote to is not ignoring us, and drawing
// "they have never written back" over it invents a slight that did not happen.
describe("an account nobody has written to", () => {
  it("draws no gap when there is a meeting but no outbound message", () => {
    draw(
      view({
        last_outbound_at: null,
        last_inbound_at: null,
        health: { last_meeting_at: "2026-06-26T09:00:00Z" },
      }),
    );

    // The head stop still dates the thread from the meeting.
    expect(screen.getByText("You last spoke")).toBeTruthy();
    // But nothing is waiting on a reply.
    expect(screen.queryByText("They have never written back")).toBeNull();
    expect(screen.queryByText("Silence since then")).toBeNull();
  });
});

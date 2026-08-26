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

// The 360 serves activities newest-first (ORDER BY occurred_at DESC), so the
// head of the list is the most recent and the thread makes no ordering
// decision of its own.
function activities(
  rows: readonly {
    id: string;
    kind: string;
    subject: string;
    at: string;
    thread?: string;
  }[],
) {
  return {
    data: rows.map((row) => ({
      id: row.id,
      kind: row.kind,
      is_done: false,
      subject: row.subject,
      occurred_at: row.at,
      thread_key: row.thread ?? null,
      source: "manual",
      captured_by: "human:test",
      created_at: row.at,
      updated_at: row.at,
    })),
    page,
  };
}

// What the thread's head says, and what it refuses to say it about.
//
// These rules moved here with the fact itself: the day's brief used to carry a
// "Last exchange" reading beside a thread that already dated the same
// conversation, so the card printed one exchange twice. The thread kept the
// fact and the brief lost the reading — and these are the three ways picking
// the wrong row goes wrong.
describe("the last thing that was actually said", () => {
  it("heads the thread with the newest conversation", () => {
    // The older conversation stays on the thread — it is the history a reader
    // came for — but only the newest carries the label the gap counts from.
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

    const titles = [...document.querySelectorAll(".co-spine-title")].map(
      (node) => node.textContent,
    );
    // Oldest first down the page, and "You last spoke" sits on the newest.
    expect(titles).toEqual([
      "An older thread",
      "You last spoke",
      "They have never written back",
    ]);
    expect(screen.getByText("Where we landed")).toBeTruthy();
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

// One stop per conversation, not per message.
//
// The thread exists so a reader who does not remember an account can see what
// was going on here. Six emails drawn as six stops is the history tab, and
// six emails drawn as ONE stop was the shape that sent that reader scrolling
// past the day's suggestions to find the kickoff that explains the account.
describe("the conversations behind the last word", () => {
  const KICKOFF = "thread-kickoff";
  const INVOICE = "thread-invoice";

  it("folds a conversation into one stop and counts its messages", () => {
    draw(
      view({
        last_outbound_at: SPOKE,
        activities: activities([
          {
            id: "m-3",
            kind: "email",
            subject: "Re: Kickoff",
            at: SPOKE,
            thread: KICKOFF,
          },
          {
            id: "m-2",
            kind: "email",
            subject: "Re: Kickoff",
            at: "2026-08-15T09:00:00Z",
            thread: KICKOFF,
          },
          {
            id: "m-1",
            kind: "email",
            subject: "Kickoff",
            at: "2026-08-12T09:00:00Z",
            thread: KICKOFF,
          },
        ]),
      }),
    );

    // One conversation, dated at its newest message and named once.
    expect(screen.getAllByText("Kickoff")).toHaveLength(1);
    expect(screen.getByText("18 Aug 2026")).toBeTruthy();
    expect(screen.queryByText("12 Aug 2026")).toBeNull();
  });

  it("keeps two conversations apart and puts the older one first", () => {
    draw(
      view({
        last_outbound_at: SPOKE,
        activities: activities([
          {
            id: "i-1",
            kind: "email",
            subject: "Invoice query",
            at: SPOKE,
            thread: INVOICE,
          },
          {
            id: "k-2",
            kind: "meeting",
            subject: "Kickoff",
            at: "2026-07-02T09:00:00Z",
            thread: KICKOFF,
          },
          {
            id: "k-1",
            kind: "email",
            subject: "Kickoff",
            at: "2026-07-01T09:00:00Z",
            thread: KICKOFF,
          },
        ]),
      }),
    );

    const titles = [...document.querySelectorAll(".co-spine-title")].map(
      (node) => node.textContent,
    );
    expect(titles).toEqual([
      "Kickoff",
      "You last spoke",
      "They have never written back",
    ]);
    expect(screen.getByText("2 messages")).toBeTruthy();
    expect(screen.getByText("Invoice query")).toBeTruthy();
  });

  // Capture threads what a provider threaded. A note or a hand-logged call
  // carries no thread_key, and "Re: Kickoff" beside "Kickoff" is one
  // conversation the reader would never accept as two.
  it("recognises a reply as the same conversation when nothing threaded it", () => {
    draw(
      view({
        last_outbound_at: SPOKE,
        activities: activities([
          { id: "u-2", kind: "email", subject: "Re: Pricing", at: SPOKE },
          {
            id: "u-1",
            kind: "email",
            subject: "Pricing",
            at: "2026-08-11T09:00:00Z",
          },
        ]),
      }),
    );

    expect(screen.getAllByText("Pricing")).toHaveLength(1);
  });

  // A long-running account has more history than the thread draws, and a
  // reader told nothing would read three stops as all there ever was.
  it("says how many earlier conversations it did not draw", () => {
    draw(
      view({
        last_outbound_at: SPOKE,
        activities: activities([
          { id: "c-5", kind: "email", subject: "Fifth", at: SPOKE },
          {
            id: "c-4",
            kind: "email",
            subject: "Fourth",
            at: "2026-08-10T09:00:00Z",
          },
          {
            id: "c-3",
            kind: "email",
            subject: "Third",
            at: "2026-08-05T09:00:00Z",
          },
          {
            id: "c-2",
            kind: "email",
            subject: "Second",
            at: "2026-07-20T09:00:00Z",
          },
          {
            id: "c-1",
            kind: "email",
            subject: "First",
            at: "2026-07-01T09:00:00Z",
          },
        ]),
      }),
    );

    expect(screen.getByText("2 earlier conversations")).toBeTruthy();
    // The two it dropped are the OLDEST, and the roll-up is dated at the
    // oldest of them so the thread still starts where the account started.
    expect(screen.queryByText("First")).toBeNull();
    expect(screen.queryByText("Second")).toBeNull();
    expect(screen.getByText("1 Jul 2026")).toBeTruthy();
    expect(screen.getByText("Third")).toBeTruthy();
  });

  // The 360 sends one capped page of the timeline. Counting conversations on a
  // page that was cut would print a number the reader can check against the
  // history tab and find wrong, and dating it would claim the account began at
  // whatever the page happened to reach.
  it("refuses to count or date the earlier conversations on a cut page", () => {
    const cut = activities([
      { id: "p-4", kind: "email", subject: "Fourth", at: SPOKE },
      {
        id: "p-3",
        kind: "email",
        subject: "Third",
        at: "2026-08-10T09:00:00Z",
      },
      {
        id: "p-2",
        kind: "email",
        subject: "Second",
        at: "2026-08-05T09:00:00Z",
      },
      {
        id: "p-1",
        kind: "email",
        subject: "First",
        at: "2026-07-01T09:00:00Z",
      },
    ]);
    draw(
      view({
        last_outbound_at: SPOKE,
        activities: { ...cut, page: { has_more: true, next_cursor: "c" } },
      }),
    );

    expect(screen.getByText("More conversations before this")).toBeTruthy();
    expect(screen.queryByText("One earlier conversation")).toBeNull();
    expect(screen.queryByText("1 Jul 2026")).toBeNull();
  });

  it("counts one dropped conversation in the singular", () => {
    draw(
      view({
        last_outbound_at: SPOKE,
        activities: activities([
          { id: "d-4", kind: "email", subject: "Fourth", at: SPOKE },
          {
            id: "d-3",
            kind: "email",
            subject: "Third",
            at: "2026-08-10T09:00:00Z",
          },
          {
            id: "d-2",
            kind: "email",
            subject: "Second",
            at: "2026-08-05T09:00:00Z",
          },
          {
            id: "d-1",
            kind: "email",
            subject: "First",
            at: "2026-07-01T09:00:00Z",
          },
        ]),
      }),
    );

    expect(screen.getByText("One earlier conversation")).toBeTruthy();
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

// What the grouping must not do to the rows a real timeline carries.
describe("the rows a conversation is recognised from", () => {
  it("folds a chain that was replied to and forwarded", () => {
    // A mail client stacks its markers. Taking one off files the same chain
    // under two names, which is two stops for one conversation.
    draw(
      view({
        last_outbound_at: SPOKE,
        activities: activities([
          { id: "r-3", kind: "email", subject: "Re: Fwd: Pricing", at: SPOKE },
          {
            id: "r-2",
            kind: "email",
            subject: "Re: Re: Pricing",
            at: "2026-08-14T09:00:00Z",
          },
          {
            id: "r-1",
            kind: "email",
            subject: "Pricing",
            at: "2026-08-11T09:00:00Z",
          },
        ]),
      }),
    );

    // One conversation, not three: the chain is the newest stop, so its
    // subject is the head's detail and the thread names it once.
    expect(screen.getAllByText("Pricing")).toHaveLength(1);
    const titles = [...document.querySelectorAll(".co-spine-title")].map(
      (node) => node.textContent,
    );
    expect(titles).toEqual(["You last spoke", "They have never written back"]);
  });

  it("skips a subject that is blank or nothing but markers", () => {
    // Two hand-logged calls with no real subject are not one conversation,
    // and a stop with an empty title names nothing at all.
    draw(
      view({
        last_outbound_at: SPOKE,
        activities: activities([
          { id: "b-2", kind: "call", subject: "   ", at: SPOKE },
          {
            id: "b-1",
            kind: "call",
            subject: "Re:",
            at: "2026-08-14T09:00:00Z",
          },
        ]),
      }),
    );

    const titles = [...document.querySelectorAll(".co-spine-title")].map(
      (node) => node.textContent,
    );
    expect(titles).toEqual(["You last spoke", "They have never written back"]);
  });

  it("keeps a provider thread id out of the subject's key space", () => {
    // A thread id shaped like the subject fallback would otherwise merge an
    // unrelated conversation into one it has nothing to do with.
    draw(
      view({
        last_outbound_at: SPOKE,
        activities: activities([
          {
            id: "n-2",
            kind: "email",
            subject: "Renewal",
            at: SPOKE,
            thread: "subject:pricing",
          },
          {
            id: "n-1",
            kind: "email",
            subject: "Pricing",
            at: "2026-08-11T09:00:00Z",
          },
        ]),
      }),
    );

    expect(screen.getByText("Pricing")).toBeTruthy();
    expect(screen.getByText("Renewal")).toBeTruthy();
  });

  // The same instant is written with any offset. Comparing the strings reads
  // 08:30-02:00 as earlier than 09:00Z when it is ninety minutes later, which
  // puts a meeting that has not happened at the head of the thread.
  it("orders an offset timestamp by its instant, not by its text", () => {
    draw(
      view({
        as_of: "2026-08-25T09:00:00Z",
        last_outbound_at: SPOKE,
        activities: activities([
          {
            id: "z-2",
            kind: "meeting",
            subject: "Not yet held",
            at: "2026-08-25T08:30:00-02:00",
          },
          {
            id: "z-1",
            kind: "email",
            subject: "Already said",
            at: "2026-08-25T10:00:00+02:00",
          },
        ]),
      }),
    );

    // 08:30-02:00 is 10:30Z and still ahead; 10:00+02:00 is 08:00Z and past.
    expect(screen.queryByText("Not yet held")).toBeNull();
    expect(screen.getByText("Already said")).toBeTruthy();
  });

  it("drops a row whose timestamp cannot be read at all", () => {
    draw(
      view({
        last_outbound_at: SPOKE,
        activities: activities([
          { id: "bad", kind: "email", subject: "Unreadable date", at: "!" },
          {
            id: "ok",
            kind: "email",
            subject: "Readable",
            at: "2026-08-11T09:00:00Z",
          },
        ]),
      }),
    );

    expect(screen.queryByText("Unreadable date")).toBeNull();
    expect(screen.getByText("Readable")).toBeTruthy();
  });
});

// The thread's head and the gap beneath it are one reading of one fact.
describe("a last word the thread cannot name", () => {
  it("dates the head from our last message even when it has no subject", () => {
    // The gap counts from the 18 August outbound. Labelling the 1 August
    // conversation "You last spoke" would date the head at one message and
    // measure the silence from another.
    draw(
      view({
        last_outbound_at: SPOKE,
        last_inbound_at: null,
        activities: activities([
          {
            id: "s-1",
            kind: "email",
            subject: "Pricing",
            at: "2026-08-01T09:00:00Z",
          },
        ]),
      }),
    );

    const stops = [...document.querySelectorAll(".co-spine-stop")].map(
      (stop) => ({
        when: stop.querySelector(".co-spine-when")?.textContent,
        title: stop.querySelector(".co-spine-title")?.textContent,
      }),
    );
    expect(stops).toEqual([
      { when: "1 Aug 2026", title: "Pricing" },
      { when: "18 Aug 2026", title: "You last spoke" },
      { when: "7 days", title: "They have never written back" },
    ]);
  });
});

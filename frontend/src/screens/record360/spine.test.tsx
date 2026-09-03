/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { RecordSpine } from "./spine";

// The thread's one rule: it may only draw what the payload supports. Every
// stop is a record or a date, and the ONE stop with neither behind it — the
// silence — is arithmetic between two dates the payload does carry.

type View = components["schemas"]["Organization360"];

const page = { has_more: false, next_cursor: null };

// A `nameOf` over a fixed table, keyed "<entity type>:<id>" — the one spelling
// the thread's own resolver uses, so a test cannot accidentally prove a lookup
// the record page could not perform.
function resolver(names: Readonly<Record<string, string>>) {
  return (entityType: string, entityId: string) =>
    names[`${entityType}:${entityId}`];
}
const AS_OF = "2026-08-25T09:00:00Z";
const SPOKE = "2026-08-18T09:00:00Z";
// Today's marker's own headline row, read as one string: the word and, under
// it, the day and month of the read's `as_of` — no year, since the year of
// today is the one date on the axis a reader already knows. Two elements in
// one row, so a textContent walk sees them joined.
const TODAY = "Today25 Aug";

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

function draw(
  v: View,
  nameOf?: (entityType: string, entityId: string) => string | undefined,
) {
  render(
    <LocaleProvider initial="en">
      <RecordSpine
        source={v}
        commercial={v?.state_strip?.commercial}
        nameOf={nameOf}
      />
    </LocaleProvider>,
  );
}

afterEach(cleanup);

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
    links?: readonly { entity_type: string; entity_id: string }[];
    // The colleague who held it. On every row, meeting or not, because the
    // column is on every row — a row that leaves it off is not proving the
    // reading is gated on kind, only that it is absent on that one row.
    host_user_id?: string;
    direction?: "inbound" | "outbound";
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
      links: row.links ?? [],
      host_user_id: row.host_user_id ?? null,
      direction: row.direction ?? null,
      source: "manual",
      captured_by: "human:test",
      created_at: row.at,
      updated_at: row.at,
    })),
    page,
  };
}

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
        <RecordSpine
          source={view({ last_outbound_at: null })}
          commercial={view({ last_outbound_at: null })?.state_strip?.commercial}
        />
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

  it("dates the close and leaves the figure to the readings above it", () => {
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
    // The pipeline's total is the readings row's own figure, one section
    // above. Printed here as well it was one fact in two places, and a reader
    // who found them had to satisfy themselves the two agreed.
    expect(screen.queryByText(/riding on it/)).toBeNull();
    expect(screen.queryByText(/48,000/)).toBeNull();
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
    // Oldest first down the page, "You last spoke" sits on the newest, and
    // nothing is dated ahead so today's line closes the axis.
    expect(titles).toEqual([
      "An older thread",
      "You last spoke",
      "They have never written back",
      TODAY,
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
      TODAY,
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

  it("says more when the page was cut, even with every conversation it holds drawn", () => {
    // Two conversations on a page the server cut: the older history is real
    // and the thread must not read as though these two were all of it.
    draw(
      view({
        last_outbound_at: SPOKE,
        activities: {
          ...activities([
            { id: "c-2", kind: "email", subject: "Second", at: SPOKE },
            {
              id: "c-1",
              kind: "email",
              subject: "First",
              at: "2026-08-05T09:00:00Z",
            },
          ]),
          page: { has_more: true, next_cursor: "c" },
        },
      }),
    );
    expect(screen.getByText("More conversations before this")).toBeTruthy();
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

    expect(screen.getByText("1 earlier conversation")).toBeTruthy();
  });
});

// A mail client stacks reply/forward markers on one chain, and taking one off
// files the same conversation under two names.
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
    expect(titles).toEqual([
      "You last spoke",
      "They have never written back",
      TODAY,
    ]);
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

// Today's marker: a line through the axis, not a stop with a record behind
// it. Where it lands is the whole point — everything left of it happened,
// everything right of it is owed — so these prove the index it lands at
// rather than merely that it renders.
describe("where today's marker sits on the axis", () => {
  function tones(): string[] {
    return [...document.querySelectorAll(".co-spine-stop")].map((node) => {
      const tone = [...node.classList].find(
        (cls) => cls.startsWith("co-spine-") && cls !== "co-spine-stop",
      );
      if (!tone) {
        throw new Error("stop rendered with no tone class");
      }
      return tone;
    });
  }

  it("lands before the first stop that has not happened yet", () => {
    draw(
      view({
        last_outbound_at: SPOKE,
        next_steps: {
          data: [
            {
              activity_id: "s-ahead",
              subject: "Send the proposal",
              due_at: "2026-08-30T09:00:00Z",
              overdue: false,
            },
          ],
          page,
        },
      }),
    );

    // Last word, the silence since it, THEN the marker, THEN what is ahead —
    // a marker one slot early would read the silence as already owed, and one
    // slot late would read the future commitment as already past.
    expect(tones()).toEqual([
      "co-spine-past",
      "co-spine-gap",
      "co-spine-now",
      "co-spine-ahead",
    ]);
  });

  it("keeps a slipped commitment on the past side however it is drawn", () => {
    // Overdue is dated in the past, so it belongs left of today's line even
    // though it shares the axis with a stop that IS still ahead.
    draw(
      view({
        next_steps: {
          data: [
            {
              activity_id: "s-late",
              subject: "Send the NDA",
              due_at: "2026-08-20T09:00:00Z",
              overdue: true,
            },
            {
              activity_id: "s-ahead",
              subject: "Send the proposal",
              due_at: "2026-08-30T09:00:00Z",
              overdue: false,
            },
          ],
          page,
        },
      }),
    );

    expect(tones()).toEqual([
      "co-spine-overdue",
      "co-spine-now",
      "co-spine-ahead",
    ]);
  });

  it("closes the axis when nothing is dated ahead", () => {
    draw(view({ last_outbound_at: SPOKE }));

    const stops = tones();
    expect(stops.at(-1)).toBe("co-spine-now");
  });

  it("names itself with the read's own date, where it falls in the title order", () => {
    draw(
      view({
        last_outbound_at: SPOKE,
        next_steps: {
          data: [
            {
              activity_id: "s-ahead",
              subject: "Send the proposal",
              due_at: "2026-08-30T09:00:00Z",
              overdue: false,
            },
          ],
          page,
        },
      }),
    );

    // The marker's own row names the read's `as_of` date — the same headline
    // row every other stop puts its subject or label on — and it sits between
    // the silence and the future commitment, exactly where the tone order
    // above puts it.
    const titles = [...document.querySelectorAll(".co-spine-title")].map(
      (node) => node.textContent,
    );
    expect(titles).toEqual([
      "You last spoke",
      "They have never written back",
      TODAY,
      "Send the proposal",
    ]);
    const marker = document.querySelector(".co-spine-now");
    expect(marker?.querySelector(".co-spine-title")?.textContent).toBe(TODAY);
  });
});

// A subject line says what a conversation was about; it says nothing about
// who it was with, and that is the half of "what happened" only a name
// resolves.
describe("who a conversation was with", () => {
  it("names the person a conversation's links resolve to", () => {
    draw(
      view({
        last_outbound_at: SPOKE,
        activities: activities([
          {
            id: "n-1",
            kind: "email",
            subject: "Renewal terms",
            at: SPOKE,
            links: [{ entity_type: "person", entity_id: "p-dana" }],
          },
        ]),
      }),
      (entityType, entityId) =>
        entityType === "person" && entityId === "p-dana"
          ? "Dana Otieno"
          : undefined,
    );

    expect(screen.getByText(/Dana Otieno/)).toBeTruthy();
  });

  it("drops a person link the resolver cannot name, rather than printing its id", () => {
    draw(
      view({
        last_outbound_at: SPOKE,
        activities: activities([
          {
            id: "n-2",
            kind: "email",
            subject: "Renewal terms",
            at: SPOKE,
            links: [{ entity_type: "person", entity_id: "p-unknown" }],
          },
        ]),
      }),
      () => undefined,
    );

    // No name resolved, so the detail line falls back to the kind alone —
    // never the raw id, which a reader cannot recognise.
    expect(screen.queryByText(/p-unknown/)).toBeNull();
    expect(screen.getByText("Renewal terms")).toBeTruthy();
  });

  it("names the first two people on a conversation and counts the rest", () => {
    draw(
      view({
        last_outbound_at: SPOKE,
        activities: activities([
          {
            id: "n-3",
            kind: "meeting",
            subject: "Quarterly review",
            at: SPOKE,
            links: [
              { entity_type: "person", entity_id: "p-1" },
              { entity_type: "person", entity_id: "p-2" },
              { entity_type: "person", entity_id: "p-3" },
              { entity_type: "person", entity_id: "p-4" },
            ],
          },
        ]),
      }),
      (_entityType, entityId) =>
        ({
          "p-1": "Ada Lin",
          "p-2": "Bo Chen",
          "p-3": "Cy Reyes",
          "p-4": "Di Fisher",
        })[entityId],
    );

    // Copy taken from co.spine.andOthers: "{names} and {count} others".
    expect(screen.getByText(/Ada Lin, Bo Chen and 2 others/)).toBeTruthy();
    expect(screen.queryByText(/Cy Reyes/)).toBeNull();
  });

  it("names who the folded-away earlier conversations were with", () => {
    draw(
      view({
        last_outbound_at: SPOKE,
        activities: activities([
          { id: "e-5", kind: "email", subject: "Fifth", at: SPOKE },
          {
            id: "e-4",
            kind: "email",
            subject: "Fourth",
            at: "2026-08-10T09:00:00Z",
          },
          {
            id: "e-3",
            kind: "email",
            subject: "Third",
            at: "2026-08-05T09:00:00Z",
          },
          {
            id: "e-2",
            kind: "email",
            subject: "Second",
            at: "2026-07-20T09:00:00Z",
            links: [{ entity_type: "person", entity_id: "p-early" }],
          },
          {
            id: "e-1",
            kind: "email",
            subject: "First",
            at: "2026-07-01T09:00:00Z",
            links: [{ entity_type: "person", entity_id: "p-early" }],
          },
        ]),
      }),
      (_entityType, entityId) =>
        entityId === "p-early" ? "Early Adopter" : undefined,
    );

    expect(screen.getByText("2 earlier conversations")).toBeTruthy();
    expect(screen.getByText("Early Adopter")).toBeTruthy();
  });
});

// A meeting is the one exchange where the record can name BOTH sides: the
// colleague who held it and who they held it with. Everything else on the
// thread names only the other party, because a mail carries no field for
// which mailbox sent it.
describe("who held a meeting", () => {
  it("names both sides when the host and the contact both resolve", () => {
    draw(
      view({
        last_outbound_at: SPOKE,
        activities: activities([
          {
            id: "h-1",
            kind: "meeting",
            subject: "Quarterly review",
            at: SPOKE,
            host_user_id: "u-lena",
            links: [{ entity_type: "person", entity_id: "p-dana" }],
          },
        ]),
      }),
      resolver({
        "user:u-lena": "Lena Fischer",
        "person:p-dana": "Dana Otieno",
      }),
    );

    expect(screen.getByText("Lena Fischer met Dana Otieno")).toBeTruthy();
  });

  it("says who held it when nobody on the other side resolves to a name", () => {
    draw(
      view({
        last_outbound_at: SPOKE,
        activities: activities([
          {
            id: "h-2",
            kind: "meeting",
            subject: "Quarterly review",
            at: SPOKE,
            host_user_id: "u-lena",
          },
        ]),
      }),
      (entityType, entityId) =>
        entityType === "user" && entityId === "u-lena"
          ? "Lena Fischer"
          : undefined,
    );

    expect(screen.getByText("Meeting held by Lena Fischer")).toBeTruthy();
  });

  it("falls back to the plain meeting line when the host id resolves to nothing", () => {
    // A host id nobody can put a name to is dropped rather than printed, the
    // same rule the contact side already keeps: a reader cannot recognise a
    // uuid, and printing one has spent the line saying nothing.
    draw(
      view({
        last_outbound_at: SPOKE,
        activities: activities([
          {
            id: "h-3",
            kind: "meeting",
            subject: "Quarterly review",
            at: SPOKE,
            host_user_id: "u-unknown",
            links: [{ entity_type: "person", entity_id: "p-dana" }],
          },
        ]),
      }),
      (entityType, entityId) =>
        entityType === "person" && entityId === "p-dana"
          ? "Dana Otieno"
          : undefined,
    );

    expect(screen.getByText("Meeting with Dana Otieno")).toBeTruthy();
    expect(screen.queryByText(/u-unknown/)).toBeNull();
    expect(screen.queryByText(/met Dana Otieno/)).toBeNull();
  });

  // `host_user_id` is a column on every activity row, not only a meeting's,
  // and an email that carries one was still sent rather than held: it reads
  // by direction like every other mail, never as something somebody "met".
  it("reads an email by direction even when it carries a host", () => {
    draw(
      view({
        last_outbound_at: SPOKE,
        activities: activities([
          {
            id: "e-host",
            kind: "email",
            subject: "Renewal terms",
            at: SPOKE,
            host_user_id: "u-lena",
            direction: "outbound",
            links: [{ entity_type: "person", entity_id: "p-dana" }],
          },
        ]),
      }),
      resolver({
        "user:u-lena": "Lena Fischer",
        "person:p-dana": "Dana Otieno",
      }),
    );

    expect(screen.getByText("Email to Dana Otieno")).toBeTruthy();
    expect(screen.queryByText(/met Dana Otieno/)).toBeNull();
    expect(screen.queryByText(/Lena Fischer/)).toBeNull();
  });
});

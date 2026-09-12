/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { RecordZoneProvider } from "../app/recordzone";
import { formatDateTime } from "../format/format";
import { viewerZone } from "../format/timezone";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import {
  boundedDecisions,
  boundedLeads,
  boundedMeetings,
  decisionRow,
  leadRow,
  meetingRow,
  readingsDay,
  wholeDecisions,
  wholeLeads,
  wholeMeetings,
} from "./brief.fixtures";
import { BriefReadingsStrip } from "./brief.readings";
import { WORKLIST_FILTER_PARAM } from "./worklist";

// The Brief's readings strip, and the one claim it makes that the Worklist's
// own strip does not: the row is FIVE slots on every morning, quiet or busy, so
// yesterday's five is never compared against today's four.
//
// Every one of the five now answers something. Two of them used to draw an em
// dash forever — promises, because the commitments lane is unwired, and quota
// pace, because targets were retired from the product — and the interesting
// cases are what replaced them: a pipeline figure that must never read as a
// target, and must say whose pipeline it measured.

// drawInZone renders the strip under a named record zone, which is what a
// date-only wire value must be read in.
function drawInZone(zone: string, ...args: Parameters<typeof readingsDay>) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <RecordZoneProvider zone={zone}>
        <LocaleProvider initial="en">
          <BriefReadingsStrip day={readingsDay(...args)} />
        </LocaleProvider>
      </RecordZoneProvider>
    </QueryClientProvider>,
  );
}

// The strip over a day built by hand, for the facts `readingsDay`'s parameters
// do not reach — `sources_unavailable` among them.
function drawDay(day: ReturnType<typeof readingsDay>) {
  // A QueryClient, because the pipeline reading is a read of its own: it is the
  // one figure on this plate that does not come from the worklist answer, and
  // it asks the same key Analytics asks.
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <BriefReadingsStrip day={day} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

function draw(...args: Parameters<typeof readingsDay>) {
  return drawDay(readingsDay(...args));
}

// Five READINGS, not five DOM children: a strip that wrapped its slots in a
// container would satisfy a child count while drawing one card, and a strip of
// five empty boxes would satisfy it while drawing none.
function labels(): string[] {
  return [
    ...screen.getByTestId("brief-readings").querySelectorAll(".stat-card"),
  ]
    .map((card) => card.querySelector(".stat-card-label")?.textContent ?? "")
    .filter((text) => text !== "");
}

/** Every figure the strip drew, as text. */
function strippedFigures(): string[] {
  return [
    ...screen
      .getByTestId("brief-readings")
      .querySelectorAll(".stat-card-value"),
  ].map((value) => value.textContent ?? "");
}

/** The ones wearing the floor mark. */
function markedFigures(): string[] {
  return strippedFigures().filter((text) => text.endsWith("+"));
}

function meetingsCard(): HTMLElement {
  const card = screen
    .getByText(en["brief.readings.meetings"])
    .closest(".stat-card");
  if (!(card instanceof HTMLElement)) {
    throw new Error("the meetings reading is not on the page");
  }
  return card;
}

/**
 * The two reads the pipeline card makes: the scope the server names for this
 * reader, and the forecast under it.
 *
 * Both, because the card holds its read until the scope arrives — a stub that
 * answered only the forecast would leave the query disabled and the test would
 * assert against a pending card forever.
 */
function stubPipeline(forecast: () => Response) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input instanceof Request ? input.url : input);
      if (url.includes("/analytics/context")) {
        return new Response(
          JSON.stringify({
            default_scope: { kind: "owner", id: "u-1", label: "Lena" },
            scopes: [],
          }),
          { status: 200, headers: { "content-type": "application/json" } },
        );
      }
      return forecast();
    }),
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the brief readings strip", () => {
  it("draws five answerable slots on a full morning", () => {
    draw();

    expect(labels()).toHaveLength(5);
    expect(screen.getByText(en["brief.readings.urgent"])).toBeTruthy();
    expect(screen.getByText(en["brief.readings.meetings"])).toBeTruthy();
    expect(screen.getByText(en["brief.readings.leads"])).toBeTruthy();
    expect(screen.getByText(en["brief.readings.pipelinePlain"])).toBeTruthy();
    expect(screen.getByText(en["brief.readings.decisions"])).toBeTruthy();
  });

  // The property that can actually regress: on a fully answered morning, no slot
  // draws an em dash. The strings of the retired slots are gone from all three
  // catalogs, so asserting THOSE are absent is an assertion nothing in the tree
  // can fail — a test that cannot go red is not a guard.
  it("draws no unanswered slot on a morning every read landed on", async () => {
    stubPipeline(
      () =>
        new Response(
          JSON.stringify({
            period_start: "2026-07-01",
            period_end: "2026-09-30",
            scope_kind: "owner",
            open_minor: 42_000_000,
            weighted_minor: 16_800_000,
            best_case_minor: 0,
            evidence_minor: 0,
            eligible_count: 12,
            priced_count: 12,
            confirmed_date_count: 8,
            fx_missing_count: 0,
            as_of: "2026-09-03T06:42:00Z",
            base_currency: "EUR",
          }),
          { status: 200, headers: { "content-type": "application/json" } },
        ),
    );
    draw();

    await screen.findByText(/420k|420,000/i);
    expect(screen.queryAllByText("—")).toHaveLength(0);
  });

  it("still draws five slots when nothing is waiting", () => {
    draw({ buyer_replies: 0, prospecting: 0 }, []);

    expect(labels()).toHaveLength(5);
    // A zero already reads as "none". What the line under it adds is the basis
    // — what the figure was taken over — which is the same on a quiet day as on
    // a busy one, so the row keeps its shape as well as its slot count.
    expect(screen.getByText(en["brief.readings.urgentBasis"])).toBeTruthy();
    expect(screen.getByText(en["brief.readings.leadsBasis"])).toBeTruthy();
    expect(screen.getByText(en["brief.readings.meetingsBasis"])).toBeTruthy();
  });

  it("counts the meetings nothing is prepared for", () => {
    // Three meetings, two of them unprepared. The count of meetings comes from
    // `counts` and the readiness figure from the ROWS, so a fixture where they
    // agree is the one that proves both are read.
    draw({ buyer_replies: 0 }, [
      meetingRow("m1", false),
      meetingRow("m2", false),
      meetingRow("m3", true),
    ]);

    expect(meetingsCard().textContent).toContain("3");
    expect(meetingsCard().textContent).toContain(
      en["brief.readings.needsPrep_other"].replace("{count}", "2"),
    );
  });

  // The absence of a warning has to read as an answer rather than as a gap: a
  // blank line under the meetings figure looks exactly like a line that failed
  // to render.
  it("says so when every meeting is prepared", () => {
    draw({}, [meetingRow("m1", true), meetingRow("m2", true)]);

    expect(screen.getByText(en["brief.readings.prepared"])).toBeTruthy();
    expect(screen.queryByText(/needs prep/)).toBeNull();
  });

  // The defect this slot is shaped around. `considered` counts every meeting
  // read and ranked; readiness can only be counted off the rows the page
  // carries. Ten considered, three on the page, two of those unprepared would
  // read "10 · 2 need prep" — telling a rep eight meetings are ready when
  // nothing checked them.
  it("refuses a readiness figure when the page carries fewer than it counted", () => {
    draw({}, [meetingRow("m1", false)], [boundedMeetings(10, 1)]);

    expect(meetingsCard().textContent).toContain("10");
    expect(screen.getByText(en["brief.readings.prepUnknown"])).toBeTruthy();
    // Not "1 needs prep", and not "all prepared": both are claims about ten
    // meetings made by looking at one.
    expect(screen.queryByText(/needs prep/)).toBeNull();
    expect(screen.queryByText(en["brief.readings.prepared"])).toBeNull();
  });

  // A day whose counts carry no meetings entry at all. That is not an
  // unanswerable question — it is zero meetings, read whole — and saying "not
  // all could be checked" tells a rep the page failed at something it finished.
  it("reads a missing meetings count as a day with none, not an unknown", () => {
    draw({}, [], []);

    expect(meetingsCard().textContent).toContain("0");
    expect(screen.getByText(en["brief.readings.meetingsBasis"])).toBeTruthy();
    expect(screen.queryByText(en["brief.readings.prepUnknown"])).toBeNull();
  });

  // Two questions with no source. Drawing a zero would be a false answer, and
  // dropping the slot would be the page losing a question without saying so.
  // A read that did not land is not a pipeline of nothing. The em dash says the
  // question went unanswered, which is the one case that spelling is still true.
  //
  // The read is REFUSED here rather than left unstubbed: an unstubbed query
  // stays pending forever, and a test asserting the failure copy over a pending
  // card would pass on a card that never resolves either way.
  it("says the pipeline went unread rather than drawing a nought", async () => {
    stubPipeline(
      () =>
        new Response(JSON.stringify({ title: "Server Error" }), {
          status: 500,
          headers: { "content-type": "application/problem+json" },
        }),
    );
    draw();

    expect(
      await screen.findByText(en["brief.readings.pipelineUnread"]),
    ).toBeTruthy();
    // The slot says it in WORDS. Every figure on this plate is read across as
    // one statement, and an em dash in one of them reads as a card that failed
    // to draw rather than as a reading nobody could take — so no slot on this
    // strip may fall back to the glyph.
    expect(
      await screen.findByText(en["brief.readings.pipelineNoRead"]),
    ).toBeTruthy();
    expect(screen.queryAllByText("—")).toHaveLength(0);
  });

  // BOTH figures, and neither of them a target. `open` is the face value of
  // every open deal and `weighted` applies each deal's own probability; one
  // without the other invites a reader to treat a face value as a forecast.
  //
  // The slot this replaced said "Quota pace — no target is set". There is no
  // authoritative target in this product to compare against, so the card must
  // never say on track, attainment or gap — it would be inventing the thing
  // that was removed.
  it("draws the open pipeline with its weighted figure beside it", async () => {
    stubPipeline(
      () =>
        new Response(
          JSON.stringify({
            period_start: "2026-07-01",
            period_end: "2026-09-30",
            scope_kind: "owner",
            open_minor: 42_000_000,
            weighted_minor: 16_800_000,
            best_case_minor: 0,
            evidence_minor: 0,
            eligible_count: 12,
            priced_count: 11,
            confirmed_date_count: 8,
            fx_missing_count: 0,
            as_of: "2026-09-03T06:42:00Z",
            base_currency: "EUR",
          }),
          { status: 200, headers: { "content-type": "application/json" } },
        ),
    );
    draw();

    // COMPACT for both figures — the slot is ~110px and a full euro amount
    // wraps mid-number. The basis line has no more room than the headline does.
    expect(await screen.findByText(/420k/i)).toBeTruthy();
    // The weighted figure and how much of the population carries a price ride
    // the same line: a weighted number over a partly priced population is a
    // floor, and a reader who cannot see the second cannot judge the first.
    expect(screen.getByText(/168k weighted · 11 priced/i)).toBeTruthy();
    // THE WINDOW the money covers, in the title. €420k of open pipeline means
    // nothing without it: the same page carries a by-currency total of
    // everything open, and a reader with no period cannot tell why the two
    // disagree. A quarter is all a dense title has room for, so the full range
    // is on the cell's hover line and asserted where that is.
    expect(
      screen.getByText(
        en["brief.readings.pipeline"].replace("{quarter}", "Q3"),
      ),
    ).toBeTruthy();
    // Never a target word. The quota table was dropped by founder decision.
    const strip = screen.getByTestId("brief-readings");
    expect(strip.textContent).not.toMatch(/on track|target|attainment|gap/i);
  });

  // THE one way this card can be wrong without looking wrong. The scope is left
  // unnamed so the server resolves it against the caller's lens — a rep gets
  // their own pipeline. If that resolution ever lands on the workspace instead,
  // the figure is the whole installation's under a heading that says "your
  // morning", and nothing about the number itself would give it away.
  //
  // So the card reads the answer's own `scope_kind` back and says so.
  it("says when the pipeline it drew is the whole workspace, not the reader's", async () => {
    stubPipeline(
      () =>
        new Response(
          JSON.stringify({
            period_start: "2026-07-01",
            period_end: "2026-09-30",
            scope_kind: "workspace",
            open_minor: 42_000_000,
            weighted_minor: 16_800_000,
            best_case_minor: 0,
            evidence_minor: 0,
            eligible_count: 12,
            priced_count: 12,
            confirmed_date_count: 8,
            fx_missing_count: 0,
            as_of: "2026-09-03T06:42:00Z",
            base_currency: "EUR",
          }),
          { status: 200, headers: { "content-type": "application/json" } },
        ),
    );
    draw();

    // The title has room for a quarter and nothing else, so the scope rides the
    // cell's hover line. Reached through the card's own DOOR, because that is
    // the one focusable thing on the cell and a focus event bubbles up to the
    // wrapper the tip hangs off — a pointer tip waits on hover intent, and a
    // reader who tabbed here has said what they want outright.
    const door = await screen.findByRole("button", {
      name: en["brief.readings.openPipeline"],
      description: en["brief.readings.pipeline"].replace("{quarter}", "Q3"),
    });
    door.focus();
    const tip = await screen.findByRole("tooltip");
    expect(tip.textContent).toContain(
      en["brief.readings.pipelineTipWorkspace"].replace("{period} · ", ""),
    );
    expect(tip.textContent).toContain("1 Jul 2026 – 30 Sept 2026");
  });

  // The first card a rep reads, at a REAL figure.
  //
  // It moved from one lane's count (`readings.buyer_replies`) to the summary's
  // own `urgent`, which is every row at the top two levels — somebody waiting or
  // a promise breaking. Every other test on this plate runs against a fixture
  // pinning `urgent: 0`, so the change was only ever exercised at zero and any
  // other zero-valued field would have satisfied them.
  it("counts every urgent row, not one lane's share of them", () => {
    // Four urgent, while the lane counts beside them say something else: a card
    // still reading buyer_replies would draw 3.
    draw({ buyer_replies: 3 }, undefined, undefined, { urgent: 4 });

    const card = screen
      .getByText(en["brief.readings.urgent"])
      .closest(".stat-card");
    if (!(card instanceof HTMLElement)) {
      throw new Error("the urgent reading is not on the page");
    }
    expect(card.textContent).toContain("4");
    // And it WARNS, because somebody is waiting. A figure drawn plain would say
    // the morning is calm while four rows say it is not. The tone lands on the
    // value rather than the card, which is where StatCard puts it.
    expect(card.querySelector(".stat-card-warn")).toBeTruthy();
  });

  // A BOUNDED READ IS A `+`, NOT A SENTENCE. The row used to carry a line
  // saying a source had been read to its limit, so every figure above was a
  // floor — a whole sentence of footnote under a plate whose argument is that
  // five readings are taken in at one glance. The fact belongs ON the figures.
  //
  // A figure wears the mark when ITS OWN categories were cut short, not when
  // some other lane was. A lane nobody could read at all is the exception and
  // is covered further down: it marks the strip, because the source-to-category
  // mapping that would narrow it lives on the server.
  it("marks a figure whose own category was read to its limit", () => {
    draw(
      { more_available: true, prospecting: 2, review: 8 },
      [],
      [boundedDecisions(8, 8), wholeLeads(2), wholeMeetings(3)],
      { urgent: 4 },
    );

    // Decisions, plus the urgent slot — which spans every lane, so any bound
    // anywhere makes it a floor. Leads and meetings were read whole and say so.
    expect(markedFigures()).toEqual(expect.arrayContaining(["8+", "4+"]));
    expect(markedFigures()).toHaveLength(2);
  });

  // THE CASE THIS EXISTS FOR. A calendar read whole used to show "3+" because
  // an unrelated lane stopped at its bound, and a `+` on the figures that are
  // exact is one a reader learns to discount.
  it("leaves a whole-read figure exact when another lane was bounded", () => {
    draw(
      { more_available: true, prospecting: 2, review: 8 },
      [],
      [boundedDecisions(8, 8), wholeMeetings(3)],
      { urgent: 0 },
    );

    expect(strippedFigures()).toContain("3");
    expect(markedFigures()).not.toContain("3+");
  });

  // A lane that never ANSWERED is not a lane read to a bound, and the strip
  // cannot tell which reading it would have fed: `sources_unavailable` names a
  // source, and only the server maps a source to its lane. So it marks the
  // whole strip rather than guessing, which over-marks instead of calling a
  // figure exact over work nobody could see.
  it("marks every figure when a source could not be read at all", () => {
    const day = readingsDay(
      { more_available: true, prospecting: 2, review: 8 },
      [],
      [wholeMeetings(3)],
      { urgent: 4 },
    );
    day.sources_unavailable = [{ source: "calendar", reason: "failed" }];
    drawDay(day);

    // All three worklist slots, LEADS INCLUDED — a fallback wired to two of
    // them would pass a test that only asked about two.
    expect(markedFigures()).toContain("3+");
    expect(markedFigures()).toContain("2+");
    expect(markedFigures()).toContain("8+");
  });

  // Each lane keeps its own mark, asked one lane at a time. The tests above set
  // the decisions lane bounded, so a `floorOf` wired to that lane alone would
  // satisfy them while leaving leads and meetings permanently exact — the
  // under-marking direction, and the one that fails silently.
  it.each([
    {
      lane: "leads",
      counts: [boundedLeads(2, 2), wholeMeetings(3), wholeDecisions(8)],
      marked: "2+",
    },
    {
      lane: "meetings",
      counts: [wholeLeads(2), boundedMeetings(3, 3), wholeDecisions(8)],
      marked: "3+",
    },
  ])("marks the $lane figure when only that lane was bounded", (each) => {
    draw({ more_available: true, prospecting: 2, review: 8 }, [], each.counts, {
      urgent: 0,
    });

    expect(markedFigures()).toEqual([each.marked]);
  });

  // A FLOOR OF NONE IS NOT A FLOOR. "0+" says "at least nothing", which is true
  // of every number there has ever been — so a bounded read that found none of
  // a kind draws a plain zero, and the mark stays on the figures that count
  // something. It shipped as `0+` on a day with no meetings.
  it("leaves a zero unmarked even where the read was bounded", () => {
    // Both lanes bounded, so the zero and the eight are asked the same
    // question and only the difference in their figures decides the mark.
    draw(
      { more_available: true, prospecting: 0, review: 8 },
      [],
      [boundedDecisions(8, 8), boundedLeads(0, 0)],
      {
        urgent: 0,
      },
    );

    const figures = strippedFigures();
    expect(figures).toContain("0");
    expect(
      figures.some((text) => text.startsWith("0") && text.endsWith("+")),
    ).toBe(false);
    // The positive control: the day's non-zero reading still wears it, or this
    // would pass over a build that stopped marking anything at all.
    expect(markedFigures()).toContain("8+");
  });

  // Why the `+` is there, on the cell rather than beside the three characters
  // that carry it — a mark that explains itself only to a pointer resting on a
  // figure is one most readers never read.
  it("says on the cell why a marked figure is a floor", async () => {
    // A non-zero urgent count, because a zero draws no mark and so owes no
    // explanation — the cell this asks would have nothing to say.
    draw({ more_available: true }, undefined, undefined, { urgent: 4 });

    // Through the reading's own door: it is the cell's only focusable element,
    // and a focus event bubbles to the wrapper carrying the tip.
    screen
      .getByRole("button", {
        name: en["brief.readings.openUrgent"],
        description: en["brief.readings.urgent"],
      })
      .focus();

    expect((await screen.findByRole("tooltip")).textContent).toBe(
      en["brief.readings.floorTip"],
    );
  });

  it("marks nothing on a day it read whole", () => {
    draw();

    expect(markedFigures()).toHaveLength(0);
  });
});

// The leads slot carries a second fact for the same reason the meetings slot
// does: how many are owed an answer does not tell a rep whether one is due
// before lunch.
describe("the leads reading", () => {
  function leadsCard(): HTMLElement {
    const card = screen
      .getByText(en["brief.readings.leads"])
      .closest(".stat-card");
    if (!(card instanceof HTMLElement)) {
      throw new Error("the leads reading is not on the page");
    }
    return card;
  }

  it("names when the nearest answer is due", () => {
    draw(
      { prospecting: 2 },
      [leadRow("l1", "2026-08-31T15:00:00Z"), leadRow("l2")],
      [wholeLeads(2)],
    );

    expect(leadsCard().textContent).toContain("2");
    // The formatted moment, not the ISO string: the slot renders it in the
    // reader's own zone and this file does not own that format.
    expect(leadsCard().textContent).not.toContain("2026-08-31T15:00:00Z");
    expect(screen.queryByText(en["brief.readings.leadsBasis"])).toBeNull();
  });

  it("names the earliest of several, not the first it meets", () => {
    draw(
      { prospecting: 2 },
      [
        leadRow("late", "2026-08-31T18:00:00Z"),
        leadRow("soon", "2026-08-31T09:00:00Z"),
      ],
      [wholeLeads(2)],
    );

    // Asserted by DIFFERENCE rather than by a literal clock reading: the two
    // moments are nine hours apart, so whichever zone the runtime is in, the
    // card must carry the earlier one's rendering and not the later one's. A
    // hard-coded hour would pin the test to the runner's own zone — the first
    // version of this line did, and read 09:00 as 16:00 under Asia/Ho_Chi_Minh.
    const earlier = formatDateTime("2026-08-31T09:00:00Z", "en", viewerZone());
    const later = formatDateTime("2026-08-31T18:00:00Z", "en", viewerZone());
    expect(leadsCard().textContent).toContain(earlier);
    expect(leadsCard().textContent).not.toContain(later);
  });

  // The defect this slot is shaped around, and the same one the meetings slot
  // guards: a cut read means an unshown lead could be due sooner than every one
  // the reader can see, so naming the earliest visible deadline states a "next"
  // that is not next.
  it("refuses a deadline when the page carries fewer leads than it counted", () => {
    draw(
      { prospecting: 9 },
      [leadRow("l1", "2026-08-31T15:00:00Z")],
      [boundedLeads(9, 1)],
    );

    expect(leadsCard().textContent).toContain("9");
    expect(screen.getByText(en["brief.readings.leadsBasis"])).toBeTruthy();
  });

  // Every lead already overdue. There is no NEXT moment to name — they have all
  // passed — so the slot says what it counts rather than a date behind it.
  it("names no deadline when every lead is already overdue", () => {
    draw({ prospecting: 2 }, [leadRow("l1"), leadRow("l2")], [wholeLeads(2)]);

    expect(leadsCard().textContent).toContain("2");
    expect(screen.getByText(en["brief.readings.leadsBasis"])).toBeTruthy();
  });
});

// A period is a CALENDAR window, not an instant, and it reads the same for
// every colleague.
//
// period_start and period_end are date-only wire values. Read in the viewer's
// own clock west of UTC they parse as UTC midnight and print the day before, so
// a rep in Los Angeles saw the third quarter labelled "30 Jun – 29 Sept" while
// a colleague in Berlin saw "1 Jul – 30 Sept" — two contacts quoting one page and
// quoting different quarters. The record's zone is the only answer that is the
// same for both.
describe("the pipeline period", () => {
  afterEach(cleanup);

  it("names the same days west of UTC as it does east of it", async () => {
    const payload = {
      period_start: "2026-07-01",
      period_end: "2026-09-30",
      scope_kind: "owner",
      open_minor: 42_000_000,
      weighted_minor: 16_800_000,
      best_case_minor: 0,
      evidence_minor: 0,
      eligible_count: 12,
      priced_count: 11,
      confirmed_date_count: 8,
      fx_missing_count: 0,
      as_of: "2026-09-03T06:42:00Z",
      base_currency: "EUR",
    };
    stubPipeline(
      () =>
        new Response(JSON.stringify(payload), {
          status: 200,
          headers: { "content-type": "application/json" },
        }),
    );

    // The VIEWER sits west of UTC while the installation's calendar does not.
    // Without this the suite's own machine decides whether the bug can appear
    // at all: east of UTC a viewer-zone read prints the right days by luck, and
    // the assertion passes over code that is wrong for half the world.
    const resolved = Intl.DateTimeFormat.prototype.resolvedOptions;
    vi.spyOn(
      Intl.DateTimeFormat.prototype,
      "resolvedOptions",
    ).mockImplementation(function (this: Intl.DateTimeFormat) {
      return { ...resolved.call(this), timeZone: "America/Los_Angeles" };
    });

    // The range is on the CELL's hover line — a dense title has room for a
    // quarter and nothing more — so it is reached by focus, which reveals a tip
    // outright where a pointer waits on hover intent.
    async function rangeSaid(zone: string): Promise<string> {
      drawInZone(zone);
      const door = await screen.findByRole("button", {
        name: en["brief.readings.openPipeline"],
        description: en["brief.readings.pipeline"].replace("{quarter}", "Q3"),
      });
      door.focus();
      return (await screen.findByRole("tooltip")).textContent ?? "";
    }

    expect(await rangeSaid("Europe/Berlin")).toContain(
      "1 Jul 2026 – 30 Sept 2026",
    );
    cleanup();

    expect(await rangeSaid("Asia/Tokyo")).toContain(
      "1 Jul 2026 – 30 Sept 2026",
    );
  });
  // EVERY DOOR NAMES ITS OWN ACTION. The strip's doors all read "Open" once,
  // which is one entry repeated in a screen reader's control list: the card
  // above each says open WHAT, and a reader tabbing the strip never sees it.
  //
  // Both halves are asserted, because each covers the other's blind spot. The
  // NAMES must be distinct — that is the defect. The DESCRIPTIONS must still
  // carry each reading's label — that is what the generic word relied on, and a
  // named door that dropped it would read as an action over no subject.
  //
  // Asserted as a SET: a per-card check passes on four doors named identically.
  it("names every reading's door for its own action", async () => {
    drawInZone("Europe/Berlin");

    await screen.findByText(en["brief.readings.urgent"]);
    // Four of the five lanes come from the worklist answer and each has a door.
    // The pipeline slot's read has not landed under this stub, and an unread
    // figure is offered no way out.
    const doors = [
      en["brief.readings.openUrgent"],
      en["brief.readings.openMeetings"],
      en["brief.readings.openLeads"],
      en["brief.readings.openDecisions"],
    ].map((name) => screen.getByRole("button", { name }));

    expect(new Set(doors).size).toBe(4);
    // Not one of them still says the generic word.
    expect(
      screen.queryAllByRole("button", { name: en["stat.open"] }),
    ).toHaveLength(0);

    const descriptions = doors.map((door) =>
      (door.getAttribute("aria-describedby") ?? "")
        .split(/\s+/)
        .map((id) => document.getElementById(id)?.textContent?.trim() ?? "")
        .join(" "),
    );
    expect(descriptions.every((text) => text.length > 0)).toBe(true);
    expect(new Set(descriptions).size).toBe(4);
  });

  // NOBODY IS BLOCKED BY A DUPLICATE PAIR. The strip said "somebody is blocked
  // until you answer" under every pending decision, and most of them are
  // contact hygiene the ranker deliberately puts BELOW a waiting customer. A
  // line that claims a person is held up by a merge suggestion is the one a
  // reader learns to discount.
  it("claims nobody is blocked when no decision holds work up", () => {
    draw(
      { review: 2 },
      [decisionRow("a1", false), decisionRow("a2", false)],
      [wholeDecisions(2)],
    );

    expect(screen.getByText(en["brief.readings.decisionsBasis"])).toBeTruthy();
    expect(screen.queryByText(/holding up customer work/)).toBeNull();
  });

  // And says so, with a count, where the server's own consequence says it is
  // true. Counted off `work_blocked` rather than off the category, so the strip
  // and the row a reader opens cannot disagree.
  it("counts only the decisions that hold customer work up", () => {
    draw(
      { review: 3 },
      [
        decisionRow("a1", true),
        decisionRow("a2", false),
        decisionRow("a3", false),
      ],
      [wholeDecisions(3)],
    );

    expect(
      screen.getByText(
        en["brief.readings.decisionsBlocking_one"].replace("{count}", "1"),
      ),
    ).toBeTruthy();
    expect(screen.queryByText(en["brief.readings.decisionsBasis"])).toBeNull();
  });

  // The COUNT, not just its presence. One blocker proves nothing about
  // counting: an implementation returning 1 whenever any blocker exists passes
  // a single-blocker test, and prints "1 holding up customer work" over three.
  it("says how many decisions hold customer work up", () => {
    draw(
      { review: 4 },
      [
        decisionRow("a1", true),
        decisionRow("a2", true),
        decisionRow("a3", true),
        decisionRow("a4", false),
      ],
      [wholeDecisions(4)],
    );

    expect(
      screen.getByText(
        en["brief.readings.decisionsBlocking_other"].replace("{count}", "3"),
      ),
    ).toBeTruthy();
  });

  // AND NOT AT ALL WHERE THE PAGE IS NOT THE WHOLE LANE. The figure beside it
  // is counted over everything the read weighed; this one can only see the
  // rows the page carries. On a day whose decisions run past page one, pairing
  // them understates a block — so the claim is withheld rather than guessed.
  it("claims no blocking count when the page is not the whole lane", () => {
    draw(
      { review: 30 },
      [decisionRow("a1", true)],
      [
        {
          category: "decisions",
          considered: 30,
          shown: 1,
          more_available: false,
        },
      ],
    );

    expect(screen.getByText(en["brief.readings.decisionsBasis"])).toBeTruthy();
    expect(screen.queryByText(/holding up customer work/)).toBeNull();
  });

  // A NAMED DOOR HAS TO GO WHERE ITS NAME SAYS. Distinct names and distinct
  // destinations are two properties, and the test above only holds the first:
  // it never presses anything, so swapping two labels — or wiring the meetings
  // door to the leads lane — passes it. This one presses each door and reads
  // where it landed.
  it.each([
    { door: "openUrgent", filter: "all" },
    { door: "openMeetings", filter: "meetings" },
    { door: "openLeads", filter: "leads" },
    { door: "openDecisions", filter: "decisions" },
  ] as const)("opens the $filter lane from its own door", async (each) => {
    draw();
    const user = userEvent.setup();

    try {
      await user.click(
        screen.getByRole("button", {
          name: en[`brief.readings.${each.door}`],
        }),
      );

      expect(window.location.hash).toContain("/worklist");
      expect(window.location.hash).toContain(
        `${WORKLIST_FILTER_PARAM}=${each.filter}`,
      );
    } finally {
      window.location.hash = "";
    }
  });

  // The fifth reading's door, and the two states it must tell apart. The
  // pipeline figure is the one slot read separately, so it is the one slot that
  // can be pending or unread — and a door onto a figure nobody could read sends
  // a reader to a section to check a number this page never had.
  it("opens the forecast from the pipeline reading once it has landed", async () => {
    stubPipeline(
      () =>
        new Response(
          JSON.stringify({
            period_start: "2026-07-01",
            period_end: "2026-09-30",
            scope_kind: "owner",
            open_minor: 42_000_000,
            weighted_minor: 16_800_000,
            best_case_minor: 0,
            evidence_minor: 0,
            eligible_count: 12,
            priced_count: 12,
            confirmed_date_count: 8,
            fx_missing_count: 0,
            as_of: "2026-09-03T06:42:00Z",
            base_currency: "EUR",
          }),
          { status: 200, headers: { "content-type": "application/json" } },
        ),
    );
    draw();
    await screen.findByText(/420k|420,000/i);

    const door = screen.getByRole("button", {
      name: en["brief.readings.openPipeline"],
      description: en["brief.readings.pipeline"].replace("{quarter}", "Q3"),
    });
    try {
      await userEvent.setup().click(door);
      expect(window.location.hash).toBe("#/analytics/forecast");
    } finally {
      window.location.hash = "";
    }
  });

  // The same slot with nothing behind it. A read that did not land is not a
  // pipeline of nothing, and it is not a door either.
  it("offers no door while the pipeline read has not landed", async () => {
    stubPipeline(() => new Response("", { status: 500 }));
    draw();

    await screen.findByText(en["brief.readings.pipelineNoRead"]);
    expect(
      screen.queryByRole("button", {
        name: en["brief.readings.openPipeline"],
        description: en["brief.readings.pipelinePlain"],
      }),
    ).toBeNull();
  });
});

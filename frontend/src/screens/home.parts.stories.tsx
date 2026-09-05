// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { BriefFeed } from "./brief.feed";
import { PlanSection } from "./brief.plan";
import { deckItems } from "./home";
import { DecisionsSection } from "./home.decisions";
import {
  bundle,
  deals,
  digest,
  leadRow,
  meetingRow,
  NOT_FOUND,
  overnightRow,
  pipelineRows,
  readingsDay,
  report,
  singles,
  type Worklist,
} from "./home.fixtures";
import { HomeGlance } from "./home.glance";
import { OvernightPanel, PositionPanel, WatchPanel } from "./home.rail";
import { HomeReadingsStrip } from "./home.readings";
import { PromisesPanel, SchedulePanel } from "./home.schedule";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

// Home, one part at a time.
//
// `home.stories.tsx` documents the whole morning; this file documents the pieces
// it is assembled from, because each of them has states the assembled page can
// only show one of at a time — a briefing whose readings have not all answered,
// a rail panel whose connector is unhealthy, a ranked queue with no run behind
// it. Same fixtures as the page (`home.fixtures.ts`), so a part cannot drift
// from the page it is part of.
//
// Read every frame in BOTH themes with the toolbar's Theme control. Nothing here
// is theme-aware in its own right, which is exactly why it needs looking at:
// every colour is a `color-mix()` of a canonical token, so a surface can be
// correct in light and wrong in dark.
//
// The clock is a fixed instant everywhere it is read. The greeting band would
// otherwise say something different every time somebody opened the catalog.

const NOW = Date.parse("2026-08-21T07:30:00Z");
const NOW_DATE = new Date(NOW);

/** The routes the rail's two reading panels fan out to. */
const RAIL_ROUTES: RouteMap = {
  "GET /me": meRoute({}),
  "GET /digest": () => jsonResponse(digest),
  "GET /projects/01a00000-0000-7000-8000-000000000001": () =>
    jsonResponse({
      id: "01a00000-0000-7000-8000-000000000001",
      name: "ERP replacement",
    }),
  "GET /projects/01a00000-0000-7000-8000-000000000002": () =>
    jsonResponse({
      id: "01a00000-0000-7000-8000-000000000002",
      name: "Depot rollout",
    }),
  "POST /reports/deals-by-stage": () => report(pipelineRows),
  "GET /organizations/org-nordwind": () =>
    jsonResponse({ id: "org-nordwind", display_name: "Nordwind Logistik" }),
  "GET /organizations/org-acme": () =>
    jsonResponse({ id: "org-acme", display_name: "Acme Fördertechnik" }),
};

/** One part, with the reads it makes answered and nothing else reachable. */
function part(node: React.ReactNode, routes: RouteMap = RAIL_ROUTES) {
  return () => {
    installFetchStub(routes);
    return <StoryProviders>{node}</StoryProviders>;
  };
}

const meta: Meta = {
  title: "Shell/Home parts",
};
export default meta;
type Story = StoryObj;

// ── The briefing ────────────────────────────────────────────────────────────

// A day with work in it, for the opening sentence. The sentence names the FIRST
// row through the same helpers the queue prints it with, so a story that passed
// no queue would show the block without the thing it exists to say.
const GLANCE_DAY = {
  as_of: "2026-06-10T06:00:00Z",
  scope: "mine",
  scope_options: ["mine"],
  queue: [
    {
      id: "g1",
      source: "waiting_customer",
      category: "customer_waiting",
      title: "Aster Handel",
      because: [],
      actions: ["open"],
      dispositions: [],
      overdue: false,
    },
    {
      id: "g2",
      source: "task",
      category: "housekeeping",
      title: "Send the Weber quote",
      because: [],
      actions: ["open"],
      dispositions: [],
      overdue: false,
    },
  ],
  counts: [],
  reach: [],
  sources_unavailable: [],
  summary: { total: 2, urgent: 1 },
} as unknown as Parameters<typeof HomeGlance>[0]["day"];

// A week that closed with a result and a debt — the two things the weekly's
// opening sentence is built from.
const GLANCE_WEEK = {
  local_week_start: "2026-06-29",
  generated_at: "2026-07-06T03:00:00Z",
  counts: {
    tasks_due: 6,
    tasks_done: 4,
    tasks_carried_over: 2,
    deals_moved: 3,
    deals_won: 2,
    deals_lost: 0,
    proposals_accepted: 1,
    proposals_rejected: 0,
    brief_items_acted: 5,
    brief_items_dismissed: 1,
    commitments_due: 4,
    commitments_kept: 2,
    leads_routed: 7,
    leads_answered_in_target: 6,
    leads_breached: 1,
    meetings_held: 5,
    meetings_with_next_step: 4,
  },
  pipeline: {
    won_minor: 4200000,
    created_minor: 0,
    lost_minor: 0,
    currency: "EUR",
  },
} as unknown as Parameters<typeof HomeGlance>[0]["week"];

// The header the Brief opens with: eyebrow, greeting, and ONE composed sentence
// about the day — not a column of counts. Each fact the old briefing lines
// stated has a better home on the page now: the decisions deck draws its own
// cards, the readings strip carries the figures, and the rail's Overnight and
// Watch panels list what the night found.
export const Glance: Story = {
  render: part(
    <HomeGlance
      view="morning"
      day={GLANCE_DAY}
      week={null}
      firstName="Lena"
      now={NOW_DATE}
    />,
  ),
};

// The name has not arrived yet. The greeting is drawn anyway, because the hour
// is known either way and a header that waited for `/me` would move under the
// reader a moment after they started reading it.
export const GlanceUnnamed: Story = {
  render: part(
    <HomeGlance
      view="morning"
      day={GLANCE_DAY}
      week={null}
      firstName={null}
      now={NOW_DATE}
    />,
  ),
};

// The weekly says its own thing under its own heading, composed from the counts
// the week was frozen with rather than from the ranked queue — which describes
// THIS morning and would read as the wrong week entirely.
export const GlanceWeekly: Story = {
  render: part(
    <HomeGlance
      view="weekly"
      day={GLANCE_DAY}
      week={GLANCE_WEEK}
      firstName="Lena"
      now={NOW_DATE}
    />,
  ),
};

// A week nobody has written yet. The heading names the view and says nothing
// about it, because there is nothing yet to say — never a quiet-week claim,
// which would tell a rep their week was calm on no evidence.
export const GlanceWeeklyUnread: Story = {
  render: part(
    <HomeGlance
      view="weekly"
      day={GLANCE_DAY}
      week={undefined}
      firstName="Lena"
      now={NOW_DATE}
    />,
  ),
};

// ── The readings strip ──────────────────────────────────────────────────────

// Five slots, always, and deliberately no money: the pipeline is worth three
// currencies at once and no honest single figure for it exists. The per-currency
// figures are the rail's Position panel, below.
export const Readings: Story = {
  render: part(<HomeReadingsStrip day={readingsDay()} />),
};

// A source ended short of its list, so every figure is a floor. The caveat sits
// under the whole strip rather than on one slot: a caveat on one figure invites
// the reading where the other four are exact.
export const ReadingsCapped: Story = {
  render: part(
    <HomeReadingsStrip
      day={readingsDay({ buyer_replies: 100, more_available: true })}
    />,
  ),
};

// A quiet morning. The two untracked slots read the same as ever — the strip
// does not get shorter on a day with less in it, because a reader comparing it
// with yesterday's would take the missing slot for an answered question.
export const ReadingsQuiet: Story = {
  render: part(
    <HomeReadingsStrip
      day={readingsDay({ buyer_replies: 0, prospecting: 0 }, [])}
    />,
  ),
};

// ── The work column ─────────────────────────────────────────────────────────

// The deck with its own heading on the toggle's row, which is what the section
// adds to `DecisionDeck`: the title, the four chips a card carries, and the one
// act that sends the tray.
export const Decisions: Story = {
  render: part(
    <DecisionsSection
      items={deckItems([...singles, ...bundle])}
      nowMs={NOW}
      state="ready"
      onAlreadyDecided={() => undefined}
    />,
    { ...RAIL_ROUTES, "GET /approvals": () => jsonResponse({ data: [] }) },
  ),
};

// A failed read of the queue. The deck says so where the cards would be, rather
// than the column going blank — the ranked queue beside it is healthy.
export const DecisionsRefused: Story = {
  render: part(
    <DecisionsSection
      items={[]}
      nowMs={NOW}
      state="failed"
      onAlreadyDecided={() => undefined}
    />,
  ),
};

// One day, ranked by the server, with four sections in the order it chose.
//
// Deliberately NOT in section order — a "move revenue" row sits above a "build
// pipeline" one and below a "respond now" one, which is what the real ranking
// produces and what makes the run-length labelling meaningful. A story whose
// rows happened to be section-sorted would draw the same page a grouping client
// draws and prove nothing.
const feedDay: Worklist = {
  as_of: "2026-09-03T06:42:00Z",
  scope: "mine",
  scope_options: ["mine"],
  summary: { urgent: 1, due: 2, lower_priority: 1, total: 4 },
  sources_unavailable: [],
  reach: [],
  counts: [],
  readings: {
    revenue_at_risk_minor: null,
    buyer_replies: 1,
    prospecting: 1,
    review: 0,
    more_available: false,
  },
  queue: [
    sectioned(leadRow("lead-1"), "respond_now"),
    sectioned(meetingRow("meet-1", false), "prepare_conversations"),
    sectioned(overnightRow("deal-1", "d-1"), "move_revenue"),
    sectioned(leadRow("lead-2", "2026-09-04T09:00:00Z"), "build_pipeline"),
  ],
};

// Two rows of one section, adjacent. The second must draw no label.
const repeatedSectionDay: Worklist = {
  ...feedDay,
  queue: [
    sectioned(overnightRow("deal-1", "d-1"), "move_revenue"),
    sectioned(overnightRow("deal-2", "d-2"), "move_revenue"),
    sectioned(leadRow("lead-1"), "respond_now"),
  ],
};

type FeedItem = components["schemas"]["WorklistItem"];

/** One row, labelled with the section the server put it in. */
function sectioned(
  item: FeedItem,
  section: NonNullable<FeedItem["brief_section"]>,
): FeedItem {
  return { ...item, brief_section: section };
}

// The morning as ONE feed, in the server's order, with the section label drawn
// where it changes. Four rows and four sections, so the run-length labelling is
// visible: a label appears once and the next row under it says nothing again.
export const Feed: Story = {
  render: part(<BriefFeed day={feedDay} state="ready" />, RAIL_ROUTES),
};

// Two rows of one section in a row. The second draws no label, which is the
// whole of what "a label, not a grouping" looks like on screen.
export const FeedRepeatedSection: Story = {
  render: part(
    <BriefFeed day={repeatedSectionDay} state="ready" />,
    RAIL_ROUTES,
  ),
};

// A read that landed on nothing. "Nothing is waiting" is only ever said about a
// read that ANSWERED — the failed case below draws something else entirely.
export const FeedClear: Story = {
  render: part(
    <BriefFeed day={{ ...feedDay, queue: [] }} state="ready" />,
    RAIL_ROUTES,
  ),
};

// The read behind the feed failed. The panel says so where the rows would be,
// rather than drawing the empty plate a clear morning shows — the two are
// different facts and a page that drew them alike would send a rep away
// believing their morning was clear.
export const FeedRefused: Story = {
  render: part(<BriefFeed day={undefined} state="failed" />, RAIL_ROUTES),
};

// ── The context rail ────────────────────────────────────────────────────────

// What the night shift did: the capture counts, the duplicates that need a look,
// and what moved on the projects.
export const Overnight: Story = {
  render: part(<OvernightPanel />),
};

// The installation's first morning: the digest 404s because no run has been made
// yet, and the panel says that rather than drawing zeroes.
export const OvernightAbsent: Story = {
  render: part(<OvernightPanel />, {
    ...RAIL_ROUTES,
    "GET /digest": () => jsonResponse(NOT_FOUND, 404),
  }),
};

// The open pipeline, one line per currency. Never summed: adding native minor
// units across currencies produces a number that is not money.
export const Position: Story = {
  render: part(<PositionPanel />),
};

// Open deals that have gone quiet, named through the same company resolution the
// pipeline board uses. Staleness is stated in WORDS — the badge — and the card
// carries no edge stripe saying the same thing a second time.
export const Watch: Story = {
  render: part(
    <WatchPanel
      deals={deals.filter((deal) => deal.stalled)}
      more={false}
      state="ready"
    />,
  ),
};

// The deals read stopped at one page. What is on the panel is some of the quiet
// deals and not all of them, so it says so under the rows — "nothing has gone
// quiet" is a claim this read cannot make.
export const WatchPartial: Story = {
  render: part(
    <WatchPanel
      deals={deals.filter((deal) => deal.stalled)}
      more
      state="ready"
    />,
  ),
};

// Nothing has gone quiet, which is news worth drawing rather than an empty rail.
export const WatchClear: Story = {
  render: part(<WatchPanel deals={[]} more={false} state="ready" />),
};

// The deals read failed. "Nothing has gone quiet" is a claim about the deals, so
// it may only be made once they have been read — a failure that reached that
// sentence told a reader their pipeline was healthy on the strength of a request
// that never answered.
export const WatchRefused: Story = {
  render: part(<WatchPanel deals={[]} more={false} state="failed" />),
};

// ── The day's schedule and what it owes ─────────────────────────────────────

// A meeting the queue carries, at the hour it starts. The time is `due_at`:
// `occurred_at` is when something HAPPENED, which a meeting still ahead of the
// reader has no answer for, and a schedule with no times in it is a duplicate
// of the meetings count in the strip above.
export const Schedule: Story = {
  render: part(
    <SchedulePanel
      day={readingsDay({}, [meetingRow("m1", false), meetingRow("m2", true)])}
      state="ready"
    />,
  ),
};

// The server sent a meeting with no start. It is drawn without a time rather
// than with an invented one — a wrong hour would send a rep somewhere nobody
// booked them for.
export const ScheduleUndated: Story = {
  render: part(
    <SchedulePanel
      day={readingsDay({}, [{ ...meetingRow("m1", true), due_at: undefined }])}
      state="ready"
    />,
  ),
};

// A day with nothing booked, which is a fact worth saying rather than an empty
// panel that reads as a read that never landed.
export const ScheduleClear: Story = {
  render: part(<SchedulePanel day={readingsDay({}, [])} state="ready" />),
};

// The tasks this rep owes, under a heading that names two things. The line
// beneath it stands on every reading, including the empty one: promises made in
// conversation reach nothing, and an empty panel would otherwise claim none are
// outstanding.
export const Promises: Story = {
  render: part(
    <PromisesPanel
      day={readingsDay({}, [meetingRow("m1", true)])}
      state="ready"
    />,
  ),
};

// ── The week ahead ──────────────────────────────────────────────────────────

/** The plan reads, for a seat holding the grants each control asks for. */
const PLAN_ROUTES: RouteMap = {
  ...RAIL_ROUTES,
  "GET /me": meRoute(
    { weekly_plan: ["read", "create", "update"] },
    { roles: ["rep"] },
  ),
  "GET /weekly-plans/current": () =>
    jsonResponse({
      id: "p1",
      local_week_start: "2026-06-08",
      status: "open",
      commitments: [
        {
          id: "c1",
          label: "Call the Aster buyer back",
          state: "open",
          position: 1,
          due_on: "2026-06-11",
          help_requested: null,
          manager_response: null,
          manager_user_id: null,
          responded_at: null,
          completed_at: null,
        },
      ],
    }),
};

// The week a rep is keeping. Ticking a box stages it and reveals Save; nothing
// reaches the wire until then, which is why these are checkboxes and not
// switches.
export const Plan: Story = {
  render: part(<PlanSection />, PLAN_ROUTES),
};

// No plan yet, and this seat may open one. The sentence is a fact about the
// week; the button is the act.
export const PlanNone: Story = {
  render: part(<PlanSection />, {
    ...PLAN_ROUTES,
    "GET /weekly-plans/current": () => jsonResponse(NOT_FOUND, 404),
  }),
};

// A seat the server refuses. The week is still reported — withholding it would
// say the reader had planned nothing — and every write verb is absent, with the
// posture said once rather than a refusal repeated on each row.
export const PlanReadOnly: Story = {
  render: part(<PlanSection />, {
    ...PLAN_ROUTES,
    "GET /me": meRoute({ weekly_plan: ["read"] }, { roles: ["read_only"] }),
  }),
};

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  digest,
  NOT_FOUND,
  pipelineRows,
  readingsDay,
  report,
  team,
  teamWeek,
} from "./brief.fixtures";
import { BriefGlance } from "./brief.glance";
import { PlanSection } from "./brief.plan";
import { BriefReadingsStrip } from "./brief.readings";
import { TeamWeeklyPanel } from "./brief.teamweekly";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

// Brief, one part at a time.
//
// `brief.stories.tsx` documents the whole morning; this file documents the
// pieces of it that have no file of their own, because each has states the
// assembled page can only show one of at a time — a briefing whose day has not
// answered, a readings row whose source stopped short, a week nobody has
// planned. Same fixtures as the page (`brief.fixtures.ts`), so a part cannot
// drift from the page it is part of.
//
// The feed, the decisions deck and the rail's panels are NOT here: each now
// ships its own co-located story file, and a second frame for the same
// component in this one is a second answer to how that component draws.
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
  // The readings strip's pipeline card makes two reads of its own: the scope
  // the server names for this reader, and the forecast under it. Both are
  // routed, because installFetchStub's fallback answers a 200 with a list
  // envelope — a shape that resolves the query SUCCESSFULLY and leaves the card
  // formatting undefined figures, which is how a catalog ends up drawing
  // "NaN of NaN priced" while every test stays green.
  "GET /analytics/context": () =>
    jsonResponse({
      default_scope: { kind: "owner", id: "u-1", label: "Lena Fischer" },
      scopes: [],
    }),
  "GET /forecast": () =>
    jsonResponse({
      period_start: "2026-07-01",
      period_end: "2026-09-30",
      scope_kind: "owner",
      open_minor: 42_000_000,
      weighted_minor: 16_800_000,
      best_case_minor: 24_000_000,
      evidence_minor: 12_000_000,
      eligible_count: 12,
      priced_count: 11,
      confirmed_date_count: 8,
      fx_missing_count: 0,
      as_of: "2026-09-03T06:42:00Z",
      base_currency: "EUR",
    }),
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
  "GET /companies/company-nordwind": () =>
    jsonResponse({ id: "company-nordwind", display_name: "Nordwind Logistik" }),
  "GET /companies/company-acme": () =>
    jsonResponse({ id: "company-acme", display_name: "Acme Fördertechnik" }),
};

/** One part, with the reads it makes answered and nothing else reachable. */
function part(node: React.ReactNode, routes: RouteMap = RAIL_ROUTES) {
  return () => {
    installFetchStub(routes);
    return <StoryProviders>{node}</StoryProviders>;
  };
}

const meta: Meta = {
  title: "Shell/Brief parts",
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
} as unknown as Parameters<typeof BriefGlance>[0]["day"];

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
} as unknown as Parameters<typeof BriefGlance>[0]["week"];

// The header the Brief opens with: a greeting and ONE composed sentence about
// the day — two lines, and nothing between or above them. The uppercase eyebrow
// naming the view, the clock reporting the minute the queue was read, and the
// date under the greeting were three lines a reader already knew.
//
// The sentence is the part to read closely: the LEAD is a link into the row it
// names, and the tail — "Then N more" — reaches the day's own order further
// down the page, so the words a reader acts on are the words they can press.
// The tail is a button rather than an anchor because every href in this product
// is a route, and a fragment link would replace it.
export const Glance: Story = {
  render: part(
    <BriefGlance
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
    <BriefGlance
      view="morning"
      day={GLANCE_DAY}
      week={null}
      firstName={null}
      now={NOW_DATE}
    />,
  ),
};

// The weekly says its own thing, composed from the counts the week was frozen
// with rather than from the ranked queue — which describes THIS morning and
// would read as the wrong week entirely. It draws the SAME two lines the
// morning does: two views drawn alike need no kicker to tell them apart, and
// one of them wearing one would be the odd page.
export const GlanceWeekly: Story = {
  render: part(
    <BriefGlance
      view="weekly"
      day={GLANCE_DAY}
      week={GLANCE_WEEK}
      firstName="Lena"
      now={NOW_DATE}
    />,
  ),
};

// A week nobody has written yet. The standing line says nothing about it,
// because there is nothing yet to say — never a quiet-week claim, which would
// tell a rep their week was calm on no evidence. It is drawn at the composed
// sentence's own face, not as a caption: in the slot the page opens with, a
// caption reads as a footnote.
export const GlanceWeeklyUnread: Story = {
  render: part(
    <BriefGlance
      view="weekly"
      day={GLANCE_DAY}
      week={undefined}
      firstName="Lena"
      now={NOW_DATE}
    />,
  ),
};

// ── The readings strip ──────────────────────────────────────────────────────

// ONE DENSE ROW: label, figure, basis line, and the way into the lane the
// figure counted in the card's own foot. The density is the change here — the
// tile keeps every line at the size every stat card in the product draws it,
// and gives up only air, so five readings are taken in at one glance instead of
// pushing the day's own work below the fold.
//
// The figure is in NEUTRAL ink unless the reading counts something BREACHING:
// four coloured numbers in a row are a traffic light rather than a comparison.
// Press anywhere on a cell — the foot's word names the destination and the tile
// stretches that button over itself, so the reading is one thing to press.
export const Readings: Story = {
  render: part(<BriefReadingsStrip day={readingsDay()} />),
};

// A source ended short of its list, so every figure drawn from it is a floor.
// It is a `+` ON the figures — `8+` — rather than the sentence that used to
// stand under the row, and the cell's own hover line says why. Every figure the
// flag covers wears the mark, which is the contract's own claim: it is set once
// for the readings block, over four populations, so marking one slot would
// invite the reading where the other three are exact. The pipeline slot is a
// read of its own and stays unmarked.
//
// The MEETINGS slot is the case to look at: this day has none, and a zero draws
// a plain `0`. "0+" says "at least nothing", which is true of every number
// there has ever been — so the mark goes on a figure that counts something and
// nowhere else.
export const ReadingsCapped: Story = {
  render: part(
    <BriefReadingsStrip
      day={readingsDay(
        { buyer_replies: 100, prospecting: 8, more_available: true },
        [],
        [],
        { urgent: 12 },
      )}
    />,
  ),
};

// The same plate at a phone's width, where it is one full-width ROW per
// reading: label and basis leading, figure and its door stacked on the trailing
// edge, one hairline between, and no boxes at all. Two-up it was five 190px
// cards and 600px of readings before a reader reached the day's own work. The
// shape belongs to `StatStrip` and keys off the slots declaring
// `density="compact"`, so no other strip in the product folds this way —
// `Design System/StatStrip` has both side by side.
export const ReadingsOnAPhone: Story = {
  globals: { viewport: { value: "phone" } },
  render: part(
    <BriefReadingsStrip
      day={readingsDay({ prospecting: 8, more_available: true }, [], [], {
        urgent: 12,
      })}
    />,
  ),
};

// A quiet morning. The strip does not get shorter on a day with less in it,
// because a reader comparing it with yesterday's would take the missing slot for
// an answered question — the zeros are the answer, and a zero gets no special
// treatment beyond the compact cell it sits in.
export const ReadingsQuiet: Story = {
  render: part(
    <BriefReadingsStrip
      day={readingsDay({ buyer_replies: 0, prospecting: 0 }, [])}
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

// ── The team's week ─────────────────────────────────────────────────────────

/**
 * The two reads the team's week makes: which teams exist, and one team's
 * snapshot.
 *
 * One team in the list on purpose — a picker whose only option is the one
 * already showing asks the reader to confirm what they cannot change, so the
 * panel reads a single team straight through and draws no control.
 */
const TEAM_ROUTES: RouteMap = {
  "GET /me": meRoute({}),
  "GET /teams": () =>
    jsonResponse({
      data: [team],
      page: { next_cursor: null, has_more: false },
    }),
  "GET /weekly-reviews/team": () => jsonResponse(teamWeek),
};

// A lead's Monday: the headline against the bar it measured, the coverage, the
// team's landing, and the agenda in the order the conversation should take.
export const TeamWeekly: Story = {
  render: part(<TeamWeeklyPanel offered />, TEAM_ROUTES),
};

// The reader may open the picker and not the week behind it: a row scope that
// reaches only their own rows. The panel says which absence this is, because a
// lead refused and a team whose first week has not closed are different facts
// and the blank space is identical.
export const TeamWeeklyForbidden: Story = {
  render: part(<TeamWeeklyPanel offered />, {
    ...TEAM_ROUTES,
    "GET /weekly-reviews/team": () =>
      jsonResponse({ title: "Forbidden", code: "forbidden" }, 403),
  }),
};

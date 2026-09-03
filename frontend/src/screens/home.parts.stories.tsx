// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { deckItems } from "./home";
import { DecisionsSection } from "./home.decisions";
import {
  bundle,
  deals,
  digest,
  NOT_FOUND,
  pipelineRows,
  quietRun,
  ranked,
  readingsDay,
  report,
  singles,
} from "./home.fixtures";
import { HomeGlance } from "./home.glance";
import { OvernightPanel, PositionPanel, WatchPanel } from "./home.rail";
import { HomeReadingsStrip } from "./home.readings";
import { FocusSection } from "./home.today";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

// A story draws one section on its own, so nothing leads the page above it.
const NOTHING_ABOVE: ReadonlySet<string> = new Set();

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
      firstName={null}
      now={NOW_DATE}
    />,
  ),
};

// The weekly says its own thing under its own heading: no composed sentence,
// because that one is built from the ranked queue and describes THIS morning.
export const GlanceWeekly: Story = {
  render: part(
    <HomeGlance
      view="weekly"
      day={GLANCE_DAY}
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

// The ranked queue: the composite as the first row of the same axis its five
// factors are drawn on, and the factors in a softer fill so the row that leads
// reads first.
export const Focus: Story = {
  render: part(
    <FocusSection
      brief={ranked}
      deals={deals}
      nowMs={NOW}
      state="ready"
      drawnAbove={NOTHING_ABOVE}
    />,
    { ...RAIL_ROUTES, "POST /brief": () => jsonResponse(ranked) },
  ),
};

// A run that ranked nothing, with the candidate count under it. Honest quiet,
// and no invented urgency.
export const FocusQuietRun: Story = {
  render: part(
    <FocusSection
      brief={quietRun}
      deals={deals}
      nowMs={NOW}
      state="ready"
      drawnAbove={NOTHING_ABOVE}
    />,
    { ...RAIL_ROUTES, "POST /brief": () => jsonResponse(quietRun) },
  ),
};

// No run has ever been made. The panel offers to make one instead of drawing an
// empty queue that looks like a failure.
export const FocusNoRun: Story = {
  render: part(
    <FocusSection
      brief={null}
      deals={deals}
      nowMs={NOW}
      state="ready"
      drawnAbove={NOTHING_ABOVE}
    />,
    {
      ...RAIL_ROUTES,
      "POST /brief": () => jsonResponse(ranked),
    },
  ),
};

// The read behind the queue failed. The panel says so where the cards would be,
// rather than drawing the empty plate a morning with no run would show — the two
// are different facts and used to look identical.
export const FocusRefused: Story = {
  render: part(
    <FocusSection
      brief={null}
      deals={deals}
      nowMs={NOW}
      state="failed"
      drawnAbove={NOTHING_ABOVE}
    />,
  ),
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

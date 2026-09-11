// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  deals,
  digest,
  meetingRow,
  NOT_FOUND,
  pipelineRows,
  readingsDay,
  report,
} from "./brief.fixtures";
import { PositionPanel, RailQuiet, WatchPanel } from "./brief.rail";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "./story-utils";
import type { WorklistItem } from "./worklist.queries";

// Brief's context rail: what the pipeline is worth, what has gone quiet, and
// the one panel a silent morning gets.
//
// TWO HALVES OF ONE RULE, AND BOTH ARE HERE. A panel earns its box by having
// something in it: empty, it draws nothing, and `RailQuiet` prints one line per
// silent source at the foot of the rail. A clear morning used to stack four
// panels of chrome around four grey sentences, and the panels that DID have
// news were read last. So the collapsed frames below are passing, and the quiet
// panel is what stands in their place.
//
// Read every frame in BOTH themes with the toolbar's Theme control.

/** The reads the rail's panels fan out to, answered. */
const RAIL_ROUTES: RouteMap = {
  "GET /me": meRoute({}),
  "GET /digest": () => jsonResponse(digest),
  "POST /reports/deals-by-stage": () => report(pipelineRows),
  "GET /companies/company-nordwind": () =>
    jsonResponse({ id: "company-nordwind", display_name: "Nordwind Logistik" }),
  "GET /companies/company-acme": () =>
    jsonResponse({ id: "company-acme", display_name: "Acme Fördertechnik" }),
};

/** One panel, in a rail-width column. */
function panel(node: React.ReactNode, routes: RouteMap = RAIL_ROUTES) {
  return () => {
    installFetchStub(routes);
    return (
      <StoryProviders>
        <div className="brief-rail" style={{ maxWidth: 320 }}>
          {node}
        </div>
      </StoryProviders>
    );
  };
}

/** A row the task lane would send. */
function taskRow(id: string, title: string): WorklistItem {
  return {
    id,
    source: "task",
    level: 2,
    category: "tasks",
    title,
    because: [],
    consequence: "task_slips",
    actions: ["complete", "open"],
  };
}

const quiet = deals.filter((deal) => deal.stalled);

const meta: Meta = {
  title: "Shell/Brief rail",
};
export default meta;
type Story = StoryObj;

// ── The open pipeline ───────────────────────────────────────────────────────

// One line per currency, never summed: adding native minor units across
// currencies produces a number that is not money.
//
// THE FIGURE IS A SCALE, NOT AN AMOUNT. `€99k` is what this column's width can
// carry and what a reader takes in at a glance; `€99,000.00` wrapped mid-number
// here, and the exact figure belongs to the Pipeline screen, which is where a
// reader checking one goes. Under 10,000 the compact form IS the exact one —
// "€8k" is no shorter than "€8,332" and says less.
export const Pipeline: Story = {
  render: panel(<PositionPanel />),
};

// A mask kept rows out of these sums, so every figure above understates the
// pipeline. The footer says so: a partial answer a reader knows is partial is
// worth having, and one they do not is wrong.
export const PipelineMasked: Story = {
  render: panel(<PositionPanel />, {
    ...RAIL_ROUTES,
    "POST /reports/deals-by-stage": () => report(pipelineRows, 4),
  }),
};

// A refusal is NOT an absence. An empty panel would read as "there is no
// pipeline", which is a claim about the data made in place of one about
// authority — so the panel keeps its place and says the figure is unavailable.
export const PipelineRefused: Story = {
  render: panel(<PositionPanel />, {
    ...RAIL_ROUTES,
    "POST /reports/deals-by-stage": () =>
      jsonResponse({ title: "Forbidden", code: "forbidden" }, 403),
  }),
};

// No open pipeline at all: no panel, and no headline skeleton either — a shape
// where a number will be is a claim that a number is coming.
export const PipelineCollapsed: Story = {
  render: panel(<PositionPanel />, {
    ...RAIL_ROUTES,
    "POST /reports/deals-by-stage": () => report([]),
  }),
};

// ── What has gone quiet ─────────────────────────────────────────────────────

// Open deals nobody has heard from, named through the same company resolution
// the pipeline board uses. Staleness is stated in WORDS — the badge — and the
// card carries no edge stripe saying the same thing a second time.
export const Watch: Story = {
  render: panel(<WatchPanel deals={quiet} more={false} state="ready" />),
};

// The deals read stopped at one page, so what is here is SOME of the quiet
// deals and not all of them. The panel keeps its box to say that: past the end
// of the page, "nothing has gone quiet" is a claim this read cannot make.
export const WatchPartial: Story = {
  render: panel(<WatchPanel deals={quiet} more state="ready" />),
};

// Nothing has gone quiet, and the read ended the list: no panel. The line moves
// to the quiet panel below.
export const WatchCollapsed: Story = {
  render: panel(<WatchPanel deals={[]} more={false} state="ready" />),
};

// The deals read failed, which is a fact about the request rather than about
// the pipeline — so the box stands and the failure is said out loud.
export const WatchRefused: Story = {
  render: panel(<WatchPanel deals={[]} more={false} state="failed" />),
};

// ── The quiet panel ─────────────────────────────────────────────────────────

// A morning with no news in it at all: nothing booked, nothing due, no nightly
// run yet, and nothing gone quiet. Four absent panels would say nothing — a
// reader cannot tell a silent source from one the page forgot to draw — so one
// panel carries one line per source, at meta size, in the rail's own order.
export const Quiet: Story = {
  render: panel(
    <RailQuiet
      day={readingsDay({}, [])}
      dayState="ready"
      deals={[]}
      more={false}
      dealsState="ready"
    />,
    { ...RAIL_ROUTES, "GET /digest": () => jsonResponse(NOT_FOUND, 404) },
  ),
};

// One source silent and the rest reporting: the panel names only what it can
// account for. A line per source it has not heard from would be the other
// failure — a rail claiming a clear day off a read still in flight.
export const QuietOneSource: Story = {
  render: panel(
    <RailQuiet
      day={readingsDay({}, [
        meetingRow("m1", true),
        taskRow("t1", "Call Alice"),
      ])}
      dayState="ready"
      deals={[]}
      more={false}
      dealsState="ready"
    />,
  ),
};

// Every source has news, so there is nothing to report about the reporting and
// the panel draws nothing. An empty frame here is the pass.
export const QuietCollapsed: Story = {
  render: panel(
    <RailQuiet
      day={readingsDay({}, [
        meetingRow("m1", true),
        taskRow("t1", "Call Alice"),
      ])}
      dayState="ready"
      deals={quiet}
      more={false}
      dealsState="ready"
    />,
  ),
};

// The rail as a reader meets it on a quiet morning: the panels that have
// something say it, and one line each stands for the sources that do not.
export const RailOnAQuietMorning: Story = {
  render: panel(
    <>
      <WatchPanel deals={quiet} more={false} state="ready" />
      <RailQuiet
        day={readingsDay({}, [])}
        dayState="ready"
        deals={quiet}
        more={false}
        dealsState="ready"
      />
    </>,
    { ...RAIL_ROUTES, "GET /digest": () => jsonResponse(NOT_FOUND, 404) },
  ),
};

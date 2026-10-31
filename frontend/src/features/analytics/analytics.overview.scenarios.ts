// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The named situations the Overview must handle, one server per view. Each
// scenario answers all three views: a setup, or the reason the pair says
// nothing. Stories and tests read this one map, so they describe one server.

import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import {
  compared,
  isMovement,
  metric,
  overviewRoutes,
  teamReview,
} from "./analytics.overview.fixtures";
import {
  backlogFindings,
  boardCharts,
  bookingsTrend,
  framed,
  frames,
  prospectingOn,
  quarterMovement,
  quarterSales,
  teamOn,
  teamReviewOn,
  waitingHandoffs,
  workday,
  workRows,
  zeroWeek,
} from "./analytics.overview.scenariodata";
import type { ReportingEvaluation } from "./reporting.model";

export type OverviewView = "sales" | "prospecting" | "team";
export const OVERVIEW_VIEWS: readonly OverviewView[] = [
  "sales",
  "prospecting",
  "team",
];

type Routes = NonNullable<Parameters<typeof overviewRoutes>[0]>;
export type OverviewSetup = Readonly<{
  answer: (url: URL) => ReportingEvaluation;
  routes: Routes;
}>;
type Pair = OverviewSetup | Readonly<{ excluded: string }>;

/** The states every figure and view must keep apart. */
export const ANALYTICS_STATES = [
  "populated",
  "trueZero",
  "noTarget",
  "wrongTargetScope",
  "notEnoughHistory",
  "limitedImportHistory",
  "partialValuation",
  "sourceUnavailable",
  "tooFewObservations",
  "individualPerformanceOff",
  "noActionableWork",
  "largeBacklog",
] as const;

export type OverviewScenario = Readonly<{
  about: string;
  views: Readonly<Record<OverviewView, Pair>>;
}>;

export function isSetup(pair: Pair): pair is OverviewSetup {
  return "answer" in pair;
}

const quarterClock = frames.quarterCutoff.clock;
const week = frames.weekAcrossMonths;
const dstWeek = frames.weekWithClockChange;

// Sales review answers its movement read apart from its headline read.
function sales(evaluation: ReportingEvaluation, routes: Routes = {}): Pair {
  const movement = quarterMovement();
  return {
    answer: (url) => (isMovement(url) ? movement : evaluation),
    routes,
  };
}

function prospecting(
  evaluation: ReportingEvaluation,
  routes: Routes = {},
): Pair {
  return { answer: () => evaluation, routes: { seat: "sdr", ...routes } };
}

function team(evaluation: ReportingEvaluation, routes: Routes = {}): Pair {
  return {
    answer: () => evaluation,
    routes: { template: "sdr", ...routes },
  };
}

const json =
  (body: unknown, status = 200) =>
  () =>
    jsonResponse(body, status);
const unavailable = json({ title: "Unavailable", status: 503 }, 503);
const day = (queue = workRows, over = {}) =>
  json(workday(week.clock, queue, over));
const handoffRows = json({
  as_of: week.clock,
  rows: waitingHandoffs(week.clock),
});
const summary = (workflow: "handoff" | "qualification") =>
  json({
    as_of: week.clock,
    scope: { kind: "owner", id: "00000000-0000-4000-8000-0000000000a1" },
    workflow,
    ...(workflow === "handoff"
      ? {
          handoffs: {
            waiting: 3,
            unassigned: 0,
            blocked: 1,
            returned: 1,
            oldest_waiting_since: "2026-10-26T08:30:00Z",
          },
        }
      : {}),
    upcoming_meetings: 4,
  });

// A team in qualification mode: the review has no handoffs, so its members
// carry nothing but meetings and zeros in the handoff columns.
const qualificationReview = teamReviewOn(dstWeek, {
  workflow: "qualification",
  handoffs: undefined,
  members: teamReview().members.map((member) => ({
    ...member,
    submitted: 0,
    accepted: 0,
    rejected: 0,
    recycled: 0,
    waiting: 0,
    oldest_waiting_since: undefined,
  })),
});

const noTarget = {
  status: "no_target",
  target_metric: "bookings_won",
} as const;

const SALES_ONLY = "Only Sales review reads the forecast landing.";
const NO_ACTION_LIST = "Team prospecting has no action list.";
const NO_MONEY = "Prospecting figures are counts and carry no money.";
const NO_TARGET_HERE = "Team prospecting shows no target.";
const QUARTERS = "Sales review reads quarters; this frame is a week.";
const WORKFLOW = "The SDR workflow does not change Sales review.";

export const OVERVIEW_SCENARIOS = {
  populated: {
    about: "Ordinary data: a quarter with a trend, a week of SDR work.",
    views: {
      sales: sales(quarterSales()),
      prospecting: prospecting(prospectingOn(week), {
        worklist: day(),
        summary: summary("handoff"),
        handoffs: handoffRows,
      }),
      team: team(teamOn(dstWeek), {
        team: json(teamReviewOn(dstWeek)),
      }),
    },
  },
  trueZero: {
    about: "Every figure was measured and is zero.",
    views: {
      sales: sales(
        quarterSales({
          metrics: [
            metric("bookings_won", 0),
            metric("closed_win_rate", null, {
              coverage: { status: "no_data", withheld: false },
            }),
            metric("open_pipeline", 0),
            metric("forecast_landing", 0, {
              target_comparison: {
                ...compared,
                landing: 0,
                won: 0,
                gap: compared.target, // ds:ignore the target comparison money gap, not a CSS gap
                remaining: compared.target,
                open_pipeline: 0,
                coverage: 0,
              },
            }),
          ],
          charts: [
            bookingsTrend(frames.quarterCutoff, [
              ["4 Oct", "2026-10-04T22:00:00Z", 0],
              ["31 Oct", quarterClock, 0],
            ]),
          ],
        }),
      ),
      prospecting: prospecting(prospectingOn(week, zeroWeek()), {
        worklist: day(workRows.slice(0, 2)),
      }),
      team: team(teamOn(dstWeek, zeroWeek()), {
        team: json(
          teamReviewOn(dstWeek, {
            meetings: { held: 0, limited_history: false },
            handoffs: {
              outcomes: { submitted: 0, accepted: 0, rejected: 0, recycled: 0 },
              cohort: {
                rounds: 0,
                accepted: 0,
                rejected: 0,
                recycled: 0,
                waiting: 0,
              },
              waiting: { waiting: 0, unassigned: 0, blocked: 0, returned: 0 },
              reasons: [],
              unattributed: 0,
            },
            members: [],
          }),
        ),
      }),
    },
  },
  noTarget: {
    about: "No target is set for the quarter or the month.",
    views: {
      sales: sales(quarterSales({ comparison: noTarget })),
      prospecting: prospecting(
        prospectingOn(week, [
          metric("meetings_held", 3),
          metric("accepted_opportunities", 1),
          metric("leads_promoted", 2, { denominator: 9 }),
          metric("lead_first_response", 2.4),
          metric("leads_awaiting_response", 4),
        ]),
      ),
      team: { excluded: NO_TARGET_HERE },
    },
  },
  wrongTargetScope: {
    about: "A target exists, but for another scope than the one selected.",
    views: {
      sales: sales(
        quarterSales({
          comparison: {
            status: "incompatible",
            target_metric: "bookings_won",
            incompatibility: "scope",
            supported_selection: {
              ...quarterSales().selection,
              scope: { kind: "workspace" },
            },
          },
        }),
      ),
      prospecting: {
        excluded: "My prospecting reads only the reader's own targets.",
      },
      team: { excluded: NO_TARGET_HERE },
    },
  },
  notEnoughHistory: {
    about: "The quarter has one point of history and no second capture.",
    views: {
      sales: sales(
        quarterSales({
          charts: [
            bookingsTrend(frames.quarterCutoff, [
              ["31 Oct", quarterClock, 42_000_000],
            ]),
            ...boardCharts(),
          ],
        }),
      ),
      prospecting: {
        excluded: "My prospecting draws no history today.",
      },
      team: { excluded: "Team prospecting draws no history today." },
    },
  },
  limitedImportHistory: {
    about: "Won deals were imported, so history starts at the import.",
    views: {
      sales: sales(
        quarterSales({
          charts: [
            bookingsTrend(frames.quarterCutoff, [
              ["25 Oct", "2026-10-25T23:00:00Z", 30_500_000],
              ["31 Oct", quarterClock, 42_000_000],
            ]),
            ...boardCharts(),
          ],
        }),
      ),
      prospecting: {
        excluded: "No import provenance reaches My prospecting.",
      },
      team: team(teamOn(dstWeek), {
        team: json(
          teamReviewOn(dstWeek, {
            meetings: { held: 7, limited_history: true },
          }),
        ),
      }),
    },
  },
  partialValuation: {
    about: "One open deal is in a currency with no exchange rate.",
    views: {
      sales: sales(
        quarterSales({
          comparison: { status: "fx_missing", target_metric: "bookings_won" },
          metrics: [
            metric("bookings_won", 42_000_000),
            metric("closed_win_rate", 31),
            metric("open_pipeline", 129_000_000, {
              coverage: {
                status: "partial",
                withheld: false,
                eligible_count: 18,
                priced_count: 17,
                reason: "1 open deal in NOK has no exchange rate to EUR.",
              },
            }),
            metric("forecast_landing", null, {
              coverage: { status: "partial", withheld: false },
              target_comparison: {
                status: "fx_missing",
                target_metric: "bookings_won",
              },
            }),
          ],
        }),
      ),
      prospecting: { excluded: NO_MONEY },
      team: { excluded: NO_MONEY },
    },
  },
  sourceUnavailable: {
    about: "One figure and one source could not be read.",
    views: {
      sales: sales(
        quarterSales({
          metrics: [
            metric("bookings_won", null, {
              coverage: { status: "failed", withheld: false },
            }),
            metric("closed_win_rate", 31),
            metric("open_pipeline", 136_000_000),
          ],
        }),
        { checks: unavailable },
      ),
      prospecting: prospecting(
        prospectingOn(week, [
          metric("meetings_held", 3),
          metric("accepted_opportunities", 1),
          metric("leads_promoted", 2, { denominator: 9 }),
          metric("lead_first_response", null, {
            coverage: { status: "failed", withheld: false },
          }),
          metric("leads_awaiting_response", 4),
        ]),
        {
          summary: unavailable,
          worklist: day(workRows.slice(0, 3), {
            sources_unavailable: ["lead_response"],
          }),
        },
      ),
      team: team(teamOn(dstWeek), { team: unavailable }),
    },
  },
  tooFewObservations: {
    about: "Too few closed deals, answers or rounds to state a rate.",
    views: {
      sales: sales(
        quarterSales({
          metrics: [
            metric("bookings_won", 42_000_000),
            metric("closed_win_rate", null, {
              coverage: { status: "insufficient_sample", withheld: false },
            }),
            metric("open_pipeline", 136_000_000),
            metric("forecast_landing", 76_000_000, {
              target_comparison: compared,
            }),
          ],
        }),
      ),
      prospecting: prospecting(
        prospectingOn(week, [
          metric("meetings_held", 0),
          metric("accepted_opportunities", 0),
          metric("leads_promoted", 0, { denominator: 3 }),
          metric("lead_first_response", null, {
            coverage: { status: "insufficient_sample", withheld: false },
          }),
          metric("leads_awaiting_response", 2),
        ]),
        {
          worklist: day(),
          summary: summary("qualification"),
          template: "sales",
        },
      ),
      team: team(teamOn(dstWeek), { team: json(teamReviewOn(dstWeek)) }),
    },
  },
  individualPerformanceOff: {
    about: "Individual performance is off for the installation.",
    views: {
      sales: sales(
        quarterSales({
          charts: [
            bookingsTrend(frames.quarterCutoff, [
              ["31 Oct", quarterClock, 42_000_000],
            ]),
            ...boardCharts().map((chart) =>
              chart.kind === "owner_attainment"
                ? {
                    ...chart,
                    coverage: { status: "ok" as const, withheld: true },
                    points: [],
                  }
                : chart,
            ),
          ],
        }),
      ),
      // The reader's own figures stay theirs to see.
      prospecting: prospecting(prospectingOn(week), { worklist: day() }),
      team: team(teamOn(dstWeek), {
        team: json(
          teamReviewOn(dstWeek, { members_withheld: true, members: [] }),
        ),
      }),
    },
  },
  noActionableWork: {
    about: "Every source was read and nothing needs anyone.",
    views: {
      sales: sales(quarterSales(), { checks: json({ data: [] }) }),
      prospecting: prospecting(prospectingOn(week), { worklist: day([]) }),
      team: { excluded: NO_ACTION_LIST },
    },
  },
  largeBacklog: {
    about: "24 open review findings on 7 deals, and a long SDR queue.",
    views: {
      sales: sales(quarterSales(), {
        checks: json({ data: backlogFindings(quarterClock) }),
      }),
      prospecting: prospecting(prospectingOn(week), {
        worklist: day(workRows, {
          summary: { urgent: 1, due: 38, lower_priority: 171, total: 210 },
        }),
      }),
      team: { excluded: NO_ACTION_LIST },
    },
  },
  modelCommitDivergence: {
    about: "No committed business yet beside a wide modelled range.",
    views: {
      sales: sales(
        quarterSales({
          metrics: [
            metric("bookings_won", 0),
            metric("closed_win_rate", 28),
            metric("open_pipeline", 210_000_000),
            metric("forecast_landing", 0, {
              range_low_minor: 140_000_000,
              range_high_minor: 260_000_000,
              target_comparison: {
                ...compared,
                landing: 0,
                won: 0,
                gap: compared.target, // ds:ignore the target comparison money gap, not a CSS gap
                remaining: compared.target,
                open_pipeline: 190_000_000,
                coverage: 2.38,
              },
            }),
          ],
        }),
      ),
      prospecting: { excluded: SALES_ONLY },
      team: { excluded: SALES_ONLY },
    },
  },
  coverageNumerator: {
    about: "Coverage counts only deals expected to close this quarter.",
    views: {
      sales: sales(
        quarterSales({
          metrics: [
            metric("bookings_won", 72_000_000),
            metric("closed_win_rate", 34),
            metric("open_pipeline", 520_000_000, {
              coverage: { status: "ok", withheld: false, eligible_count: 61 },
            }),
            metric("forecast_landing", 78_500_000, {
              target_comparison: {
                ...compared,
                landing: 78_500_000,
                won: 72_000_000,
                gap: 1_500_000, // ds:ignore the target comparison money gap, not a CSS gap
                remaining: 8_000_000,
                open_pipeline: 310_000_000,
                coverage: 38.75,
              },
            }),
          ],
        }),
      ),
      prospecting: { excluded: SALES_ONLY },
      team: { excluded: SALES_ONLY },
    },
  },
  cutoffVersusHorizon: {
    about: "Actuals run to 31 Oct; the quarter and its target to 31 Dec.",
    views: {
      sales: sales(quarterSales()),
      prospecting: { excluded: QUARTERS },
      team: { excluded: QUARTERS },
    },
  },
  fiscalQuarter: {
    about: "A business year from February: Q3 is August to October.",
    views: {
      sales: sales(
        framed(
          quarterSales({
            charts: [
              bookingsTrend(frames.fiscalQuarter, [
                ["2 Aug", "2026-08-02T22:00:00Z", 4_000_000],
                ["6 Sep", "2026-09-06T22:00:00Z", 21_000_000],
                ["4 Oct", "2026-10-04T22:00:00Z", 35_500_000],
                ["20 Oct", frames.fiscalQuarter.clock, 42_000_000],
              ]),
              ...boardCharts(),
            ],
          }),
          frames.fiscalQuarter,
        ),
      ),
      prospecting: { excluded: QUARTERS },
      team: { excluded: QUARTERS },
    },
  },
  weekAcrossMonths: {
    about: "The week crosses into November; targets stay October's.",
    views: {
      sales: { excluded: QUARTERS },
      prospecting: prospecting(prospectingOn(week), { worklist: day() }),
      team: team(teamOn(week), { team: json(teamReviewOn(week)) }),
    },
  },
  titleOnlyWork: {
    about: "Work rows carry a reason, a due time, an owner and a verb.",
    views: {
      sales: { excluded: "Sales review lists review findings, not work rows." },
      prospecting: prospecting(prospectingOn(week), { worklist: day() }),
      team: { excluded: NO_ACTION_LIST },
    },
  },
  handoffWorkflow: {
    about: "SDRs hand prospects on; some wait, one on an inactive seat.",
    views: {
      sales: { excluded: WORKFLOW },
      prospecting: prospecting(prospectingOn(week), {
        worklist: day(),
        summary: summary("handoff"),
        handoffs: handoffRows,
      }),
      team: team(teamOn(dstWeek), { team: json(teamReviewOn(dstWeek)) }),
    },
  },
  qualificationWorkflow: {
    about: "SDRs qualify leads themselves; nothing is handed on.",
    views: {
      sales: { excluded: WORKFLOW },
      prospecting: prospecting(prospectingOn(week), {
        template: "sales",
        worklist: day(),
        summary: summary("qualification"),
      }),
      team: team(teamOn(dstWeek), {
        template: "sales",
        team: json(qualificationReview),
      }),
    },
  },
  weakNoHandoffTable: {
    about: "A qualification team whose member table has no qualified column.",
    views: {
      sales: { excluded: WORKFLOW },
      prospecting: {
        excluded: "My prospecting has no member table.",
      },
      team: team(teamOn(dstWeek), {
        template: "sales",
        team: json(qualificationReview),
      }),
    },
  },
  targetReached: {
    about: "Sales won has passed the quarter's target.",
    views: {
      sales: sales(
        quarterSales({
          comparison: {
            status: "target_reached",
            target_metric: "bookings_won",
            basis: "commit_evidence",
            target: 80_000_000,
            landing: 96_000_000,
            won: 88_000_000,
            remaining: 0,
            open_pipeline: 54_000_000,
          },
        }),
      ),
      prospecting: { excluded: SALES_ONLY },
      team: { excluded: SALES_ONLY },
    },
  },
  noForecast: {
    about: "The forecast could not give a landing for the quarter.",
    views: {
      sales: sales(
        quarterSales({
          metrics: [
            metric("bookings_won", 42_000_000),
            metric("closed_win_rate", 31),
            metric("open_pipeline", 136_000_000),
            metric("forecast_landing", null, {
              coverage: { status: "unavailable", withheld: false },
            }),
          ],
        }),
      ),
      prospecting: { excluded: SALES_ONLY },
      team: { excluded: SALES_ONLY },
    },
  },
} as const satisfies Record<string, OverviewScenario>;

export type ScenarioName = keyof typeof OVERVIEW_SCENARIOS;

const VIEW_STORY_PREFIX: Readonly<Record<OverviewView, string>> = {
  sales: "SalesReview",
  prospecting: "MyProspecting",
  team: "TeamProspecting",
};

/** The story export that draws one scenario on one view. */
export function scenarioStoryName(name: ScenarioName, view: OverviewView) {
  return `${VIEW_STORY_PREFIX[view]}${name.charAt(0).toUpperCase()}${name.slice(1)}`;
}

/** The address that opens a view, whatever a reader chose before. */
export function overviewAddress(view: OverviewView): string {
  return `#/analytics/overview?overview=${view}`;
}

/**
 * Serves a setup to the app: its routes, and each evaluation answered by the
 * request it is, so the movement read gets its own answer.
 */
export function installOverviewServer(setup: OverviewSetup): void {
  installFetchStub(overviewRoutes(setup.routes));
  const routed = globalThis.fetch;
  globalThis.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
    const request = input instanceof Request ? input : null;
    const url = new URL(
      request ? request.url : String(input),
      "http://localhost",
    );
    if (url.pathname.endsWith("/analytics/evaluate"))
      return Promise.resolve(jsonResponse(setup.answer(url)));
    return routed(input, init);
  }) as typeof fetch;
}

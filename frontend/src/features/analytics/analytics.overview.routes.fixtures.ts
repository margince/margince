// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { dayInZone, startOfDayInZone } from "../../format/timezone";

import type { GrantSpec } from "../../testing/mefixture";
import {
  interval,
  REPORTING_FIXTURE_ZONE,
  reportingStoryFramework,
} from "../../testing/reporting.fixtures";
import {
  jsonResponse,
  meRoute,
  type RouteMap,
} from "../../testing/story-utils";
import {
  CONTEXTS,
  MINE,
  overviewCatalog,
  QUIET_DAY,
  READS,
  RENEWALS_PIPELINE,
  SALES_PIPELINE,
  type Seat,
  salesEvaluation,
  teamReview,
} from "./analytics.overview.fixtures";

function pipelinePage(pipelines: readonly (typeof SALES_PIPELINE)[]) {
  return jsonResponse({
    data: pipelines,
    page: { next_cursor: null, has_more: false },
  });
}

/** `GET /pipelines` with the default pipeline and Renewals beside it. */
export const twoPipelines = () =>
  pipelinePage([SALES_PIPELINE, RENEWALS_PIPELINE]);

export function overviewRoutes(
  over: Readonly<{
    seat?: Seat;
    template?: "sales" | "sdr";
    grants?: GrantSpec;
    summary?: () => Response | Promise<Response>;
    checks?: () => Response | Promise<Response>;
    coverage?: () => Response | Promise<Response>;
    framework?: () => Response | Promise<Response>;
    team?: () => Response | Promise<Response>;
    worklist?: () => Response | Promise<Response>;
    handoffs?: () => Response | Promise<Response>;
    meetings?: () => Response | Promise<Response>;
    // The handoffs sent back: the waiting route asked with state=returned,
    // which the route map cannot tell apart, so openOverview answers it.
    returned?: () => Response;
    pipelines?: () => Response | Promise<Response>;
    evidence?: () => Response | Promise<Response>;
  }> = {},
): RouteMap {
  const seat = over.seat ?? "leader";
  const template = over.template ?? (seat === "sdr" ? "sdr" : "sales");
  return {
    "GET /me": meRoute(over.grants ?? READS),
    "GET /analytics/context": () => jsonResponse(CONTEXTS[seat]),
    "GET /analytics/metrics": () =>
      jsonResponse({ ...overviewCatalog, schedule_ready: true }),
    "GET /analytics/framework":
      over.framework ??
      (() =>
        jsonResponse({
          ...reportingStoryFramework,
          definition: { ...reportingStoryFramework.definition, template },
        })),
    "GET /pipelines": over.pipelines ?? (() => pipelinePage([SALES_PIPELINE])),
    "GET /forecast/assurance/exceptions":
      over.checks ?? (() => jsonResponse({ data: [] })),
    "GET /analytics/coverage":
      over.coverage ??
      (() =>
        jsonResponse({
          run_id: "00000000-0000-4000-8000-0000000000c1",
          as_of: interval.end_at,
          sources: [],
        })),
    "GET /analytics/sdr-summary":
      over.summary ??
      (() =>
        jsonResponse({
          as_of: interval.end_at,
          scope: MINE,
          workflow: template === "sdr" ? "handoff" : "qualification",
          handoffs:
            template === "sdr"
              ? { waiting: 2, unassigned: 0, blocked: 0, returned: 0 }
              : undefined,
          upcoming_meetings: 0,
          upcoming_window: upcomingWindow(interval.end_at),
        })),
    "GET /analytics/sdr-summary/handoffs":
      over.handoffs ??
      (() => jsonResponse({ as_of: interval.end_at, rows: [] })),
    "GET /analytics/sdr-summary/meetings":
      over.meetings ??
      (() =>
        jsonResponse({
          as_of: interval.end_at,
          window: upcomingWindow(interval.end_at),
          rows: [],
        })),
    "GET /analytics/sdr-team": over.team ?? (() => jsonResponse(teamReview())),
    "GET /worklist": over.worklist ?? (() => jsonResponse(QUIET_DAY)),
    // The records behind a figure: none, unless the case lists some.
    "GET /analytics/evidence":
      over.evidence ??
      (() =>
        jsonResponse({
          context: salesEvaluation().context,
          metric: "bookings_won",
          rows: [],
          truncated: false,
        })),
  };
}

/** The fourteen days the summary counts booked meetings in, from `asOf`. */
export function upcomingWindow(asOf: string) {
  // Fourteen days from the reporting zone's day, ending at its midnight.
  const day = dayInZone(Date.parse(asOf), REPORTING_FIXTURE_ZONE);
  return {
    start_at: asOf,
    end_at: startOfDayInZone(day, REPORTING_FIXTURE_ZONE, 14),
    timezone: REPORTING_FIXTURE_ZONE,
  };
}

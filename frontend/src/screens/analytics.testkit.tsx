// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What every Analytics suite needs before it can assert anything: the frame the
// screen reads first, and a server that answers the report POSTs.
//
// Its own file rather than a copy per suite. The stub models the SHAPE of a
// report result — the frame fields, the derivation link, the rows keyed by
// report — and a second copy is how one suite comes to be tested against a
// response no live server sends.

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { type RenderResult, render as rtlRender } from "@testing-library/react";
import type { ReactNode } from "react";
import { vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

export const render = (ui: ReactNode): RenderResult => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
};

export type ReportsStubOpts = {
  onRun?: (key: string, body: Record<string, unknown>) => void;
  // Model a server that sends only PART of the frame — an installation
  // mid-upgrade, which is the one place a partial result actually arrives.
  // Dropping the whole frame would be a weaker fixture: a guard that checks
  // only one of the three fields passes against it, and the caption then
  // renders with an undefined zone.
  partialFrame?: boolean;
  stageRows?: Record<string, unknown>[];
  forecastRows?: Record<string, unknown>[];
  companyRows?: Record<string, unknown>[];
  winLossRows?: Record<string, unknown>[];
  stageAgeRows?: Record<string, unknown>[];
  meetingRows?: Record<string, unknown>[];
  phaseRows?: Record<string, unknown>[];
  commitmentRows?: Record<string, unknown>[];
  quietRows?: Record<string, unknown>[];
  // The coverage read: a payload, a status (403 for a seat without the ops
  // grant, 404 for a fresh installation), or omitted for the default 403.
  coverage?: { status: number; body?: unknown };
  derivation?: Record<string, unknown>;
  onDerivation?: (url: string) => void;
  context?: Record<string, unknown>;
};

// The rows one report answers with.
//
// A NAMED report with no rows given answers an empty set: that is the honest
// fixture for "the engine ran and found nothing", and it is a different claim
// from the default pipeline row below, which stands in for whichever report a
// suite did not think about. Keyed rather than chained so a tenth report is one
// line and not one more level of nesting.
function reportRows(
  key: string,
  opts: ReportsStubOpts,
): Record<string, unknown>[] {
  const byReport: Readonly<
    Record<string, Record<string, unknown>[] | undefined>
  > = {
    forecast: opts.forecastRows,
    "activities-by-kind": opts.meetingRows,
    "projects-by-phase": opts.phaseRows,
    "project-commitments": opts.commitmentRows,
    "projects-gone-quiet": opts.quietRows,
    "win-loss": opts.winLossRows,
    "stage-age": opts.stageAgeRows,
    "open-deals-per-company": opts.companyRows,
  };
  if (Object.hasOwn(byReport, key)) {
    return byReport[key] ?? [];
  }
  return (
    opts.stageRows ?? [
      { stage_id: "pl-s1", raw_minor: 100000, deal_count: 2, currency: "EUR" },
    ]
  );
}

// The seat the suites read as. The coverage grant follows the coverage option,
// because the tab appears only for a reader who holds it and a stub that always
// granted it would draw a tab no such reader has.
function meAnswer(opts: ReportsStubOpts) {
  const granted = opts.coverage !== undefined && opts.coverage.status !== 403;
  return meFixture({
    roles: ["rep"],
    allow: { data_coverage: granted ? ["read"] : [] },
  });
}

// The nightly check's answer: a payload, or the status a seat without the ops
// grant (403) or a fresh installation (404) meets.
function coverageAnswer(opts: ReportsStubOpts) {
  const asked = opts.coverage ?? { status: 403 };
  return jsonResponse(
    asked.body ?? { title: "Forbidden", status: asked.status },
    asked.status,
  );
}

// Which population these numbers cover, and whether this reader may publish a
// forecast. Every Analytics surface reads it FIRST, so a stub without it leaves
// the screen waiting and every assertion looking like a rendering bug.
function contextAnswer(opts: ReportsStubOpts) {
  return (
    opts.context ?? {
      default_scope: { kind: "workspace", label: "Whole workspace" },
      allowed_scopes: [{ kind: "workspace", label: "Whole workspace" }],
      capabilities: {
        view_manager_forecast: true,
        submit_manager_forecast: true,
      },
      as_of: "2026-09-04T00:00:00Z",
      timezone: "Europe/Berlin",
      base_currency: "EUR",
    }
  );
}

// One default pipeline with one open stage: enough for the stage tables to name
// a stage rather than print its id.
const PIPELINES = {
  data: [
    {
      id: "pl",
      name: "Sales",
      is_default: true,
      position: 0,
      stages: [
        {
          id: "pl-s1",
          pipeline_id: "pl",
          name: "Qualify",
          position: 1,
          semantic: "open",
          win_probability: 20,
        },
      ],
    },
  ],
  page: { next_cursor: null },
};

// One report result, in the shape the endpoint answers with.
function reportAnswer(key: string, opts: ReportsStubOpts) {
  // The frame the server sends with every result. A fixture that omits it
  // models a response no live server produces, and the screen would then be
  // tested against a shape it never meets — `partialFrame` is the ONE case
  // where that is the point, an installation mid-upgrade.
  const frame = opts.partialFrame
    ? {}
    : {
        timezone: "Europe/Berlin",
        base_currency: "EUR",
        fiscal_year_start_month: 1,
      };
  return {
    report: key,
    plan: {},
    columns: [],
    rows: reportRows(key, opts),
    as_of: "2026-03-04T09:00:00Z",
    ...frame,
    derivation_url: `/v1/reports/${key}/derivation?by=stage_id&agg=sum:amount_minor:raw_minor&stage_id=pl-s1`,
  };
}

// What the stub was asked, in the one shape both call styles arrive as: the
// engine sends a `Request`, and a hand-written test may pass a url and init.
type Asked = Readonly<{
  url: string;
  method: string;
  json: () => Promise<Record<string, unknown>>;
}>;

type Route = Readonly<{
  when: (asked: Asked) => boolean;
  answer: (asked: Asked, opts: ReportsStubOpts) => Promise<Response> | Response;
}>;

async function reportRoute(asked: Asked, opts: ReportsStubOpts) {
  const key = asked.url.match(/\/reports\/([^/?]+)/)?.[1] ?? "";
  opts.onRun?.(key, await asked.json());
  return jsonResponse(reportAnswer(key, opts));
}

// The endpoints an Analytics suite meets, and what each answers. A TABLE and
// not a ladder of conditions: one row is one endpoint, so what the server does
// is readable without unwinding a nest, and the first match wins in the order
// written rather than in the order somebody happened to insert a branch.
const ROUTES: readonly Route[] = [
  {
    when: ({ url }) => url.endsWith("/me"),
    answer: (_asked, opts) => jsonResponse(meAnswer(opts)),
  },
  {
    when: ({ url }) => url.includes("/analytics/coverage"),
    answer: (_asked, opts) => coverageAnswer(opts),
  },
  {
    when: ({ url }) => url.includes("/analytics/context"),
    answer: (_asked, opts) => jsonResponse(contextAnswer(opts)),
  },
  {
    when: ({ url, method }) => method === "GET" && url.includes("/derivation"),
    answer: ({ url }, opts) => {
      opts.onDerivation?.(url);
      return jsonResponse(opts.derivation ?? {});
    },
  },
  {
    when: ({ url }) => url.includes("/pipelines"),
    answer: () => jsonResponse(PIPELINES),
  },
  {
    when: ({ url, method }) => method === "POST" && url.includes("/reports/"),
    answer: reportRoute,
  },
];

export function reportsStub(opts: ReportsStubOpts = {}) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = input instanceof Request ? input : null;
    const asked: Asked = {
      url: String(request ? request.url : input),
      method: request ? request.method : (init?.method ?? "GET"),
      json: async () =>
        request ? await request.json() : JSON.parse(String(init?.body)),
    };
    const route = ROUTES.find((candidate) => candidate.when(asked));
    // An endpoint this table does not name answers an empty page rather than
    // failing: the screen reads several lists it makes no claim about here.
    return route
      ? route.answer(asked, opts)
      : jsonResponse({ data: [], page: { next_cursor: null } });
  });
}

// A context whose default lens is one seat: the rep's own.
export const ownLensContext = {
  default_scope: { kind: "owner", id: "u-rep-1", label: "Riley Rep" },
  allowed_scopes: [{ kind: "owner", id: "u-rep-1", label: "Riley Rep" }],
  capabilities: {
    view_manager_forecast: false,
    submit_manager_forecast: false,
  },
  as_of: "2026-09-04T00:00:00Z",
  timezone: "Europe/Berlin",
  base_currency: "EUR",
};

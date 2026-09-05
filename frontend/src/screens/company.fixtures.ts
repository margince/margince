// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type Mock, vi } from "vitest";

// What the server answers a company record with, in ONE place.
//
// Mounting `CompanyScreen` fires the same five reads on every render — the
// composite 360, the hierarchy roll-up, the deterministic brief, the
// relationship strength, the assistant's context — before a suite can assert
// anything about the one card or tab it is actually about. A suite that carries
// its own copy of those answers is making a second claim about the same wire,
// free to drift from this one, and the next read the screen grows reaches only
// the copy whose file the author happened to have open.
//
// This module answers the WIRE and nothing else: response bodies for real
// endpoints, and the stub that routes a request to one. It never stands in for
// what the screen does with them — a fixture that reimplemented the page's own
// logic would prove the fixture right rather than the page. Mounting is the
// suite's own business, and keeping it out is what lets this file stay JSX-free:
// `scripts/fe-uat.mjs` reads every `.tsx` under `src/` as a component and asks
// for a story that renders it, which a set of response bodies cannot have.

export function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

/**
 * An empty, terminal collection page — the body every list read returns when
 * there is nothing to list, and the shape each section of the 360 carries.
 */
export const emptySection = {
  data: [],
  page: { has_more: false, next_cursor: null },
};

/** The same empty page as a ready `Response`, for a fetch stub's fall-through. */
export function emptyPage(): Response {
  return jsonResponse(emptySection);
}

export const org = {
  id: "o-1",
  display_name: "Brandt Automotive GmbH",
  industry: "Automotive",
  captured_by: "human:u1",
  source: "manual",
  version: 1,
  // Stated rather than omitted: an ABSENT `writable` means NOT writable (the
  // contract fails closed), so a fixture without it models a record nobody may
  // edit — a different record from the ordinary one these suites describe.
  writable: true,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

/**
 * The 360 backstop: an assembled account with every section present and empty.
 *
 * Assembled-but-empty is the state that lets a suite about one card render the
 * whole page without asserting anything about the rest of it — and it is a
 * different fact from a section the reader's role cannot read, which comes back
 * absent and named in `sections_omitted`. A suite that IS about a section
 * spreads its own over this one.
 */
export const org360 = {
  as_of: "2026-06-01T09:00:00Z",
  organization: org,
  sections_omitted: [],
  people: emptySection,
  deals: {
    ...emptySection,
    won_lifetime: { amount_minor: 0, currency: "EUR" },
    lost_count: 0,
  },
  activities: emptySection,
  next_steps: emptySection,
  pending_approvals: emptySection,
  tags: [],
  since_last_visit: {
    baseline_at: null,
    new_activities: 0,
    deal_stage_moves: 0,
    pending_proposals: 0,
  },
  suggestions: [],
  suggestions_dropped: 0,
};

/**
 * The roll-up backstop. It sits in the company view's left rail rather than
 * behind a tab, so every test that renders the page fires this GET.
 */
export const emptyRollup = {
  root_id: "o-1",
  scope: "tree",
  weighted_pipeline: { amount_minor: 0, currency: "EUR" },
  closed_won: { amount_minor: 0, currency: "EUR" },
  activity_count_30d: 0,
  aggregated_account_count: 1,
  restricted_excluded: [],
  computed_at: "2026-06-01T09:00:00Z",
};

/** A deterministic brief with nothing to say, in its quietest real state. */
export const emptyBrief = {
  organization_id: "o-1",
  generated_at: "2026-06-01T09:00:00Z",
  generated_by: "deterministic",
  sentences: [],
};

/**
 * The dormant/no-interactions strength response. The Company Overview fires
 * this GET unconditionally, so it is the default for every suite that is not
 * itself about the strength card.
 */
export const dormantStrength = {
  score: 0,
  bucket: "none",
  factors: { recency: 0, frequency: 0, reciprocity: 0, direction: 0 },
  last_interaction: null,
};

/**
 * Answers the record read the page shell needs and an empty page for everything
 * else, so a suite exercising one card does not have to plumb every other
 * request the screen fires.
 */
export async function companyBackstop(url: string): Promise<Response> {
  return url.endsWith("/organizations/o-1") ? jsonResponse(org) : emptyPage();
}

// rollupResponse lets a suite hand back either a body or a whole Response,
// because one of them asserts the honest 422 rather than a payload.
function rollupResponse(rollup: unknown): Response {
  if (rollup instanceof Response) {
    return rollup;
  }
  return jsonResponse(rollup ?? emptyRollup);
}

/**
 * A URL-capturing fetch stub for the company surfaces: every request is
 * recorded so a test can assert the params it carried, and a caller-supplied
 * responder decides what comes back.
 *
 * The reads the page shell fires on every render are answered up front from
 * their quiet defaults, so a suite that does not care about relationship
 * strength or the roll-up never plumbs a branch for them. A suite that IS about
 * one of them passes its own body through `options` — or, for the roll-up, a
 * whole `Response` when what it asserts is a refusal.
 */
// A reader who has never asked for this account's scan: the state the page
// meets on a first open, before its own ensure answers. The server merges
// the rules' live advice into every scan it wires, a never-read one included,
// so a never-scanned account still carries the 360's own rows.
export const neverScanned: {
  organization_id: string;
  state: string;
  findings: unknown[];
  findings_dropped: number;
} = {
  organization_id: "o-1",
  state: "never",
  findings: [],
  findings_dropped: 0,
};

// The rows a scan echoes from a 360 body: the server merges the rules' advice
// into the scan, so the fixture's default scan reads them from the 360 the
// same suite handed in rather than answering an empty list the server never
// would.
function neverScannedEchoing(view: unknown): typeof neverScanned {
  if (typeof view !== "object" || view === null) {
    return neverScanned;
  }
  const findings =
    "suggestions" in view && Array.isArray(view.suggestions)
      ? view.suggestions
      : [];
  const dropped =
    "suggestions_dropped" in view &&
    typeof view.suggestions_dropped === "number"
      ? view.suggestions_dropped
      : 0;
  return { ...neverScanned, findings, findings_dropped: dropped };
}

// The reads every company-page test meets and few of them are about,
// answered from the fixtures unless a suite hands in its own. Named routes
// rather than a chain in the mock, so a suite reads what the backstop
// answers as a table.
type BackstopOptions = Readonly<{
  strength?: unknown;
  org360?: unknown;
  scan?: unknown;
  rollup?: unknown;
  brief?: unknown;
}>;

function backstopAnswer(
  pathname: string,
  options?: BackstopOptions,
): Response | undefined {
  if (pathname.endsWith("/strength")) {
    return jsonResponse(options?.strength ?? dormantStrength);
  }
  if (pathname.endsWith("/context")) {
    return jsonResponse({
      anchor: { type: "organization", id: "o-1" },
      sections: [],
    });
  }
  if (pathname.endsWith("/360")) {
    return jsonResponse(options?.org360 ?? org360);
  }
  if (pathname.endsWith("/hierarchy-rollup")) {
    return rollupResponse(options?.rollup);
  }
  if (pathname.endsWith("/brief")) {
    return jsonResponse(options?.brief ?? emptyBrief);
  }
  // The account scan the page asks for on open: `never`, echoing the 360's
  // rows, unless a suite hands in one, so a page test is not also a scan test.
  if (pathname.endsWith("/scan")) {
    return jsonResponse(
      options?.scan ?? neverScannedEchoing(options?.org360 ?? org360),
    );
  }
  return undefined;
}

export function stubFetch(
  responder: (
    url: string,
    method: string,
    request: Request,
  ) => Promise<Response>,
  options?: BackstopOptions,
): {
  fetchMock: Mock<(request: Request) => Promise<Response>>;
  urls: string[];
} {
  const urls: string[] = [];
  const fetchMock = vi.fn(async (request: Request) => {
    urls.push(request.url);
    return (
      backstopAnswer(new URL(request.url).pathname, options) ??
      responder(request.url, request.method, request)
    );
  });
  vi.stubGlobal("fetch", fetchMock);
  return { fetchMock, urls };
}

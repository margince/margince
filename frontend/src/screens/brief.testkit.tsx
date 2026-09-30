import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { type RenderResult, render as rtlRender } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, vi } from "vitest";
import type { components } from "../api/schema";
import { meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { readingsDay } from "./brief.fixtures";
import type { Deal, MorningBrief } from "./brief.queries";

// Brief's suites share one harness, because they share one screen.
//
// Brief fans out to several reads on mount, and what a case is ABOUT is one of
// them; the rest have to be answered honestly or every case declares routes it
// does not care about. `stubApi` is that answer, and it also RECORDS every
// call, because what this screen must not do is as load-bearing as what it
// must: a staged verdict is only staged if nothing was sent, and no rendering
// can prove that.
//
// It lives here rather than in a suite because the file crossed the 1000-line
// ceiling frontend/AGENTS.md sets and had to be split, and the split has one
// honest shape. Importing across test files is refused by the lint, and rightly
// — a test file is not a module. Copying the harness would put a SECOND answer
// to "what does an unrouted read reply with" in the tree, and these cases turn
// on exactly that answer.
//
// Named `.testkit.` rather than `.test.`: the design-system and lint gates skip
// test files, and this one answers to the app's rules. The suffix is also what
// tells fe-uat this is not a component owing a story.

export type Approval = components["schemas"]["Approval"];

export function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

export function render(ui: ReactNode): RenderResult {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

export const emptyPage = {
  data: [],
  page: { next_cursor: null, has_more: false },
};

/** One request the screen made, as the route it names and what it carried. */
export type Call = { method: string; path: string; body: unknown };

export type Routes = Record<
  string,
  (body: unknown, query: URLSearchParams) => Response | Promise<Response>
>;

// Every read Brief fans out to, answered honestly by default so each case
// declares only the route it is about: a session, no nightly digest, no brief
// run.
//
// The table is exhaustive, and `stubApi` fails a case that asks for anything
// not in it. So a route leaves here when its caller does — the deals-by-stage
// report went with Brief's open-pipeline panel — and a read that arrives by
// accident is named rather than quietly served an empty page.
const DEFAULTS: Routes = {
  "GET /me": () => jsonResponse(meFixture()),
  "GET /brief": () => jsonResponse({ title: "Not Found" }, 404),
  "GET /digest": () =>
    jsonResponse({ title: "Not Found", code: "no_digest_yet" }, 404),
  // The Brief's strip, sentence and Do next section are all drawn from this ONE
  // answer, so an unrouted read has to reply with the real shape. The generic
  // empty page carries no `readings` and no `counts`, and a screen reading a
  // required field off it fails in a way no server could produce.
  "GET /worklist": () => jsonResponse(readingsDay({}, [])),
  "GET /worklist/handled": () =>
    jsonResponse({
      as_of: "2026-09-13T08:00:00Z",
      receipts: [],
      truncated: false,
    }),
  // The receipt at the foot of the page. Every lane, `totals` and both caveat
  // lists are required by the contract, so the generic empty page would fail
  // the panel in a way no server could produce — the same reason /worklist is
  // answered above. A QUIET receipt: nothing ran, and nothing was withheld.
  "GET /magic": () =>
    jsonResponse({
      as_of: "2026-09-13T08:00:00Z",
      since: "2026-09-12T08:00:00Z",
      done: [],
      needs_you: [],
      could_not_complete: [],
      watching: [],
      totals: { done: 0, needs_you: 0, could_not_complete: 0, watching: 0 },
      not_shown: [],
      sources_unavailable: [],
    }),
  // The plan panel reads `commitments` off this, which the contract marks
  // required. The generic empty page carries none, so an unrouted read would
  // fail the panel in a way no server could produce — the same reason
  // /worklist is answered above. 404 is the honest default: most screens under
  // test have not started a week.
  // The plan panel reads `commitments` off this, which the contract marks
  // required, so an unrouted read fails it in a way no server could produce —
  // the same reason /worklist is answered above.
  //
  // A STARTED-BUT-EMPTY week rather than a 404: the shape is what the panel
  // needs, and a 404 puts every screen that merely mounts the panel through an
  // error path, which moved the timing of nine tests in screens with nothing to
  // do with planning.
  "GET /weekly-plans/current": () =>
    jsonResponse({
      id: "00000000-0000-0000-0000-000000000001",
      local_week_start: "2026-06-08",
      status: "open",
      commitments: [],
    }),
  // The archive's index, as the weeks this rep has a review for. `weeks` is
  // what the hook reads; the generic empty page carries none.
  "GET /weekly-reviews": () => jsonResponse({ weeks: [] }),
  // 404 and not an empty body: a rep whose first Monday has not come round has
  // no review, and the hook reads that status as "no review yet" rather than as
  // a failure. An empty 200 would reach the branch that checks
  // `local_week_start` instead, which is the same answer by a path no server
  // takes.
  "GET /weekly-reviews/latest": () => jsonResponse({ title: "Not Found" }, 404),
  // Both answer `{ data, page }`, which the empty page already is — declared
  // anyway, because the point of this table is that every read Brief makes is
  // one somebody chose to answer. A read that arrives by accident and is served
  // the fallback is exactly what nothing could see before.
  "GET /users": () => jsonResponse(emptyPage),
  "GET /agent-tools": () => jsonResponse(emptyPage),
};

/**
 * Routes the stubbed fetch by method+path and RECORDS every call, because what
 * this screen must not do is as load-bearing as what it must: a staged verdict
 * is only staged if nothing was sent, and no rendering can prove that.
 */
export function stubApi(routes: Routes): Call[] {
  const calls: Call[] = [];
  const mock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const call = await readCall(input, init);
    calls.push(call);
    const route = `${call.method} ${call.path}`;
    const handler = routes[route] ?? DEFAULTS[route];
    if (!handler) {
      unrouted.push(route);
      return jsonResponse(emptyPage);
    }
    return handler(
      call.body,
      new URL(
        input instanceof Request ? input.url : String(input),
        "https://test.local",
      ).searchParams,
    );
  });
  vi.stubGlobal("fetch", mock);
  return calls;
}

// Routes asked for that neither the case nor DEFAULTS named.
//
// Answering one with a well-formed empty page is how a read nobody vetted stays
// invisible: no case can tell "Brief made no such request" from "Brief made one
// nobody stubbed", so a regression reintroducing a removed read leaves every
// case green. Collected rather than thrown from inside the stub — a throw there
// arrives as a query failure the screen renders as its empty state, which looks
// exactly the same.
let unrouted: string[] = [];

// Registered on import, so a suite gets this by using the harness rather than
// by remembering to ask. A per-suite opt-in is the version that decays: the
// twenty-first Brief suite is written by copying the twentieth, and whichever
// line was easiest to leave out is the one it leaves out.
afterEach(() => {
  const asked = unrouted;
  unrouted = [];
  expect(asked).toEqual([]);
});

/** One outbound request, read the two ways the client can have spelled it. */
async function readCall(
  input: RequestInfo | URL,
  init?: RequestInit,
): Promise<Call> {
  const request = input instanceof Request ? input : null;
  const url = new URL(
    request ? request.url : String(input),
    "https://test.local",
  );
  const method = request?.method ?? init?.method ?? "GET";
  return {
    method,
    path: url.pathname.replace(/^\/v1/, ""),
    body: method === "GET" ? null : await readBody(request, init),
  };
}

/**
 * What a write carried, or null.
 *
 * A write with no body at all (POST /brief) is not a malformed one, so a parse
 * failure is null rather than a throw.
 */
async function readBody(
  request: Request | null,
  init?: RequestInit,
): Promise<unknown> {
  try {
    // `clone()`: the client sends a Request and the route handler may read the
    // same body again.
    return request
      ? await request.clone().json()
      : JSON.parse(String(init?.body));
  } catch {
    return null;
  }
}

/**
 * Everything that left the browser as a write, in the order it went.
 *
 * `/reports/{report}` is excluded because it is a READ spelled as a POST — the
 * query plan does not fit a URL — and counting it would make "nothing was sent"
 * untrue of a page that sent nothing. No Brief surface runs one today; the
 * exclusion stays because what makes a report POST not a write is its shape,
 * which does not change with the caller.
 */
export function writes(calls: readonly Call[]): Call[] {
  return calls.filter(
    (call) => call.method !== "GET" && !call.path.startsWith("/reports/"),
  );
}

/** The routes those writes named, which is what a commit is judged on. */
export function writeRoutes(calls: readonly Call[]): string[] {
  return writes(calls).map((call) => `${call.method} ${call.path}`);
}

/** Brief's two work sections, in the order the document holds them. */
export function workOrder(): string[] {
  return [...document.querySelectorAll("#brief-decisions, #brief-today")].map(
    (section) => section.id,
  );
}

export const fleetDeal: Deal = {
  id: "d-1",
  name: "Fleet retrofit",
  amount_minor: 4_800_000,
  currency: "EUR",
  pipeline_id: "pl",
  stage_id: "s2",
  status: "open",
  stalled: false,
  source: "manual",
  captured_by: "human:u1",
  version: 1,
  created_at: "2026-05-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

export const run: MorningBrief = {
  id: "br-1",
  generated_at: "2026-07-05T05:30:00Z",
  as_of: "2026-07-05T05:00:00Z",
  candidate_count: 1,
  factors_omitted: [],
  items: [
    {
      id: "bi-1",
      deal_id: "d-1",
      rank: 1,
      composite: 0.74,
      feature_vector: {
        winnability: 0.4,
        revenue: 1,
        timing: 0.75,
        momentum: 1,
        warmth: 0.47,
      },
      evidence_ids: ["ev-1", "ev-2"],
      state: "new",
      state_at: null,
    },
  ],
};

/** One staged proposal, named by the sentence its card leads with. */
export function proposal(
  id: string,
  summary: string,
  over: Partial<Approval> = {},
) {
  const staged: Approval = {
    id,
    kind: "send_email",
    status: "pending",
    proposed_by: "agent:runner",
    summary,
    proposed_change: { body: "Hi — shall we sync next week?" },
    created_at: "2026-07-05T05:00:00Z",
    ...over,
  };
  return staged;
}

/** The pending queue, minus whatever the case has had decided. */
export function pendingPage(
  queue: readonly Approval[],
  decided: ReadonlySet<string>,
) {
  return jsonResponse({
    data: queue.filter((approval) => !decided.has(approval.id)),
    page: { next_cursor: null, has_more: false },
  });
}

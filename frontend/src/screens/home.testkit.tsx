import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { type RenderResult, render as rtlRender } from "@testing-library/react";
import type { ReactNode } from "react";
import { vi } from "vitest";
import type { components } from "../api/schema";
import { meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { readingsDay } from "./home.fixtures";
import type { Deal, MorningBrief } from "./home.queries";

// Home's suites share one harness, because they share one screen.
//
// Home fans out to several reads on mount, and what a case is ABOUT is one of
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
  (body: unknown) => Response | Promise<Response>
>;

// Every read Home fans out to, answered honestly by default so each case
// declares only the route it is about: a session, no nightly digest, no brief
// run, and a pipeline report with no rows. The report matters — the fallback
// empty PAGE carries no `rows`, which the pipeline reading would read as a
// failure and put a refusal in the rail of every case in this file.
const DEFAULTS: Routes = {
  "GET /me": () => jsonResponse(meFixture()),
  "GET /brief": () => jsonResponse({ title: "Not Found" }, 404),
  "GET /digest": () =>
    jsonResponse({ title: "Not Found", code: "no_digest_yet" }, 404),
  "POST /reports/deals-by-stage": () =>
    jsonResponse({ report: "deals-by-stage", plan: {}, columns: [], rows: [] }),
  // The Brief's strip, sentence and Do next section are all drawn from this ONE
  // answer, so an unrouted read has to reply with the real shape. The generic
  // empty page carries no `readings` and no `counts`, and a screen reading a
  // required field off it fails in a way no server could produce.
  "GET /worklist": () => jsonResponse(readingsDay({}, [])),
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
    return handler ? handler(call.body) : jsonResponse(emptyPage);
  });
  vi.stubGlobal("fetch", mock);
  return calls;
}

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
 * query plan does not fit a URL — and the rail runs one on every mount. Counting
 * it would make "nothing was sent" untrue of a page that sent nothing.
 */
export function writes(calls: readonly Call[]): Call[] {
  return calls.filter(
    (call) => call.method !== "GET" && !call.path.startsWith("/reports/"),
  );
}

/**
 * The `snoozed_until` a write carried, narrowed rather than asserted: a body is
 * `unknown` here because it came off the wire, and casting one into shape hides
 * the case where the field never went at all.
 */
export function readSnoozedUntil(body: unknown): string {
  if (
    typeof body !== "object" ||
    body === null ||
    !("snoozed_until" in body) ||
    typeof body.snoozed_until !== "string"
  ) {
    throw new Error(
      `the snooze write carried no instant: ${JSON.stringify(body)}`,
    );
  }
  return body.snoozed_until;
}

/** The routes those writes named, which is what a commit is judged on. */
export function writeRoutes(calls: readonly Call[]): string[] {
  return writes(calls).map((call) => `${call.method} ${call.path}`);
}

/** Home's two work sections, in the order the document holds them. */
export function workOrder(): string[] {
  return [...document.querySelectorAll("#home-decisions, #home-today")].map(
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

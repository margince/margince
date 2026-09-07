import type { paths } from "@composition/schema";
import createClient from "openapi-fetch";
import { beginModelCall, endModelCall } from "./model-inflight";

// The ONE API seam (architecture/01: the frontend depends on the generated
// contract, never Go internals). Types come from src/api/schema.d.ts —
// regenerate with `pnpm gen:api` after a crm.yaml change; never hand-edit.
//
// The specifier is the composition alias, not "./schema", and that is the whole
// of what makes an extension unit's screen able to use this client (ADR-0069).
// The VANILLA lane resolves it to src/api/schema.d.ts — the base contract — so
// `api.GET("/ext/notes")` is correctly a type error there: that
// installation does not serve the route. The COMPOSED lane
// (tsconfig.composed.json, `make fe-typecheck-composed`) resolves it to the
// types generated from the MERGED contract, where the route exists and is
// typed. One client, no wrapper, no cast, and the vanilla tree still refuses to
// compile a call it could not answer.
//
// It is a TYPE-ONLY import, so nothing changes at runtime and no bundler alias
// is needed: `verbatimModuleSyntax` erases the line entirely.
//
// One installation serves one organization (A107/ADR-0061): the server
// resolves its singleton organization itself — the client sends no tenant
// selector, only the session cookie.

// The reader's language, read from where the shell stores it. Sent on every
// request as Accept-Language so a server-side writer — the model-written
// briefs — answers in the language the reader is reading, rather than making
// them translate a summary they asked for. Storage can throw (private windows,
// blocked site data), and a client that cannot read a preference still has to
// make the call, so a failure is simply no header.
function readerLanguage(): string | undefined {
  try {
    return globalThis.localStorage?.getItem("margince.locale") ?? undefined;
  } catch {
    return undefined;
  }
}

// How long a request may stay open before this client stops waiting for it.
//
// This used to be derived from the API's 30s WriteTimeout — "no answer this
// client can legitimately receive takes longer than that". That stopped being
// true: the endpoints that call a model and wait (a reply draft, a dossier, a
// growth-fit read) run 13 to 45 seconds against a cloud provider, and the
// server now allows them the AI layer's own ceiling rather than cutting the
// response at 30s.
//
// So this deadline was cutting exactly the answers it was written to be
// generous towards, and the reader saw a request that failed for no stated
// reason. It sits above the server's own ceiling now, so the SERVER is what
// ends a hopeless request — it knows what the work was — and this remains what
// it was for: the request that opened and will never answer at all.
export const REQUEST_TIMEOUT_MS = 360_000;

/**
 * A request that opened and never answered.
 *
 * Its own type because a stall and a refusal are different facts and the code
 * above has to be able to tell them apart: a refusal arrives as a ProblemError
 * carrying what the server said, and this one carries the fact that the server
 * said nothing at all. It is deliberately NOT retried (app/queryclient.ts
 * retries only what the server reported as its own fault) — the surface whose
 * read failed offers the reader a retry, which is a person deciding to wait
 * again rather than this client deciding for them.
 */
export class RequestTimeoutError extends Error {
  constructor(method: string, url: string, timeoutMs: number) {
    super(`${method} ${url} did not answer within ${timeoutMs} ms`);
    this.name = "RequestTimeoutError";
  }
}

// A deadline on every request through this seam, because a request that opens
// and never answers is indistinguishable, to everything above, from one still
// arriving: `isPending` stays true, no error state is ever reached, and the
// authenticated shell — which holds every route on a splash until two reads
// settle (App.tsx) — leaves the reader with no error, no retry and no
// explanation. A proxy holding a connection open, or a laptop resuming into a
// dead socket, is enough to do it. A rejected fetch is a state every caller on
// this client already renders; an eternal pending one is a state nothing does.
//
// An AbortController on a timer rather than the shorter `AbortSignal.timeout`:
// that one counts on a clock inside the platform, which no test can advance, and
// a deadline nothing can exercise is a deadline nobody knows still works.
async function fetchWithDeadline(request: Request): Promise<Response> {
  const deadline = new AbortController();
  // The REQUEST's own signal is what React Query aborts when a screen unmounts
  // or a query is cancelled. Without this the deadline was the only way a call
  // could end, so navigating away from a 45-second draft left it running and a
  // manual retry ran a second model call beside the first.
  if (request.signal.aborted) {
    deadline.abort(request.signal.reason);
  } else {
    request.signal.addEventListener(
      "abort",
      () => deadline.abort(request.signal.reason),
      {
        once: true,
      },
    );
  }
  const expiry = globalThis.setTimeout(() => {
    deadline.abort(
      new RequestTimeoutError(request.method, request.url, REQUEST_TIMEOUT_MS),
    );
  }, REQUEST_TIMEOUT_MS);
  try {
    return withGatewayProblem(
      await globalThis.fetch(request, { signal: deadline.signal }),
      request,
    );
  } finally {
    // Whatever the outcome. A cleared timer is what keeps a settled request
    // from holding the page awake, and — on a request that failed for its own
    // reasons — from aborting a controller nobody is listening to any more.
    globalThis.clearTimeout(expiry);
  }
}

// The statuses a PROXY answers with when it gave up on the app behind it,
// rather than the app refusing something.
const GATEWAY_STATUSES = new Set([502, 503, 504]);

// The routes whose handler calls a model and waits, and whether they do so on
// every call.
//
// A MIRROR of the contract, never a second opinion: each entry is an operation
// `backend/api/crm.yaml` marks `x-waits-on-model`, spelled as the contract
// spells it, and `backend/gates/modelroutes_test.go` fails when the two
// disagree in either direction. Read the handler before you mark an operation
// there; the entry here then follows. The list used to be nine path suffixes
// checked for POST only, and it was one edit behind the contract for a long
// time in both directions: it named a route that calls a data provider and no
// model, and it missed the intro drafts, the role proposals, the onboarding
// conversation — and every GET that generates, which is how the meeting brief
// came to run two model calls with the chrome at rest.
//
// TWO readers want this set, for the same reason: duration. A model call runs
// for tens of seconds, so a proxy giving up on one really does leave work in
// flight and really does make a retry a second call rather than a repeat
// (`withGatewayProblem`), and a person who pressed the button really is waiting
// on the agent for that whole time (`model-inflight.ts`, which is what lights
// the AI-activity rail the instant the request leaves rather than at its next
// poll of the feed).
//
// `always` is a handler that generates on every call. `on-miss` is one that
// serves a stored reading and generates only when it has none: the dossier, the
// growth-fit band, the person brief, the deal status, the morning brief. Those
// answer from the store in well under a second and from the model in many, and
// the two are told apart by nothing the client can see at the moment the
// request leaves. So an `on-miss` call is counted as the agent working only
// once it has outlived CACHE_ANSWER_GRACE_MS — a stored answer never lights the
// orb, which is the reader's own click, and a generation does, a moment late.
//
// A route that ENQUEUES model work rather than waiting for it does not belong
// here in either reader's sense: it answers at once, so there is no long
// request for a proxy to cut and nothing for this tab to wait on, and its
// occurrence reaches the chrome the way every background run does, through the
// feed. The deep read, the document extraction, the account scan, the
// transcript proposals and the technical enrich are all that shape.
type ModelWait = "always" | "on-miss";

const MODEL_ROUTES: Readonly<Record<string, ModelWait>> = {
  "GET /activities/{id}/meeting-brief": "always",
  "GET /deals/{id}/status": "on-miss",
  "GET /organizations/{id}/brief": "on-miss",
  "GET /organizations/{id}/dossier": "on-miss",
  "GET /organizations/{id}/growth-fit": "on-miss",
  "GET /people/{id}/brief": "on-miss",
  "POST /activities/{id}/draft-email": "always",
  "POST /brief": "on-miss",
  "POST /coldstart": "always",
  "POST /coldstart/preview": "always",
  "POST /company/site-reads/{readId}/messages": "always",
  "POST /deals/{id}/role-proposals": "always",
  "POST /knowledge/corpora/{id}/ask": "always",
  "POST /leads/{id}/draft-email": "always",
  "POST /offers/{id}/regenerate": "always",
  "POST /onboarding/company/messages": "always",
  "POST /organizations/{id}/ask": "always",
  "POST /organizations/{id}/brief": "always",
  "POST /organizations/{id}/dossier": "always",
  "POST /organizations/{id}/draft-email": "always",
  "POST /organizations/{id}/enrich": "always",
  "POST /organizations/{id}/growth-fit": "always",
  "POST /organizations/{id}/intro-request-draft": "always",
  "POST /people/{id}/brief": "always",
  "POST /people/{id}/draft-email": "always",
  "POST /people/{id}/intro-note-draft": "always",
};

// How long an `on-miss` request may stay open before it counts as the agent
// working.
//
// Above what a stored reading costs to serve — a row read and a response, a
// few hundred milliseconds at the far end of a slow link — and well below the
// shortest model call a cloud provider answers. A cached answer therefore never
// lights the orb, and a generation lights it a second late rather than not at
// all, which was the alternative: a first open of a company dossier ran twenty
// seconds with the chrome reporting an agent at rest.
export const CACHE_ANSWER_GRACE_MS = 1_000;

// One compiled matcher per contract path, built once from the table: a path
// parameter matches one segment, and the template is anchored at the END so
// the client's `/v1` mount — or any other prefix a deployment serves under —
// needs no spelling here.
const MODEL_ROUTE_MATCHERS: readonly (readonly [
  method: string,
  path: RegExp,
  wait: ModelWait,
])[] = Object.entries(MODEL_ROUTES).map(([route, wait]) => {
  const [method, template] = route.split(" ", 2);
  const pattern = template
    .split("/")
    .map((segment) =>
      /^\{[^}]+\}$/.test(segment)
        ? "[^/]+"
        : segment.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"),
    )
    .join("/");
  return [method, new RegExp(`${pattern}$`), wait] as const;
});

/**
 * Give a proxy's failure ON A SLOW AI ROUTE a problem body, so the reader is
 * told what happened instead of "the request failed".
 *
 * These responses come from the PROXY, not the app, so they carry no RFC 7807
 * body — `problemDetail` finds no code and the caller falls back to its own
 * generic sentence. That is how a reply draft that ran 45 seconds and had its
 * connection cut reached the screen as "The request failed. Please try again."
 *
 * Identified by CONTENT TYPE, not an empty body: nginx and Vite both answer
 * with an HTML error page, so a check for "no body at all" matches almost none
 * of the real cases. Anything the app itself answered is problem+json and
 * passes through untouched — the server's own sentence is always better.
 */
function withGatewayProblem(response: Response, request: Request): Response {
  if (!GATEWAY_STATUSES.has(response.status) || modelWaitOf(request) === null) {
    return response;
  }
  const contentType = response.headers.get("Content-Type") ?? "";
  if (contentType.includes("application/problem+json")) {
    return response;
  }
  return new Response(
    JSON.stringify({
      type: "about:blank",
      title: "Gateway",
      status: response.status,
      code: "gateway_unavailable",
      detail: "the server did not finish this request",
    }),
    {
      status: response.status,
      statusText: response.statusText,
      headers: { "Content-Type": "application/problem+json" },
    },
  );
}

// Whether THIS request holds a model call open, and on which terms: by method
// AND path, because the dossier is read and refreshed at one path and only the
// refresh is the agent at work on every call. Null for every other request —
// the reader's own click, which the rail must not narrate in the agent's voice.
function modelWaitOf(request: Request): ModelWait | null {
  const path = URL.canParse(request.url)
    ? new URL(request.url).pathname
    : request.url;
  for (const [method, pattern, wait] of MODEL_ROUTE_MATCHERS) {
    if (request.method === method && pattern.test(path)) {
      return wait;
    }
  }
  return null;
}

// A request counted as the agent working for as long as it is open, from the
// moment it leaves.
function fetchCountedAtOnce(request: Request): Promise<Response> {
  beginModelCall();
  return fetchWithDeadline(request).finally(endModelCall);
}

// A request counted as the agent working only once it has outlived the time a
// stored answer takes — see CACHE_ANSWER_GRACE_MS. Ended in the same `finally`
// as the eager count, so a refusal and a stall release it as surely as an
// answer does, and a request that answered inside the grace releases nothing
// because it counted nothing.
function fetchCountedAfterGrace(request: Request): Promise<Response> {
  let counted = false;
  const grace = globalThis.setTimeout(() => {
    counted = true;
    beginModelCall();
  }, CACHE_ANSWER_GRACE_MS);
  return fetchWithDeadline(request).finally(() => {
    globalThis.clearTimeout(grace);
    if (counted) {
      endModelCall();
    }
  });
}

export const api = createClient<paths>({
  // same-origin absolute base + the /v1 mount: contract paths are
  // unprefixed, the server serves them under /v1 (same as curl :8080/v1/me)
  baseUrl:
    typeof globalThis.window === "undefined"
      ? "http://localhost/v1"
      : `${globalThis.location.origin}/v1`,
  credentials: "include",
  // resolve the CURRENT global fetch per call (test stubs, SW interception)
  fetch: (request) => {
    const language = readerLanguage();
    if (language && !request.headers.has("Accept-Language")) {
      request.headers.set("Accept-Language", language);
    }
    // The chrome learns the agent is working HERE, where it is already known,
    // rather than on the next poll of the activity feed seconds later.
    switch (modelWaitOf(request)) {
      case "always":
        return fetchCountedAtOnce(request);
      case "on-miss":
        return fetchCountedAfterGrace(request);
      case null:
        return fetchWithDeadline(request);
    }
  },
});

// The cursor a first page asks with: none. Named and typed once, because
// `initialPageParam: null` on its own narrows the page param to `null` and then
// rejects the string cursors every page after the first carries — so each call
// site that spelled the plain literal had to assert its way out, and seven of
// them did. A keyset walk is an API shape, so the value it starts from lives
// beside the client rather than being reinvented per screen.
export const FIRST_PAGE: string | null = null;

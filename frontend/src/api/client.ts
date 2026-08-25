import type { paths } from "@composition/schema";
import createClient from "openapi-fetch";

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
// The API's own http.Server carries a 30s WriteTimeout, so no answer this
// client can legitimately receive takes longer than that; the rest is headroom
// for whatever sits between the two. Nothing long-lived comes through here —
// the polling screens issue ordinary short requests on a refetchInterval, and
// the file uploads call `fetch` directly rather than this client — so there is
// no request this deadline can cut off mid-answer.
const REQUEST_TIMEOUT_MS = 45_000;

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
  const expiry = globalThis.setTimeout(() => {
    deadline.abort(
      new RequestTimeoutError(request.method, request.url, REQUEST_TIMEOUT_MS),
    );
  }, REQUEST_TIMEOUT_MS);
  try {
    return await globalThis.fetch(request, { signal: deadline.signal });
  } finally {
    // Whatever the outcome. A cleared timer is what keeps a settled request
    // from holding the page awake, and — on a request that failed for its own
    // reasons — from aborting a controller nobody is listening to any more.
    globalThis.clearTimeout(expiry);
  }
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
    return fetchWithDeadline(request);
  },
});

// The cursor a first page asks with: none. Named and typed once, because
// `initialPageParam: null` on its own narrows the page param to `null` and then
// rejects the string cursors every page after the first carries — so each call
// site that spelled the plain literal had to assert its way out, and seven of
// them did. A keyset walk is an API shape, so the value it starts from lives
// beside the client rather than being reinvented per screen.
export const FIRST_PAGE: string | null = null;

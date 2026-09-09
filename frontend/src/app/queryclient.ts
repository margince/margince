import {
  MutationCache,
  QueryCache,
  QueryClient,
  type QueryKey,
} from "@tanstack/react-query";
import { isRecordRead } from "../screens/activitykeys";
import { logUnexpectedError, ProblemError } from "../screens/common";
import { ENTITY_NAME_KEY } from "../screens/entityref";

// The data layer's parameters (architecture/frontend, FE-PARAM-1..5). The
// library's defaults are not this product's: they hold nothing back from the
// network, retry a refusal the server has already made final, and drop every
// failure on the floor. Each value below is chosen, and the ones the reader
// can feel are pinned by the tests next to this file.

// FE-PARAM-1. A query serves its cached answer for this long before a mount
// refetches in the background. A surface that needs a different window sets
// its own (the /me probe holds five minutes and refetches on focus, because a
// grant change must not sit behind a stale snapshot).
const STALE_TIME_MS = 30_000;

// FE-PARAM-5. How often the record a reader has OPEN re-reads itself.
//
// Work reaches a record from places the tab cannot see: an agent files a task,
// a colleague answers a mail, a promise falls due while the page is on screen.
// Read once on arrival, the page keeps answering from that instant — and the
// failure is silent, because a stale "what needs you" looks exactly like a
// current one.
//
// A MINUTE, and the number is a cost decision rather than a feel one. The
// cadence buys exactly one case: a reader sitting on a record while it changes
// under them. Every other way a record goes stale — coming back to the tab, a
// write this app made — is already answered, the first by the focus refetch
// below and the second by the mutation cache's invalidations, and neither
// costs a request while nothing is happening.
//
// What a tick costs is a whole composite assembly: the 360 endpoints carry no
// ETag, so a poll that finds nothing new is priced the same as one that finds
// work, and the assembly runs to a query budget the composite is designed
// around. Against that, the difference between hearing about a colleague's
// task in twenty seconds and in sixty is not worth three times the reads.
//
// It is a cadence and not a stream: the contract serves no push, so the reader
// sees new work within one cadence rather than the moment it lands. A shorter
// one is worth revisiting when a 304 makes an unchanged read cheap, or against
// a measured p95 for the assembly — neither of which exists today.
const LIVE_RECORD_MS = 60_000;

// FE-PARAM-5, applied: a read is live because of WHAT IT IS, not because the
// screen that mounts it remembered to ask.
//
// The alternative was per-hook options, and it fails the way lists fail: the
// fifth record page is written without them, nothing says so, and the page
// serves a stale answer that looks exactly like a fresh one. Keyed on the read
// instead, `isRecordRead` derives its corpus from the table that already knows
// which key carries a record — so a record kind joins by existing rather than
// by being remembered here.
//
// It reaches every surface showing that record and not only its page: the
// composer anchored on a contact and the worklist's record pane read under the
// same key, so they see what the page sees, share its one interval, and stop
// it when the last of them unmounts. What it deliberately does NOT reach is a
// read whose answer a model writes — the deal briefing is rewritten
// server-side whenever the deal has moved, and a cadence on that would spend
// the workspace's AI budget on an open tab. Its key is the status card's, not
// the deal record's, which is what keeps it out.
function liveInterval(query: { queryKey: QueryKey }): number | false {
  return isRecordRead(query.queryKey) && LIVE_RECORD_MS;
}

// FE-PARAM-2. Two retries, and only for a failure the server reported as its
// own fault.
const MAX_RETRIES = 2;

// The 5xx statuses that are SETTLED rather than failed, and so are not retried.
//
// The 4xx/5xx split is the wrong seam for these two. Both are classed as server
// errors and neither says the server failed trying: 501 says it does not
// support what was asked, and 505 that it will not speak this version of the
// protocol. Nothing about asking again changes either within a session, so a
// retry buys three requests and three console errors where one would do — and
// an operator reading the console for a real fault reads those first.
//
// 501 is not hypothetical here. httperr has a generated 501 path precisely so
// a surface the contract specifies and this build does not implement refuses
// cleanly instead of 500ing, which is a state this product reaches on purpose:
// an installation with no embeddings model bound answers 501 to the reindex
// status, every time it is asked.
const SETTLED_SERVER_REFUSALS = new Set([501, 505]);

// The RFC-7807 `status` a failure carries, or null when it carries none. Only
// a ProblemError holds a server problem body, on the same terms as
// problemCodeOf: a rejected fetch, or a query function that threw a plain
// Error, never claims a status the server did not send.
function problemStatusOf(error: unknown): number | null {
  if (!(error instanceof ProblemError)) {
    return null;
  }
  const problem = error.problem;
  if (typeof problem !== "object" || problem === null || !("status" in problem))
    return null;
  return typeof problem.status === "number" ? problem.status : null;
}

// The RFC-7807 `code` a failure carries, on the same terms as the status above.
function problemCodeOf(error: unknown): string | null {
  if (!(error instanceof ProblemError)) {
    return null;
  }
  const problem = error.problem;
  if (typeof problem !== "object" || problem === null || !("code" in problem)) {
    return null;
  }
  return typeof problem.code === "string" ? problem.code : null;
}

// FE-PARAM-2: retry a server error that the server may yet recover from —
// never a client error, and never a refusal it has already settled.
export function retryQuery(failureCount: number, error: Error): boolean {
  const status = problemStatusOf(error);
  // A failure that carries no status is NOT retried, and that stays true now
  // that a server refusal always arrives as a ProblemError: what is left
  // without a status is a failure that never reached the server (a rejected
  // fetch) or one raised inside the query function itself. Neither is a fault
  // the server reported, which is the only thing FE-PARAM-2 retries — and a
  // bug in a query function returns the same failure however often it is
  // asked. The error state that follows offers the reader a retry either way,
  // so nothing is lost but the silent second request.
  // A gateway that gave up is NOT retried, and this is the one 5xx where a
  // second request is worse than none: the proxy stopped waiting, but the work
  // behind it — a model call that legitimately runs the better part of a
  // minute — is very likely still running. Retrying twice more starts up to
  // three of them, and the reader is told in the same breath that the first
  // may still be working. Their own retry is a decision; this one is not.
  const gaveUpWaiting = problemCodeOf(error) === "gateway_unavailable";
  return (
    status !== null &&
    status >= 500 &&
    !gaveUpWaiting &&
    !SETTLED_SERVER_REFUSALS.has(status) &&
    failureCount < MAX_RETRIES
  );
}

// FE-PARAM-4: the ONE place a query failure is reported. This installation
// has no telemetry sink, so reporting means the browser console — where an
// operator can read it and the reader never sees it. It must stay that way:
// the surface whose query failed renders its own error state, and a second,
// global one would talk over it.
function reportQueryError(error: Error): void {
  console.error("margince: query failed", error);
}

// A write can rename the record it touches, and the chrome around the reader is
// naming that record: the trail at the top of the window and every reference
// chip read the name on their own key, with a freshness window measured in
// minutes, so nothing a screen invalidates brings them back. A company renamed
// on its own page kept the old name in the trail until the reader reloaded.
//
// Invalidated for every successful mutation rather than beside each rename,
// because "which writes can change a display name" is a list — twelve PATCH
// sites today — and the thirteenth would be written without it. Only MOUNTED
// reads refetch, and the chrome holds one per named reference on screen.
function refreshNamedReferences(client: QueryClient): void {
  client.invalidateQueries({
    predicate: (query) => query.queryKey[1] === ENTITY_NAME_KEY,
  });
}

// A record's history is a read of what has just been written to it, so ANY
// successful write makes the open history stale — including one made from
// another panel on the same page, and including a restore, whose whole purpose
// is to add a line to the list the reader is looking at.
//
// Invalidated for every successful mutation, for the same reason the named
// references are: "which writes change a record's history" is every write, and
// a list of them is a list the next one written is left off. Only MOUNTED
// reads refetch, so a reader with no history panel open pays nothing.
function refreshRecordHistory(client: QueryClient): void {
  client.invalidateQueries({
    predicate: (query) =>
      query.queryKey[0] === "record-history" ||
      query.queryKey[0] === "field-history",
  });
}

// Built per call rather than exported as a module singleton so the policy can
// be exercised without importing main.tsx, which mounts the application into
// the document as a side effect of being imported.
export function createQueryClient(): QueryClient {
  const client: QueryClient = new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: STALE_TIME_MS,
        retry: retryQuery,
        // FE-PARAM-5. A record on screen re-reads itself; everything else is
        // answered from cache exactly as before.
        refetchInterval: liveInterval,
        // The library's default, restated because it is the half that keeps a
        // tab forgotten on a second monitor from re-reading all night: the
        // interval runs only while the window has focus.
        refetchIntervalInBackground: false,
        // FE-PARAM-3. Returning to the tab refetches nothing by default; a
        // query whose freshness matters opts in for itself. A record does:
        // coming back to one after an hour away is exactly when the cached
        // answer is wrong, and exactly when the reader acts on it.
        refetchOnWindowFocus: (query) => isRecordRead(query.queryKey),
      },
    },
    queryCache: new QueryCache({ onError: reportQueryError }),
    // FE-PARAM-4 for the other half of the data layer. A mutation has no
    // shared cache entry a screen sits on, so nothing observes its failure
    // unless something is wired to — and wiring that per mutation is how the
    // next one written loses it: the failure the reader is shown as one
    // generic sentence would then exist nowhere at all.
    //
    // logUnexpectedError rather than the unconditional report the query half
    // makes: a server problem is already on the screen in the reader's own
    // words, and a refusal a form renders field by field is a routine answer
    // to a mutation, not a fault anyone should open a console for.
    mutationCache: new MutationCache({
      onError: logUnexpectedError,
      onSuccess: () => {
        refreshNamedReferences(client);
        refreshRecordHistory(client);
      },
    }),
  });
  return client;
}

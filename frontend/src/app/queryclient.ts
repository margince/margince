import { MutationCache, QueryCache, QueryClient } from "@tanstack/react-query";
import { logUnexpectedError, ProblemError } from "../screens/common";
import { ENTITY_NAME_KEY } from "../screens/entityref";

// The data layer's parameters (architecture/frontend, FE-PARAM-1..4). The
// library's defaults are not this product's: they hold nothing back from the
// network, retry a refusal the server has already made final, and drop every
// failure on the floor. Each value below is chosen, and the ones the reader
// can feel are pinned by the tests next to this file.

// FE-PARAM-1. A query serves its cached answer for this long before a mount
// refetches in the background. A surface that needs a different window sets
// its own (the /me probe holds five minutes and refetches on focus, because a
// grant change must not sit behind a stale snapshot).
const STALE_TIME_MS = 30_000;

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
        // FE-PARAM-3. Returning to the tab refetches nothing by default; a
        // query whose freshness matters opts in for itself.
        refetchOnWindowFocus: false,
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

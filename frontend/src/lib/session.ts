import { AuthProbeError, readSession } from "./session.read";

export {
  AuthProbeError,
  type AuthProbeKind,
  readSession,
} from "./session.read";

import {
  type QueryClient,
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import type { Middleware } from "openapi-fetch";
import { useEffect, useRef } from "react";
import { api } from "../api/client";
import { healthScope } from "./healthscopekey";
import { endIdBatches } from "./idbatch";
import { unwrap } from "./problem";
import { forgetSeat } from "./storage";

// authExitNotice marks a chosen sign-out so the boundary's next 401 reads
// as "signed out", not "session expired". Module-scoped: one boundary
// consumes it.
let authExitNotice: "signed-out" | null = null;

export function consumeAuthExitNotice(): "signed-out" | null {
  const notice = authExitNotice;
  authExitNotice = null;
  return notice;
}

// The session principal (GET /v1/me): identity + effective role keys. One
// spelling, one ["me"] cache entry: the App auth gate, the settings identity
// card, and role-aware affordances all read the same probe. The server binds
// the installation's singleton company itself, so the probe needs nothing but
// the session cookie.
export const SESSION_STALE_MS = 5 * 60_000;

export const sessionOptions = queryOptions({
  queryKey: ["me"],
  staleTime: SESSION_STALE_MS,
  // A role change does not revoke live sessions, and the UI scopes
  // affordances by the grants this snapshot carries. So it must not sit stale
  // for its full staleTime. "always", not `true`: `true`
  // refetches on focus only once the entry is stale, and that five-minute
  // window is the one this needs to close.
  refetchOnWindowFocus: "always",
  retry: false,
  // A refused probe is asked again by sign-in or Retry. A mount after the
  // startup prefetch was refused would only repeat the refusal.
  retryOnMount: false,
  queryFn: ({ signal }) => readSession(signal),
});

export function useMe(enabled = true) {
  const client = useQueryClient();
  const query = useQuery({ ...sessionOptions, enabled });
  const scope = healthScope(query.data);
  // The last signed-in principal, kept across a signed-out gap: an expired
  // session that ends in another seat's sign-in reuses this QueryClient.
  const previousScope = useRef(scope);
  const previousUser = useRef(query.data?.user?.id);
  useEffect(() => {
    if (scope === undefined) return;
    const previous = previousScope.current;
    const user = query.data?.user?.id;
    if (previousUser.current !== undefined && previousUser.current !== user)
      // Most record caches are keyed by record, not by reader. Only the
      // snapshot itself (the bare ["me"] key) survives; ["me", ...] reads such
      // as the reader's own sessions and two-step state belong to the seat.
      client.removeQueries({
        predicate: (entry) =>
          entry.queryKey[0] !== "me" || entry.queryKey.length > 1,
      });
    else if (previous !== undefined && previous !== scope)
      client.removeQueries({
        predicate: (entry) =>
          entry.queryKey.includes(previous) ||
          entry.queryKey[0] === "reader-rendering" ||
          entry.queryKey[0] === "scheduling-profile",
      });
    previousScope.current = scope;
    previousUser.current = user;
  }, [client, scope, query.data?.user?.id]);
  return query;
}

/** True when /me gives the boundary any answer but the connection screen. */
export function sessionAnswers(): Promise<boolean> {
  return readSession().then(
    () => true,
    (error: unknown) =>
      error instanceof AuthProbeError && error.kind !== "connection",
  );
}

// Order matters, and it is why this is one function rather than four
// spellings. queryClient.clear() destroys every Query in the cache, ["me"]
// included. A reset after clear() finds no ["me"] to reset, and clear() alone
// never refetches, so AuthGate keeps rendering its last authenticated snapshot.
// So drop every other entry first, then reset ["me"]. That query still has a
// mounted observer, and resetQueries refetches it at once: 401, then login.
//
// Every caller learns the session is over: the chosen sign-out, or a 401 a
// long-lived read finds. A cached answer must not outlive its member, or the
// next contact to sign in within the cache lifetime would be served it.
export function resetToSignedOut(queryClient: QueryClient): Promise<void> {
  // The next member must not inherit what this one stored in the browser.
  forgetSeat();
  // A name batch still reading must not ask its next page as the next member.
  endIdBatches();
  queryClient.removeQueries({
    predicate: (query) => query.queryKey[0] !== "me",
  });
  return queryClient.resetQueries({ queryKey: ["me"] });
}

export function useLogout() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async () => {
      unwrap(await api.POST("/auth/logout"));
    },
    onSuccess: async () => {
      // The next 401 at the boundary is this chosen exit, not an expired
      // session, and the login screen greets it accordingly.
      authExitNotice = "signed-out";
      await resetToSignedOut(queryClient);
    },
  });
}

// The reader's own user id, for the provenance tags on this screen. Undefined
// while /me is in flight. The tags then read "a contact, not provably you",
// the only claim they can make until the session is known.
export function useViewerId(enabled = true): string | undefined {
  return useMe(enabled).data?.user.id;
}

// Whether the probe's last refusal already says what `status` would: sign-in
// answers every refusal, a lock answers a 403 but not a lapse.
function refusalKnown(queryClient: QueryClient, status: number): boolean {
  const { error } = queryClient.getQueryState(sessionOptions.queryKey) ?? {};
  if (!(error instanceof AuthProbeError)) return false;
  if (error.kind === "unauthorized") return true;
  return (
    status === 403 &&
    (error.kind === "must-change-password" || error.kind === "must-enrol-mfa")
  );
}

// A 403 means the session snapshot is stale: grants changed, or the account is
// now confined. A 401 may be a wrong code or a lapsed session, and the contract
// gives both one answer. So /me is asked again at once, not at the next focus.
export function sessionRefusalWatch(queryClient: QueryClient): Middleware {
  return {
    onResponse({ request, response }) {
      if (
        (response.status === 401 || response.status === 403) &&
        !new URL(request.url).pathname.endsWith("/me") &&
        !refusalKnown(queryClient, response.status)
      ) {
        // Exact, and never cancelling: a prefix match re-asks the ["me", …] read
        // that just 403'd, and a cancel discards the probe's own answer.
        void queryClient.invalidateQueries(
          { queryKey: ["me"], exact: true },
          { cancelRefetch: false },
        );
      }
      return response;
    },
  };
}

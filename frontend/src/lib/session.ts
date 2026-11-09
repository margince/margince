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
  queryFn: ({ signal }) => readSession(signal),
});

export function useMe(enabled = true) {
  const client = useQueryClient();
  const query = useQuery({ ...sessionOptions, enabled });
  const scope = healthScope(query.data);
  const previousScope = useRef(scope);
  useEffect(() => {
    const previous = previousScope.current;
    if (previous !== undefined && previous !== scope)
      client.removeQueries({
        predicate: (entry) =>
          entry.queryKey.includes(previous) ||
          entry.queryKey[0] === "reader-rendering",
      });
    previousScope.current = scope;
  }, [client, scope]);
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

// A 403 means the session snapshot is stale: grants changed, or the account is
// now confined. A 401 may be a wrong code or a lapsed session, and the contract
// gives both one answer. So /me is asked again at once, not at the next focus.
export function sessionRefusalWatch(queryClient: QueryClient): Middleware {
  return {
    onResponse({ request, response }) {
      if (
        (response.status === 401 || response.status === 403) &&
        !new URL(request.url).pathname.endsWith("/me")
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

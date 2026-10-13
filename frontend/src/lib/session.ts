import {
  type QueryClient,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { api } from "../api/client";
import { problemMessage, unwrap } from "./problem";
import { forgetSeat } from "./storage";

// Authentication and availability are different product states: a failed
// session probe is typed so the auth boundary can render login (401), the
// connection-problem screen (network/5xx), or the installation-unavailable
// screen (503 — pre-bootstrap or a violated singleton invariant) instead of
// collapsing every error into login.
export type AuthProbeKind =
  | "unauthorized"
  | "connection"
  | "installation"
  // The account is authenticated but still using a password its operator
  // chose, and the server refuses every route but the one that replaces it.
  // A distinct kind because the remedy is distinct: sending this to the login
  // screen would loop — the credentials are correct, and using them again
  // lands in the same refusal.
  | "must-change-password";

export class AuthProbeError extends Error {
  readonly kind: AuthProbeKind;
  constructor(kind: AuthProbeKind, message: string) {
    super(message);
    this.name = "AuthProbeError";
    this.kind = kind;
  }
}

// probeKindFor maps a /me response status onto the boundary state. 503 is
// the middleware's installation-not-ready answer; any other 5xx (or a
// rejected fetch) is a connectivity problem; everything else reads as "no
// session" — the login screen.
function probeKindFor(status: number, code?: string): AuthProbeKind {
  if (status === 503) return "installation";
  if (status >= 500) return "connection";
  if (status === 403 && code === "password_change_required") {
    return "must-change-password";
  }
  return "unauthorized";
}

// codeInProblemBody reads the machine code straight off a problem BODY. The
// exported problemCodeOf below takes a thrown ProblemError instead; here the
// probe still holds the decoded body, and the code is what separates a refusal
// the caller can act on from one they cannot.
function codeInProblemBody(error: unknown): string | undefined {
  if (typeof error === "object" && error !== null && "code" in error) {
    const { code } = error as { code?: unknown };
    return typeof code === "string" ? code : undefined;
  }
  return undefined;
}

// authExitNotice marks a DELIBERATE sign-out so the boundary's next 401
// reads as "signed out", not "session expired". Module-scoped: exactly one
// boundary consumes it.
let authExitNotice: "signed-out" | null = null;

export function consumeAuthExitNotice(): "signed-out" | null {
  const notice = authExitNotice;
  authExitNotice = null;
  return notice;
}

// The session principal (GET /v1/me): identity + effective role keys. One
// spelling, one ["me"] cache entry — the App auth gate, the settings identity
// card, and role-aware affordances all read the same probe. The server binds
// the installation's singleton company itself (ADR-0061) — the
// probe needs nothing but the session cookie.
export function useMe(enabled = true) {
  return useQuery({
    queryKey: ["me"],
    enabled,
    staleTime: 5 * 60_000,
    // A role change does not revoke live sessions, so this snapshot is the one
    // cache entry that must not sit stale for its full staleTime: the UI now
    // scopes affordances by the grants it carries. "always" rather than `true`
    // deliberately — `true` refetches on focus only once the entry is already
    // stale, which is exactly the five-minute window this needs to close.
    refetchOnWindowFocus: "always",
    retry: false,
    queryFn: readSession,
  });
}

// The boundary's one reading of /me; the connection screen's check shares it.
async function readSession() {
  const result = await api.GET("/me").catch(() => null);
  if (!result) {
    throw new AuthProbeError("connection", "the API could not be reached");
  }
  const { data, error, response } = result;
  if (error) {
    throw new AuthProbeError(
      probeKindFor(response.status, codeInProblemBody(error)),
      problemMessage(error),
    );
  }
  if (!data?.user) {
    // Without the user the contract requires it is no session, whatever the
    // status said: a server answering garbage is an availability problem.
    throw new AuthProbeError("connection", "malformed /me response");
  }
  return data;
}

/** True when /me gives the boundary any answer but the connection screen. */
export function sessionAnswers(): Promise<boolean> {
  return readSession().then(
    () => true,
    (error: unknown) =>
      error instanceof AuthProbeError && error.kind !== "connection",
  );
}

// ORDER MATTERS, and it is the whole reason this is one function rather than
// four spellings. queryClient.clear() destroys every Query object in the cache,
// INCLUDING ["me"]'s. If ["me"] were reset only after a full clear(),
// resetQueries would find nothing matching that key to reset — the mounted
// AuthGate observer would keep rendering its last authenticated snapshot, since
// clear() alone never triggers a refetch. So: drop every OTHER entry first,
// leaving ["me"] intact, then reset ["me"] specifically. That query still
// exists, still has a mounted observer, and resetQueries forces it to refetch
// immediately, landing the boundary on 401 → login.
//
// Every caller is a place that learns the session is over: the deliberate
// sign-out, and the 401 a long-lived read discovers for itself. What must not
// happen is a cached answer outliving the member it was fetched for — the next
// contact to sign in inside the cache lifetime would be served it.
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
      // The next 401 at the boundary is this deliberate exit, not an
      // expired session — the login screen greets it accordingly.
      authExitNotice = "signed-out";
      await resetToSignedOut(queryClient);
    },
  });
}

// The reader's own user id, for the provenance tags on this screen. Undefined
// while /me is in flight, which the tags read as "a contact, not provably you"
// — the honest reading until the session is known.
export function useViewerId(enabled = true): string | undefined {
  return useMe(enabled).data?.user.id;
}

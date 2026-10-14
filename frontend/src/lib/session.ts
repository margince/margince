import {
  type QueryClient,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { api } from "../api/client";
import { problemMessage, unwrap } from "./problem";
import { forgetSeat } from "./storage";

// Authentication and availability are different product states. A failed
// session probe is typed, so the auth boundary can pick its screen. It shows
// login (401), the connection problem (network/5xx), or installation
// unavailable (503, pre-bootstrap or a broken singleton). Not every error is login.
export type AuthProbeKind =
  | "unauthorized"
  | "connection"
  | "installation"
  // The account is authenticated but still using a password its operator
  // chose, and the server refuses every route but the one that replaces it.
  // A distinct kind because the remedy is distinct. The login screen would
  // loop: the credentials are correct, and using them again lands in the
  // same refusal.
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
// session", the login screen.
function probeKindFor(status: number, code?: string): AuthProbeKind {
  if (status === 503) return "installation";
  if (status >= 500) return "connection";
  if (status === 403 && code === "password_change_required") {
    return "must-change-password";
  }
  return "unauthorized";
}

// codeInProblemBody reads the machine code straight off a problem body. The
// exported problemCodeOf (lib/problem.ts) takes a thrown ProblemError instead.
// Here the probe still holds the decoded body. The code separates a refusal
// the caller can act on from one they cannot.
function codeInProblemBody(error: unknown): string | undefined {
  if (typeof error === "object" && error !== null && "code" in error) {
    const { code } = error as { code?: unknown };
    return typeof code === "string" ? code : undefined;
  }
  return undefined;
}

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
export function useMe(enabled = true) {
  return useQuery({
    queryKey: ["me"],
    enabled,
    staleTime: 5 * 60_000,
    // A role change does not revoke live sessions, and the UI scopes
    // affordances by the grants this snapshot carries. So it must not sit stale
    // for its full staleTime. "always", not `true`: `true`
    // refetches on focus only once the entry is stale, and that five-minute
    // window is the one this needs to close.
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

import type { MessageKey } from "../i18n/en";
import { ConnectivityError, writeFailureKey } from "./connectivity";
import {
  type FieldProblem,
  isRecord,
  problemCode,
  problemFieldErrors,
} from "./problembody";
import { PROBLEM_CODE_COPY } from "./problemcodecopy";

export type { FieldProblem } from "./problembody";
export {
  problemCode,
  problemExistingId,
  problemFieldErrors,
} from "./problembody";

// RFC 7807 bodies carry the honest detail; surface it instead of a generic
// failure so the error state names its cause. `null` says the body carried no
// such text at all — a non-OK response the server sent no body with, or one
// whose body was never RFC 7807 in the first place.
//
// That distinction is the whole reason this sits apart from problemMessage
// below: "the server described this failure" and "the server said nothing a
// reader can use" are different facts, and only the second may be answered
// with catalog copy. A caller that cannot tell them apart either invents copy
// over a real detail or shows a placeholder as though the server had spoken.
//
// A refusal is the OPPOSITE case: `permission_denied` is one code over two
// authorities — an object-RBAC denial (this role does not admit the action on
// this kind of record) and a row-authority denial (the record is on screen
// through a read share) — and nothing on the wire says which of the two
// happened. So the catalog copy replaces the server's detail and deliberately
// does not guess: copy that named the wrong authority would be worse than the
// bare sentinel.
//
// Replacing it loses nothing a reader wanted, and that is the part worth
// knowing before anyone tries to "keep the more specific answer". `httperr`
// builds a refusal's detail from `err.Error()`, and every producer of this
// sentinel wraps it with INTERNALS: `auth.Require` sends the RBAC object and
// verb ("contact.update: permission denied"), the admission gate sends its own
// spec name and resolver state. None of that is copy, and showing it would
// leak the shape of the authority model to a client. There is no path on which
// the server sends a sentence written for a reader here.
//
// `seat_tier_insufficient` is the LICENSING ceiling, and it is a different
// refusal from the two above: it is decided before any role is consulted, so a
// reader whose role admits the action is refused anyway. Its server detail is
// the bare sentinel ("seat tier insufficient"), which names a concept no
// reader has met and offers nothing to do about it, so the catalog copy
// replaces it and points at the one contact who can lift the ceiling. It names
// the SEAT rather than "your seat": the same code answers a read seat's own
// mutation, an agent passport acting for one, and a grant that would give a
// read seat write access. A surface that knows WHOSE seat it is (share.tsx
// knows it is the recipient's) says so in its own words before it gets here.
function problemDetail(
  problem: unknown,
  t?: (key: MessageKey) => string,
): string | null {
  const code = problemCode(problem);
  const copyKey = PROBLEM_CODE_COPY.get(code ?? "");
  if (t && copyKey) return t(copyKey);
  if (isRecord(problem)) {
    // A field present but blank is the same fact as an absent one — it puts no
    // words on the screen — so it falls through to the title, and then to the
    // caller's own copy, instead of rendering an error state with nothing in it.
    const detail = readableField(problem.detail);
    const title = readableField(problem.title);
    return detail ?? title;
  }
  return null;
}

function readableField(value: unknown): string | null {
  return typeof value === "string" && value.trim() !== "" ? value : null;
}

export function problemMessage(
  problem: unknown,
  t?: (key: MessageKey) => string,
): string {
  // A body with no reader text still has to answer something here — this is
  // also the message a ProblemError carries into a stack trace, where an empty
  // string would name nothing. problemMessageOf is the reader's path and
  // answers that same body with catalog copy in the reader's own language.
  return problemDetail(problem, t) ?? "request failed";
}

// A create/update whose server error we want to keep STRUCTURED (not just its
// message) throws this — the raw RFC-7807 body rides along so the form can read
// details.existing_id for the dedupe "view existing" link.
export class ProblemError extends Error {
  readonly problem: unknown;
  constructor(problem: unknown, t?: (key: MessageKey) => string) {
    super(problemMessage(problem, t));
    this.name = "ProblemError";
    this.problem = problem;
  }
}

export function throwProblem(
  problem: unknown,
  t?: (key: MessageKey) => string,
): never {
  throw new ProblemError(problem, t);
}

// The data arm of an API call's result: the arm whose error is absent.
type ApiAnswer<R> = R extends { data: infer D; error?: undefined } ? D : never;

// unwrap answers an API call's data, or throws its problem as throwProblem does.
export function unwrap<R extends { data?: unknown; error?: unknown }>(
  result: R,
  t?: (key: MessageKey) => string,
): ApiAnswer<R>;
export function unwrap(
  result: { data?: unknown; error?: unknown },
  t?: (key: MessageKey) => string,
): unknown {
  if (result.error) throwProblem(result.error, t);
  return result.data;
}

// The same discriminator, read off a query/mutation FAILURE rather than a raw
// body: only a ProblemError carries a server problem, so a network exception
// or a thrown Error never claims a server code it doesn't have.
export function problemCodeOf(error: unknown): string | null {
  return error instanceof ProblemError ? problemCode(error.problem) : null;
}

// The ONE way a caught failure becomes words: only a ProblemError's detail was
// written for a reader, so a bug, a thrown string or a bare fetch error cannot.
//
// A problem body with no words in it, a proxy's 502 among them, reads as the
// shared line too; a body that carries text always keeps its own.
//
// A caller's `fallback` is its own translated copy and beats the shared line; a
// write the network refused beats both, as only it knows whether it was sent.
export function problemMessageOf(
  error: unknown,
  t: (key: MessageKey) => string,
  fallback?: string,
): string {
  const refusedWrite = writeFailureKey(error);
  if (refusedWrite !== null) return t(refusedWrite);
  const detail =
    error instanceof ProblemError ? problemDetail(error.problem, t) : null;
  return detail ?? fallback ?? t("common.errorNoCause");
}

// Its counterpart: the ONE place a failure the reader is NOT shown reaches the
// console. A ProblemError is already on screen, and an outage on its banner.
//
// Wired ONCE, as the client's mutation-cache sink (app/queryclient.ts,
// FE-PARAM-4), never per mutation and never as a render-time call or an effect
// watching `isError`. The cache observes every mutation the application runs,
// so no screen has to remember this and none can lose it; and because
// react-query runs a mutation to completion independently of whichever
// component started it, the sink fires exactly once per actual failure —
// including the one where the reader leaves mid-flight and the component that
// would have hosted an effect is already unmounted when the request settles.
export function logUnexpectedError(error: unknown): void {
  if (!(error instanceof ProblemError || error instanceof ConnectivityError)) {
    console.error(error);
  }
}

// The same per-field assertions read off a query/mutation FAILURE, on the same
// terms as problemCodeOf: only a ProblemError carries a server problem, so a
// network exception never claims field errors it doesn't have.
export function problemFieldErrorsOf(error: unknown): FieldProblem[] {
  return error instanceof ProblemError ? problemFieldErrors(error.problem) : [];
}

// A 409 naming the If-Match precondition failure: the record changed under the
// caller since the form opened, which problemExistingId's duplicate is not.
export function isVersionSkew(problem: unknown): boolean {
  return problemCode(problem) === "version_skew";
}

// The same question, read off a query/mutation FAILURE rather than a raw body,
// on the same terms as problemCodeOf: only a ProblemError carries a server
// problem, so a dropped connection never reads as a concurrent edit. Two call
// sites had spelled the instanceof-then-unwrap step by hand, which is one
// forgotten guard away from telling a reader somebody else changed the record
// when the request never reached the server at all.
export function isVersionSkewOf(error: unknown): boolean {
  return error instanceof ProblemError && isVersionSkew(error.problem);
}

// A 409 whose code names the "already decided" race — another caller (or
// the same one, replayed) already approved/rejected this staged item before
// this request landed. Distinguished from version_skew: the row itself
// didn't change, the DECISION already happened, so the honest response is
// to drop the stale pending row rather than offer a re-stage retry.
export function isAlreadyDecided(problem: unknown): boolean {
  return problemCode(problem) === "already_decided";
}

// A 409 whose code names the consent suppression gate: the send's recipients
// have no active `granted` contact_consent for the purpose it falls under
// (default-deny per purpose, ADR-0011). Distinguished from RBAC (403) and
// validation (422) so the composer can point the user at the consent surface
// rather than showing a raw server detail.
export function isConsentNotGranted(problem: unknown): boolean {
  return problemCode(problem) === "consent_not_granted";
}

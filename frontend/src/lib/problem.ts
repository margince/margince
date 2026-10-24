import type { MessageKey } from "../i18n/en";
import { ConnectivityError, writeFailureKey } from "./connectivity";
import {
  type FieldProblem,
  isRecord,
  problemCode,
  problemFieldErrors,
} from "./problembody";
import { PROBLEM_CODE_COPY, heldByColleagueCopy } from "./problemcodecopy";

export type { FieldProblem } from "./problembody";
export {
  problemCode,
  problemExistingId,
  problemFieldErrors,
} from "./problembody";

// RFC 7807 bodies carry the server's own detail. Surface it instead of a
// generic failure, so the error state names its cause. `null` says the body
// carried no such text: a non-OK response with no body, or a body that was
// never RFC 7807.
//
// That is why this sits apart from problemMessage below. "The server described
// this failure" and "the server said nothing a reader can use" are different
// facts. Only the second may be answered with catalog copy. A caller that
// cannot tell them apart invents copy over a real detail, or shows a
// placeholder as though the server had spoken.
//
// A refusal is the opposite case. `permission_denied` is one code over two
// authorities. One is an object-RBAC denial (this role does not admit the
// action on this kind of record). The other is a row-authority denial (the
// record is on screen through a read share). Nothing on the wire says which. So the
// catalog copy replaces the server's detail and does not guess. Copy that
// named the wrong authority would be worse than the bare sentinel.
//
// Replacing it loses nothing a reader wanted; know this before trying to "keep
// the more specific answer". `httperr` builds a refusal's detail from
// `err.Error()`, and every producer of this sentinel wraps it with internals.
// `auth.Require` sends the RBAC object and verb ("contact.update: permission
// denied"); the admission gate sends its spec name and resolver state. None of
// that is copy, and showing it would leak the shape of the authority model to
// a client. No path sends a sentence written for a reader here.
//
// `seat_tier_insufficient` is the licensing ceiling, a third kind of refusal.
// It is decided before any role is consulted, so a reader whose role admits
// the action is refused anyway. Its server detail is the bare sentinel ("seat
// tier insufficient"). That names a concept no reader has met and offers
// nothing to do. So the catalog copy replaces it and points at the one contact
// who can lift the ceiling. It names "the seat", not "your seat". The same code
// answers a read seat's own mutation and an agent passport acting for one. It
// also answers a grant that would give a read seat write access. A surface
// that knows whose seat it is (sharerevoke.tsx knows it is the recipient's) says so
// before it gets here.
function problemDetail(
  problem: unknown,
  t?: (key: MessageKey) => string,
): string | null {
  const copyKey =
    heldByColleagueCopy(problem) ??
    PROBLEM_CODE_COPY.get(problemCode(problem) ?? "") ??
    fieldCopyKey(problem);
  if (t && copyKey) return t(copyKey);
  if (isRecord(problem)) {
    // A field present but blank puts no words on the screen, like an absent one.
    // So it falls through to the title, then to the caller's own copy, instead
    // of rendering an error state with nothing in it.
    const detail = readableField(problem.detail);
    const title = readableField(problem.title);
    return detail ?? title;
  }
  return null;
}

// A module's own refusal reaches the wire as a `validation_error` with its code
// only on the field fault, so the top-level code never names it.
function fieldCopyKey(problem: unknown): MessageKey | undefined {
  for (const fault of problemFieldErrors(problem)) {
    const key = PROBLEM_CODE_COPY.get(fault.code);
    if (key) return key;
  }
  return undefined;
}

function readableField(value: unknown): string | null {
  return typeof value === "string" && value.trim() !== "" ? value : null;
}

export function problemMessage(
  problem: unknown,
  t?: (key: MessageKey) => string,
): string {
  // A body with no reader text still has to answer something here. This is
  // also the message a ProblemError carries into a stack trace, where an empty
  // string would name nothing. problemMessageOf is the reader's path and
  // answers that same body with catalog copy in the reader's own language.
  return problemDetail(problem, t) ?? "request failed";
}

// A create/update whose server error must stay structured (not just its
// message) throws this. The raw RFC-7807 body rides along, so the form can
// read details.existing_id for the dedupe "view existing" link.
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

// The same discriminator, read off a query/mutation failure rather than a raw
// body. Only a ProblemError carries a server problem, so a network exception
// or a thrown Error never claims a server code it doesn't have.
export function problemCodeOf(error: unknown): string | null {
  return error instanceof ProblemError ? problemCode(error.problem) : null;
}

// The one way a caught failure becomes words. Only a ProblemError's detail was
// written for a reader, so a bug, a thrown string or a bare fetch error cannot.
//
// A problem body with no words in it, a proxy's 502 among them, reads as the
// shared line too. A body that carries text always keeps its own.
//
// A caller's `fallback` is its own translated copy and beats the shared line.
// A write the network refused beats both, as only it knows whether it was sent.
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

// Its counterpart: the one place a failure the reader is not shown reaches the
// console. A ProblemError is already on screen, and an outage on its banner.
//
// Wired once, as the client's mutation-cache sink (app/queryclient.ts). Never
// per mutation, a render-time call, or an effect watching `isError`. The cache
// observes every mutation, so no screen has to remember this or can lose it.
// React-query runs a mutation to completion apart from the component that
// started it. So the sink fires once per failure, even after the reader leaves
// mid-flight and the component that would host an effect has unmounted.
export function logUnexpectedError(error: unknown): void {
  if (!(error instanceof ProblemError || error instanceof ConnectivityError)) {
    console.error(error);
  }
}

// The same per-field assertions read off a query/mutation failure, on the same
// terms as problemCodeOf. Only a ProblemError carries a server problem, so a
// network exception never claims field errors it doesn't have.
export function problemFieldErrorsOf(error: unknown): FieldProblem[] {
  return error instanceof ProblemError ? problemFieldErrors(error.problem) : [];
}

// A 409 naming the If-Match precondition failure: the record changed under the
// caller since the form opened, which problemExistingId's duplicate is not.
export function isVersionSkew(problem: unknown): boolean {
  return problemCode(problem) === "version_skew";
}

// The same question, read off a query/mutation failure rather than a raw body,
// on the same terms as problemCodeOf. Only a ProblemError carries a server
// problem, so a dropped connection never reads as a concurrent edit. An
// instanceof-then-unwrap spelled by hand is one forgotten guard away from
// telling a reader somebody changed the record when the request never landed.
export function isVersionSkewOf(error: unknown): boolean {
  return error instanceof ProblemError && isVersionSkew(error.problem);
}

// A 409 whose code names the "already decided" race. Another caller (or the
// same one, replayed) already approved or rejected this staged item before
// this request landed. Unlike version_skew, the row itself didn't change; the
// decision already happened. So the right response is to drop the stale
// pending row rather than offer a re-stage retry.
export function isAlreadyDecided(problem: unknown): boolean {
  return problemCode(problem) === "already_decided";
}

// A 409 whose code names the consent suppression gate: the send's recipients
// have no active `granted` contact_consent for the purpose it falls under
// (default-deny per purpose). Distinguished from RBAC (403) and
// validation (422) so the composer can point the user at the consent surface
// rather than showing a raw server detail.
export function isConsentNotGranted(problem: unknown): boolean {
  return problemCode(problem) === "consent_not_granted";
}

// The HTTP status out of a thrown RFC-7807 body, or 0 when the failure carried
// none (a dropped connection throws no problem document at all).
export function problemStatus(err: unknown): number {
  // `typeof null === "object"`, so the null check is needed. Without it, a
  // ProblemError with a null body throws here and turns a handled error into
  // an unhandled one.
  if (
    !(err instanceof ProblemError) ||
    typeof err.problem !== "object" ||
    err.problem === null
  ) {
    return 0;
  }
  if (!("status" in err.problem)) {
    return 0;
  }
  const { status } = err.problem;
  return typeof status === "number" ? status : 0;
}

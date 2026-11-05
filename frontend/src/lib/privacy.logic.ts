import type { components } from "../api/schema";
import type { StatusKind } from "../design-system/statusbadge";
import type { MessageKey } from "../i18n/en";
import { problemFieldErrors } from "./problem";

export type DsrStatus = "open" | "in_progress" | "fulfilled" | "rejected";
export type DsrKind = "access" | "rectify" | "erasure";
export type DsrStatusFacet = "all" | DsrStatus;

export const DSR_STATUS_FACETS: readonly DsrStatusFacet[] = [
  "all",
  "open",
  "in_progress",
  "fulfilled",
  "rejected",
];

// Mirrors the server's closed status machine (consent/dsr.go:58-61). A
// closed request never reopens — a new concern is a new request. Offering a
// transition the store rejects would be a 422 the human never needed to see.
const TRANSITIONS: Record<DsrStatus, readonly DsrStatus[]> = {
  open: ["in_progress", "fulfilled", "rejected"],
  in_progress: ["fulfilled", "rejected"],
  fulfilled: [],
  rejected: [],
};

export function nextStatuses(current: DsrStatus): readonly DsrStatus[] {
  return TRANSITIONS[current];
}

export function isTerminal(status: DsrStatus): boolean {
  return TRANSITIONS[status].length === 0;
}

// Pure: the caller supplies now (format/now.ts#useNow) so nothing here races
// a real clock. A closed request is never overdue — the deadline it met (or
// missed) stopped mattering when it closed.
export function isOverdue(
  dueAtIso: string,
  status: DsrStatus,
  nowMs: number,
): boolean {
  if (isTerminal(status)) {
    return false;
  }
  return Date.parse(dueAtIso) < nowMs;
}

export const DSR_KIND_LABEL: Record<DsrKind, MessageKey> = {
  access: "privacy.kindAccess",
  rectify: "privacy.kindRectify",
  erasure: "privacy.kindErasure",
};

export const DSR_STATUS_LABEL: Record<DsrStatus, MessageKey> = {
  open: "privacy.statusOpen",
  in_progress: "privacy.statusInProgress",
  fulfilled: "privacy.statusFulfilled",
  rejected: "privacy.statusRejected",
};

export type IdentityCheck = NonNullable<
  components["schemas"]["UpdateDataSubjectRequest"]["identity_check"]
>;

export const IDENTITY_CHECKS: readonly IdentityCheck[] = [
  "confirm_link",
  "address_on_file",
  "id_document",
  "in_person",
  "not_verified",
];

export const IDENTITY_CHECK_LABEL: Record<IdentityCheck, MessageKey> = {
  confirm_link: "privacy.identityCheck.confirm_link",
  address_on_file: "privacy.identityCheck.address_on_file",
  id_document: "privacy.identityCheck.id_document",
  in_person: "privacy.identityCheck.in_person",
  not_verified: "privacy.identityCheck.not_verified",
};

// What the officer drafts for a close: the answer and what the request's proof
// records about it.
export type DsrClosing = Readonly<{
  resolution: string;
  identityCheck: IdentityCheck | "";
  requesterEmail: string;
}>;

// A close to `next` needs its answer and the identity check, and an identity
// nobody could check closes only as rejected. The server takes a confirm-link
// case without the check; the form asks every time, so one rule covers both.
export function closingReady(
  next: DsrStatus,
  closing: DsrClosing,
  storedResolution: string | null | undefined,
): boolean {
  return (
    Boolean(closing.resolution.trim() || storedResolution) &&
    closing.identityCheck !== "" &&
    !(next === "fulfilled" && closing.identityCheck === "not_verified")
  );
}

// The patch body that moves a request to `status`. Blank text is omitted: an
// empty string is a value the server would write.
export function closingBody(
  status: DsrStatus,
  closing: DsrClosing,
): components["schemas"]["UpdateDataSubjectRequest"] {
  const body: components["schemas"]["UpdateDataSubjectRequest"] = { status };
  const answer = closing.resolution.trim();
  if (answer) body.resolution = answer;
  if (!isTerminal(status)) return body;
  if (closing.identityCheck) body.identity_check = closing.identityCheck;
  const address = closing.requesterEmail.trim();
  if (address) body.requester_email = address;
  return body;
}

// Rejected is an outcome the officer chose, not a breakage, so it stays grey.
export const DSR_STATUS_KIND: Record<DsrStatus, StatusKind> = {
  open: "neutral",
  in_progress: "neutral",
  fulfilled: "done",
  rejected: "neutral",
};

// The status machine refuses an illegal move with a validation error naming
// `status`, which is the one sign the request moved on underneath this reader.
export function isIllegalTransition(problem: unknown): boolean {
  return problemFieldErrors(problem).some((error) => error.field === "status");
}

// An erasure fulfil's only conflict is a contact under statutory legal hold.
export function isLegalHold(problem: unknown): boolean {
  return (
    !!problem &&
    typeof problem === "object" &&
    "code" in problem &&
    problem.code === "conflict"
  );
}

// A DSR due date is a statutory deadline picked as a calendar day in the
// operator's own zone; the instant it names is minted by the zone module so
// the timeline's date range and this deadline cannot disagree about what a
// day is. Re-exported so the privacy screen keeps reading it from its logic.
export { endOfDayInZone } from "../format/timezone";

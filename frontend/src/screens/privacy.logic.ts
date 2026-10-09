import type { MessageKey } from "../i18n/en";
import { problemFieldErrors } from "./common";

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

// Work in flight reads info; a closed request reads its outcome.
export const DSR_STATUS_TONE: Record<
  DsrStatus,
  "info" | "success" | "danger" | undefined
> = {
  open: undefined,
  in_progress: "info",
  fulfilled: "success",
  rejected: "danger",
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
